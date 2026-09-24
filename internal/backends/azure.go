// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/magnobit/quell/internal/config"
)

const azureScope = "https://quantum.microsoft.com/.default"

// RunAzure submits a circuit to Azure Quantum and returns measurement counts.
// Auth uses the AAD OAuth2 client-credentials flow (service principal),
// mirroring the JWT/token-exchange shape already used for Google service
// accounts in google.go's googleAccessToken.
//
// Note: a production Azure Quantum integration normally uploads the
// compiled circuit to an Azure Blob Storage container first and references
// it by SAS URI in the job-create request, rather than inlining the source.
// This adapter inlines qasm3 directly to keep the same submit → poll →
// results shape as the other backends in this package; a real deployment
// would add a blob-upload step ahead of azureSubmit.
func RunAzure(cfg *config.AzureConfig, qasm3 string) (*RunResult, error) {
	if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("azure: tenant_id, client_id, and client_secret are required (azure.* in quell.config.yml)")
	}
	if cfg.SubscriptionID == "" || cfg.ResourceGroup == "" || cfg.Workspace == "" {
		return nil, fmt.Errorf("azure: subscription_id, resource_group, and workspace are required")
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("azure: target is required (e.g. ionq.simulator, quantinuum.sim.h1-1sc)")
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 500
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID)
	if cfg.BaseURL != "" {
		tokenURL = cfg.BaseURL + "/oauth2/v2.0/token"
	}
	token, err := azureAccessToken(tokenURL, cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("azure: auth: %w", err)
	}
	fmt.Println("  Azure AAD auth OK")

	jobID, err := azureSubmit(token, cfg, qasm3, shots)
	if err != nil {
		return nil, fmt.Errorf("azure: submit: %w", err)
	}
	notifySubmitted(cfg.OnSubmitted, jobID)
	fmt.Printf("  Azure Quantum job submitted: %s\n", jobID)

	if err := azurePoll(token, cfg, jobID); err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}

	counts, err := azureResults(token, cfg, jobID)
	if err != nil {
		return nil, fmt.Errorf("azure: results: %w", err)
	}

	return &RunResult{
		JobID:   jobID,
		Backend: "Azure Quantum / " + cfg.Target,
		Shots:   shots,
		Counts:  counts,
	}, nil
}

// azureAccessToken exchanges AAD service-principal credentials for a bearer
// token via the client-credentials grant.
func azureAccessToken(tokenURL, clientID, clientSecret string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("scope", azureScope)

	resp, err := providerHTTPClient.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", &ProviderError{Provider: "azure", Class: classifyNet(err), Message: redactSecrets(err.Error())}
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tok.Error != "" {
		return "", fmt.Errorf("%s: %s", tok.Error, tok.ErrorDesc)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}
	return tok.AccessToken, nil
}

func azureWorkspaceBase(cfg *config.AzureConfig) string {
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Quantum/workspaces/%s",
		cfg.SubscriptionID, cfg.ResourceGroup, cfg.Workspace)
}

