// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/internal/targetfmt"
)

const (
	azureScope      = "https://quantum.microsoft.com/.default"
	azureAPIVersion = "2026-01-15-preview"
)

// RunAzure submits a circuit to Azure Quantum and returns measurement counts.
// Auth uses the AAD client-credentials flow (service principal). Following
// the data-plane contract, the circuit is uploaded to the workspace's
// storage through a SAS URL and the job references it by URI.
func RunAzure(cfg *config.AzureConfig, qasm3 string) (*RunResult, error) {
	if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("azure: tenant_id, client_id, and client_secret are required (azure.* in quell.config.yml)")
	}
	if cfg.SubscriptionID == "" || cfg.ResourceGroup == "" || cfg.Workspace == "" {
		return nil, fmt.Errorf("azure: subscription_id, resource_group, and workspace are required")
	}
	if cfg.Location == "" && cfg.BaseURL == "" {
		return nil, fmt.Errorf("azure: location is required (the workspace region, e.g. eastus)")
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("azure: target is required (e.g. quantinuum.sim.h2-1e)")
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 500
	}
	providerID := strings.SplitN(cfg.Target, ".", 2)[0]
	extra := make(map[string]string, len(cfg.Extra))
	for k, v := range cfg.Extra {
		extra[k] = v
	}
	input, formats, err := azureJobInput(cfg.Target, qasm3, extra)
	if err != nil {
		return nil, err
	}

	token, err := azureAccessToken(azureTokenURL(cfg), cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("azure: auth: %w", err)
	}
	fmt.Println("  Azure AAD auth OK")

	jobID, err := azureSubmit(token, cfg, providerID, formats, input, shots, extra)
	if err != nil {
		return nil, fmt.Errorf("azure: submit: %w", err)
	}
	notifySubmitted(cfg.OnSubmitted, jobID)
	fmt.Printf("  Azure Quantum job submitted: %s\n", jobID)

	outputURI, err := azurePoll(token, cfg, jobID)
	if err != nil {
		return nil, fmt.Errorf("azure: %w", err)
	}

	counts, err := azureResults(outputURI, shots)
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

func azureTokenURL(cfg *config.AzureConfig) string {
	if cfg.BaseURL != "" {
		return strings.TrimRight(cfg.BaseURL, "/") + "/oauth2/v2.0/token"
	}
	return "https://login.microsoftonline.com/" + url.PathEscape(cfg.TenantID) + "/oauth2/v2.0/token"
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

// azureWorkspaceBase is the regional data-plane URL of the workspace.
func azureWorkspaceBase(cfg *config.AzureConfig) string {
	host := "https://" + cfg.Location + ".quantum.azure.com"
	if cfg.BaseURL != "" {
		host = strings.TrimRight(cfg.BaseURL, "/")
	}
	return fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Quantum/workspaces/%s",
		host, url.PathEscape(cfg.SubscriptionID), url.PathEscape(cfg.ResourceGroup), url.PathEscape(cfg.Workspace))
}

func azureJobURL(cfg *config.AzureConfig, jobID string) string {
	return azureWorkspaceBase(cfg) + "/jobs/" + url.PathEscape(jobID) + "?api-version=" + azureAPIVersion
}

