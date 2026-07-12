// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

	jobID, err := rigettiSubmit(cfg.APIKey, cfg.Device, qasm3, shots, cfg.Extra)
	if err != nil {
		return nil, fmt.Errorf("rigetti: submit: %w", err)
	}
	fmt.Printf("  Rigetti job submitted: %s\n", jobID)

	if err := rigettiPoll(cfg.APIKey, jobID); err != nil {
		return nil, fmt.Errorf("rigetti: %w", err)
	}

	counts, err := rigettiResults(cfg.APIKey, jobID)
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

func rigettiSubmit(apiKey, device, qasm3 string, shots int, extra map[string]string) (string, error) {
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

	resp, err := rigettiDo("POST", rigettiBase+"/jobs", apiKey, body)
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

func rigettiPoll(apiKey, jobID string) error {
	url := fmt.Sprintf("%s/jobs/%s", rigettiBase, jobID)
	for {
		resp, err := rigettiDo("GET", url, apiKey, nil)
		if err != nil {
			return err
		}
		var r struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()

		switch r.Status {
		case "COMPLETED":
			fmt.Print("\n")
			return nil
		case "FAILED", "CANCELLED":
			msg := r.Error
			if msg == "" {
				msg = r.Status
			}
			return fmt.Errorf("job %s: %s", jobID, msg)
		default:
			fmt.Printf("\r  Rigetti job status: %-10s", r.Status)
			time.Sleep(4 * time.Second)
		}
	}
}

func rigettiResults(apiKey, jobID string) (map[string]int, error) {
	url := fmt.Sprintf("%s/jobs/%s/results", rigettiBase, jobID)
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
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from Rigetti: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}