func azureSubmit(token string, cfg *config.AzureConfig, qasm3 string, shots int) (string, error) {
	jobID := fmt.Sprintf("quell-%d", time.Now().UnixNano())
	reqURL := fmt.Sprintf("%s/jobs/%s?api-version=2022-09-12-preview", azureWorkspaceBase(cfg), jobID)

	// Real Azure Quantum targets expect a provider-specific input format
	// (e.g. "honeywell.openqasm.v1", "ionq.circuit.v1", "rigetti.openqasm.v1")
	// chosen to match cfg.Target's provider. Quell always emits OpenQASM 3,
	// so this generic value is a placeholder — override it per-target with
	// `--set azure.inputDataFormat=ionq.circuit.v1` (or the config file's
	// `azure.extra.inputDataFormat`) until the provider-specific mapping is
	// built in.
	inputDataFormat := "honeywell.openqasm.v1"
	extra := make(map[string]string, len(cfg.Extra))
	for k, v := range cfg.Extra {
		extra[k] = v
	}
	if v, ok := extra["inputDataFormat"]; ok {
		inputDataFormat = v
		delete(extra, "inputDataFormat")
	}

	inputParams := map[string]any{
		"shots": shots,
	}
	mergeExtra(inputParams, extra)

	body, _ := json.Marshal(map[string]any{
		"id":               jobID,
		"target":           cfg.Target,
		"name":             "quell-job",
		"inputDataFormat":  inputDataFormat,
		"outputDataFormat": "microsoft.quantum-results.v1",
		"inputParams":      inputParams,
		"inputData":        qasm3,
	})

	resp, err := azureDo("PUT", reqURL, token, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var r struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	if r.ID == "" {
		// Azure's create-job PUT sometimes echoes no body on success — fall
		// back to the id we generated and sent in the request.
		return jobID, nil
	}
	return r.ID, nil
}

func azurePoll(token string, cfg *config.AzureConfig, jobID string) error {
	reqURL := fmt.Sprintf("%s/jobs/%s?api-version=2022-09-12-preview", azureWorkspaceBase(cfg), jobID)
	return pollUntil("azure", jobID, func() (pollTick, error) {
		resp, err := azureDo("GET", reqURL, token, nil)
		if err != nil {
			return pollTick{}, err
		}
		var r struct {
			Status    string `json:"status"`
			ErrorData struct {
				Message string `json:"message"`
			} `json:"errorData"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if decErr != nil {
			return pollTick{}, &ProviderError{Provider: "azure", Class: ClassInvalidRequest, Message: "malformed job status"}
		}
		switch r.Status {
		case "Succeeded":
			return pollTick{Done: true, Status: r.Status}, nil
		case "Failed", "Cancelled":
			return pollTick{Failed: true, Status: r.Status, Message: r.ErrorData.Message}, nil
		default:
			return pollTick{Status: r.Status}, nil
		}
	})
}

func azureResults(token string, cfg *config.AzureConfig, jobID string) (map[string]int, error) {
	reqURL := fmt.Sprintf("%s/jobs/%s?api-version=2022-09-12-preview", azureWorkspaceBase(cfg), jobID)
	resp, err := azureDo("GET", reqURL, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var r struct {
		OutputDataURI string `json:"outputDataUri"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.OutputDataURI == "" {
		return nil, fmt.Errorf("no output data URI in job response: %s", string(raw))
	}

	resultResp, err := http.Get(r.OutputDataURI)
	if err != nil {
		return nil, fmt.Errorf("fetch results blob: %w", err)
	}
	defer resultResp.Body.Close()
	resultRaw, _ := io.ReadAll(resultResp.Body)

	var out struct {
		Histogram map[string]int `json:"histogram"`
	}
	if err := json.Unmarshal(resultRaw, &out); err != nil || out.Histogram == nil {
		return nil, fmt.Errorf("unrecognised result format: %s", string(resultRaw))
	}
	return out.Histogram, nil
}

func azureDo(method, url, token string, body []byte) (*http.Response, error) {
	return doJSON("azure", method, url, "Bearer "+token, nil, body)
}

// CancelAzure asks Azure Quantum to cancel an in-flight job.
func CancelAzure(cfg *config.AzureConfig, providerJobID string) error {
	if cfg == nil || cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return fmt.Errorf("azure: tenant_id, client_id, and client_secret are required to cancel")
	}
	if providerJobID == "" {
		return fmt.Errorf("azure: provider job id is required to cancel")
	}
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID)
	if cfg.BaseURL != "" {
		tokenURL = cfg.BaseURL + "/oauth2/v2.0/token"
	}
	token, err := azureAccessToken(tokenURL, cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return fmt.Errorf("azure: auth: %w", err)
	}
	reqURL := fmt.Sprintf("%s/jobs/%s?api-version=2022-09-12-preview", azureWorkspaceBase(cfg), providerJobID)
	resp, err := azureDo("DELETE", reqURL, token, nil)
	if err != nil {
		return fmt.Errorf("azure: cancel: %w", err)
	}
	resp.Body.Close()
	return nil
}
