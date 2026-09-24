// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/magnobit/quell/internal/config"
)

// Rigetti's production stack (Quantum Cloud Services) is normally accessed
// through pyQuil, which talks to devices via a gRPC translation layer
// (Quil-T), not a plain REST job API. This adapter targets the public REST
// job-submission surface Rigetti exposes for QCS, modeled with the same
// submit → poll → results shape as the other backends in this package so
// every adapter presents one consistent interface.
const rigettiBase = "https://api.qcs.rigetti.com/v1"

// RunRigetti submits a circuit to Rigetti QCS and returns measurement counts.
func RunRigetti(cfg *config.RigettiConfig, qasm3 string) (*RunResult, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("rigetti: api_key is required (set rigetti.api_key in quell.config.yml or RIGETTI_API_KEY env var)")
	}
	if cfg.Device == "" {
		return nil, fmt.Errorf("rigetti: device is required (e.g. Aspen-M-3)")
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1024
	}
	base := rigettiBase
	if cfg.BaseURL != "" {
		base = cfg.BaseURL
	}

	jobID, err := rigettiSubmit(base, cfg.APIKey, cfg.Device, qasm3, shots, cfg.Extra)
	if err != nil {
		return nil, fmt.Errorf("rigetti: submit: %w", err)
	}
	notifySubmitted(cfg.OnSubmitted, jobID)
	fmt.Printf("  Rigetti job submitted: %s\n", jobID)

	if err := rigettiPoll(base, cfg.APIKey, jobID); err != nil {
		return nil, fmt.Errorf("rigetti: %w", err)
	}

	counts, err := rigettiResults(base, cfg.APIKey, jobID)
	if err != nil {
		return nil, fmt.Errorf("rigetti: results: %w", err)
	}

	return &RunResult{
		JobID:   jobID,
		Backend: "Rigetti / " + cfg.Device,
		Shots:   shots,
		Counts:  counts,
	}, nil
}

func rigettiSubmit(base, apiKey, device, qasm3 string, shots int, extra map[string]string) (string, error) {
	payload := map[string]any{
		"quantumProcessorId": device,
		"shots":              shots,
		"program": map[string]any{
			"format": "openqasm3",
			"source": qasm3,
		},
	}
	mergeExtra(payload, extra)
	body, _ := json.Marshal(payload)

	resp, err := rigettiDo("POST", base+"/jobs", apiKey, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var r struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if r.Error != "" {
		return "", fmt.Errorf("%s", r.Error)
	}
	if r.ID == "" {
		return "", fmt.Errorf("no job id returned")
	}
	return r.ID, nil
}

func rigettiPoll(base, apiKey, jobID string) error {
	url := fmt.Sprintf("%s/jobs/%s", base, jobID)
	return pollUntil("rigetti", jobID, func() (pollTick, error) {
		resp, err := rigettiDo("GET", url, apiKey, nil)
		if err != nil {
			return pollTick{}, err
		}
		var r struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if decErr != nil {
			return pollTick{}, &ProviderError{Provider: "rigetti", Class: ClassInvalidRequest, Message: "malformed job status"}
		}
		switch r.Status {
		case "COMPLETED":
			return pollTick{Done: true, Status: r.Status}, nil
		case "FAILED", "CANCELLED":
			return pollTick{Failed: true, Status: r.Status, Message: r.Error}, nil
		default:
			return pollTick{Status: r.Status}, nil
		}
	})
}

func rigettiResults(base, apiKey, jobID string) (map[string]int, error) {
	url := fmt.Sprintf("%s/jobs/%s/results", base, jobID)
	resp, err := rigettiDo("GET", url, apiKey, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var r struct {
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Counts == nil {
		return nil, fmt.Errorf("unrecognised result format: %s", string(raw))
	}
	return r.Counts, nil
}

func rigettiDo(method, url, apiKey string, body []byte) (*http.Response, error) {
	return doJSON("rigetti", method, url, "Bearer "+apiKey, nil, body)
}

// CancelRigetti asks Rigetti QCS to cancel an in-flight job.
func CancelRigetti(cfg *config.RigettiConfig, providerJobID string) error {
	if cfg == nil || cfg.APIKey == "" {
		return fmt.Errorf("rigetti: api_key is required to cancel")
	}
	if providerJobID == "" {
		return fmt.Errorf("rigetti: provider job id is required to cancel")
	}
	base := rigettiBase
	if cfg.BaseURL != "" {
		base = cfg.BaseURL
	}
	resp, err := rigettiDo("DELETE", base+"/jobs/"+providerJobID, cfg.APIKey, nil)
	if err != nil {
		return fmt.Errorf("rigetti: cancel: %w", err)
	}
	resp.Body.Close()
	return nil
}