// azureSASURI asks the workspace for a SAS URL to a container (blob == "")
// or a blob in it. This API version creates the container when missing.
func azureSASURI(token string, cfg *config.AzureConfig, container, blob string) (string, error) {
	req := map[string]string{"containerName": container}
	if blob != "" {
		req["blobName"] = blob
	}
	body, _ := json.Marshal(req)
	resp, err := azureDo("POST", azureWorkspaceBase(cfg)+"/storage/sasUri?api-version="+azureAPIVersion, token, body)
	if err != nil {
		if strings.Contains(err.Error(), "ManagedIdentityForbiddenStorageAccess") {
			return "", fmt.Errorf("%w — grant the workspace managed identity access to its linked storage account, then wait a few minutes", err)
		}
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		SasURI string `json:"sasUri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.SasURI == "" {
		return "", fmt.Errorf("no SAS URI returned for %s", container)
	}
	return r.SasURI, nil
}

func azureUploadBlob(sasURI string, data []byte) error {
	req, err := http.NewRequest(http.MethodPut, sasURI, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", "text/plain")
	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return &ProviderError{Provider: "azure", Class: classifyNet(err), Message: "upload input: " + redactSecrets(err.Error())}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return httpStatusError("azure", resp.StatusCode, b)
	}
	return nil
}

// azureJobInput turns OpenQASM into the provider body Azure uploads.
// An explicit inputDataFormat keeps the source bytes unchanged so a caller
// can still send a prepared payload.
func azureJobInput(target, qasm3 string, extra map[string]string) ([]byte, [2]string, error) {
	var formats [2]string
	if v, ok := extra["outputDataFormat"]; ok {
		formats[1] = v
		delete(extra, "outputDataFormat")
	}
	if v, ok := extra["inputDataFormat"]; ok {
		formats[0] = v
		delete(extra, "inputDataFormat")
		if formats[1] == "" {
			formats[1] = "microsoft.quantum-results.v1"
		}
		return []byte(qasm3), formats, nil
	}
	payload, err := targetfmt.Azure(target, qasm3)
	if err != nil {
		return nil, formats, fmt.Errorf("azure: %w", err)
	}
	formats[0] = payload.InputFormat
	if formats[1] == "" {
		formats[1] = payload.OutputFormat
	}
	return payload.Body, formats, nil
}

func azureSubmit(token string, cfg *config.AzureConfig, providerID string, formats [2]string, input []byte, shots int, extra map[string]string) (string, error) {
	jobID, err := newUUID()
	if err != nil {
		return "", err
	}
	container := "job-" + jobID
	inputURI, err := azureSASURI(token, cfg, container, "inputData")
	if err != nil {
		return "", err
	}
	if err := azureUploadBlob(inputURI, input); err != nil {
		return "", err
	}
	containerURI, err := azureSASURI(token, cfg, container, "")
	if err != nil {
		return "", err
	}

	inputParams := map[string]any{"shots": shots, "count": shots}
	mergeExtra(inputParams, extra)

	body, _ := json.Marshal(map[string]any{
		"id":               jobID,
		"name":             "quell-job",
		"itemType":         "Job",
		"providerId":       providerID,
		"target":           cfg.Target,
		"containerUri":     containerURI,
		"inputDataUri":     inputURI,
		"inputDataFormat":  formats[0],
		"outputDataFormat": formats[1],
		"inputParams":      inputParams,
	})

	resp, err := azureDo("PUT", azureJobURL(cfg, jobID), token, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var r struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	if r.ID == "" {
		return jobID, nil
	}
	return r.ID, nil
}

// azurePoll waits for the job to finish and returns its output blob URI.
func azurePoll(token string, cfg *config.AzureConfig, jobID string) (string, error) {
	var outputURI string
	err := pollUntil("azure", jobID, func() (pollTick, error) {
		resp, err := azureDo("GET", azureJobURL(cfg, jobID), token, nil)
		if err != nil {
			return pollTick{}, err
		}
		var r struct {
			Status        string `json:"status"`
			OutputDataURI string `json:"outputDataUri"`
			ErrorData     struct {
				Message string `json:"message"`
			} `json:"errorData"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if decErr != nil {
			return pollTick{}, &ProviderError{Provider: "azure", Class: ClassInvalidRequest, Message: "malformed job status"}
		}
		switch r.Status {
		case "Succeeded", "Completed":
			outputURI = r.OutputDataURI
			return pollTick{Done: true, Status: r.Status}, nil
		case "Failed", "Cancelled":
			return pollTick{Failed: true, Status: r.Status, Message: r.ErrorData.Message}, nil
		default:
			return pollTick{Status: r.Status}, nil
		}
	})
	if err == nil && outputURI == "" {
		return "", fmt.Errorf("job finished without an output data URI")
	}
	return outputURI, err
}

// azureResults downloads the output blob. Providers write either a
// histogram (counts or probabilities) or per-register lists of shot
// bitstrings (Quantinuum).
func azureResults(outputURI string, shots int) (map[string]int, error) {
	resp, err := providerHTTPClient.Get(outputURI)
	if err != nil {
		return nil, fmt.Errorf("fetch results blob: %s", redactSecrets(err.Error()))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, httpStatusError("azure", resp.StatusCode, raw)
	}
	return parseAzureResults(raw, shots)
}

func parseAzureResults(raw []byte, shots int) (map[string]int, error) {
	var hist struct {
		Histogram map[string]float64 `json:"histogram"`
	}
	if json.Unmarshal(raw, &hist) == nil && len(hist.Histogram) > 0 {
		sum := 0.0
		for _, v := range hist.Histogram {
			sum += v
		}
		counts := make(map[string]int, len(hist.Histogram))
		probabilities := sum > 0 && sum <= 1.0001
		for k, v := range hist.Histogram {
			if probabilities {
				counts[k] = int(math.Round(v * float64(shots)))
			} else {
				counts[k] = int(math.Round(v))
			}
		}
		return counts, nil
	}
	var regs map[string][]string
	if json.Unmarshal(raw, &regs) == nil && len(regs) > 0 {
		counts := map[string]int{}
		for _, shotsList := range regs {
			for _, bits := range shotsList {
				counts[bits]++
			}
			break
		}
		if len(counts) > 0 {
			return counts, nil
		}
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) == nil {
		if rawCounter, ok := doc["counter"]; ok {
			var counter map[string]float64
			if json.Unmarshal(rawCounter, &counter) == nil && len(counter) > 0 {
				counts := make(map[string]int, len(counter))
				for bits, n := range counter {
					counts[bits] = int(math.Round(n))
				}
				return counts, nil
			}
		}
		for _, rawReg := range doc {
			var shots2 [][]float64
			if json.Unmarshal(rawReg, &shots2) != nil || len(shots2) == 0 {
				continue
			}
			counts := map[string]int{}
			for _, shot := range shots2 {
				var bits strings.Builder
				for _, bit := range shot {
					if bit >= 0.5 {
						bits.WriteByte('1')
					} else {
						bits.WriteByte('0')
					}
				}
				counts[bits.String()]++
			}
			if len(counts) > 0 {
				return counts, nil
			}
		}
	}
	return nil, fmt.Errorf("unrecognised result format: %s", string(raw))
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
	token, err := azureAccessToken(azureTokenURL(cfg), cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return fmt.Errorf("azure: auth: %w", err)
	}
	resp, err := azureDo("DELETE", azureJobURL(cfg, providerJobID), token, nil)
	if err != nil {
		return fmt.Errorf("azure: cancel: %w", err)
	}
	resp.Body.Close()
	return nil
}
