// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
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

	token, err := azureAccessToken(cfg.TenantID, cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("azure: auth: %w", err)
	}
	fmt.Println("  Azure AAD auth OK")

	jobID, err := azureSubmit(token, cfg, qasm3, shots)
	if err != nil {
		return nil, fmt.Errorf("azure: submit: %w", err)
	}
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
func azureAccessToken(tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("scope", azureScope)

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
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
		"id":                jobID,
		"target":            cfg.Target,
		"name":              "quell-job",
		"inputDataFormat":   inputDataFormat,
		"outputDataFormat":  "microsoft.quantum-results.v1",
		"inputParams":       inputParams,
		"inputData":         qasm3,
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
	for {
		resp, err := azureDo("GET", reqURL, token, nil)
		if err != nil {
			return err
		}
		var r struct {
			Status    string `json:"status"`
			ErrorData struct {
				Message string `json:"message"`
			} `json:"errorData"`
		}
		json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()

		switch r.Status {
		case "Succeeded":
			fmt.Print("\n")
			return nil
		case "Failed", "Cancelled":
			msg := r.ErrorData.Message
			if msg == "" {
				msg = r.Status
			}
			return fmt.Errorf("job %s: %s", jobID, msg)
		default:
			fmt.Printf("\r  Azure Quantum job status: %-12s", r.Status)
			time.Sleep(5 * time.Second)
		}
	}
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
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from Azure: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}
