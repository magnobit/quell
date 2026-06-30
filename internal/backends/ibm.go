// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package backends implements hardware submission for IBM Quantum, AWS Braket,
// and Google Quantum Engine.
package backends

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magnobit/quell/internal/config"
)

const ibmBase = "https://api.quantum.ibm.com"

// RunIBM submits a circuit to IBM Quantum via the Qiskit Runtime REST API
// (Sampler V2 primitive). token is the IBM Quantum API token; device is the
// backend name (e.g. "ibm_brisbane"); instance is "hub/group/project".
func RunIBM(cfg *config.IBMConfig, qasm3 string, numQubits int) (*RunResult, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("ibm: token is required (set ibm.token in quell.config.yml or IBM_QUANTUM_TOKEN env var)")
	}
	if cfg.Device == "" {
		return nil, fmt.Errorf("ibm: device is required (e.g. ibm_brisbane)")
	}
	if cfg.Instance == "" {
		cfg.Instance = "ibm-q/open/main"
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1024
	}

	jobID, err := ibmSubmit(cfg.Token, cfg.Device, cfg.Instance, qasm3, shots)
	if err != nil {
		return nil, fmt.Errorf("ibm: submit: %w", err)
	}
	fmt.Printf("  IBM job submitted: %s\n", jobID)

	if err := ibmPoll(cfg.Token, jobID); err != nil {
		return nil, fmt.Errorf("ibm: %w", err)
	}

	counts, err := ibmResults(cfg.Token, jobID, shots, numQubits)
	if err != nil {
		return nil, fmt.Errorf("ibm: results: %w", err)
	}

	return &RunResult{
		JobID:   jobID,
		Backend: "IBM Quantum / " + cfg.Device,
		Shots:   shots,
		Counts:  counts,
	}, nil
}

func ibmSubmit(token, backend, instance, qasm3 string, shots int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"program_id": "sampler",
		"backend":    backend,
		"instance":   instance,
		"params": map[string]any{
			"pubs":    []any{[]any{qasm3, nil, shots}},
			"version": 2,
		},
	})

	resp, err := ibmDo("POST", ibmBase+"/runtime/jobs", token, body)
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

func ibmPoll(token, jobID string) error {
	url := fmt.Sprintf("%s/runtime/jobs/%s", ibmBase, jobID)
	for {
		resp, err := ibmDo("GET", url, token, nil)
		if err != nil {
			return err
		}
		var r struct {
			Status string `json:"status"`
			Error  struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()

		switch r.Status {
		case "Completed":
			fmt.Print("\n")
			return nil
		case "Failed", "Cancelled":
			msg := r.Error.Message
			if msg == "" {
				msg = r.Status
			}
			return fmt.Errorf("job %s: %s", jobID, msg)
		default:
			fmt.Printf("\r  IBM job status: %-10s (job: %s)", r.Status, jobID[:min(8, len(jobID))])
			time.Sleep(4 * time.Second)
		}
	}
}

func ibmResults(token, jobID string, shots, numQubits int) (map[string]int, error) {
	url := fmt.Sprintf("%s/runtime/jobs/%s/results", ibmBase, jobID)
	resp, err := ibmDo("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	// Format 1: circuit-runner — {"results": [{"data": {"counts": {"0x0": 512}}}]}
	var crFmt struct {
		Results []struct {
			Data struct {
				Counts map[string]int `json:"counts"`
			} `json:"data"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &crFmt) == nil && len(crFmt.Results) > 0 && crFmt.Results[0].Data.Counts != nil {
		return hexCountsToStr(crFmt.Results[0].Data.Counts, numQubits), nil
	}

	// Format 2: Sampler V2 — {"results": [{"data": {"<reg>": {"__class__": "BitArray", "array": ..., "shape": [...]}}}]}
	var samplerFmt struct {
		Results []struct {
			Data     map[string]json.RawMessage `json:"data"`
			Metadata struct {
				NumQubits int `json:"num_qubits"`
			} `json:"metadata"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &samplerFmt) == nil && len(samplerFmt.Results) > 0 {
		nq := numQubits
		if samplerFmt.Results[0].Metadata.NumQubits > 0 {
			nq = samplerFmt.Results[0].Metadata.NumQubits
		}
		for _, regRaw := range samplerFmt.Results[0].Data {
			// Try BitArray with "array" field
			var ba struct {
				Array string `json:"array"`
				Shape []int  `json:"shape"`
			}
			if json.Unmarshal(regRaw, &ba) == nil && ba.Array != "" {
				return decodeBitArray(ba.Array, ba.Shape, nq)
			}
			// Try ndarray format with "ndarray" field
			var nd struct {
				Ndarray string `json:"ndarray"`
				Shape   []int  `json:"shape"`
			}
			if json.Unmarshal(regRaw, &nd) == nil && nd.Ndarray != "" {
				return decodeBitArray(nd.Ndarray, nd.Shape, nq)
			}
		}
	}

	return nil, fmt.Errorf("unrecognised result format: %s", string(raw))
}

// decodeBitArray decodes IBM's packed or unpacked BitArray.
// shape[0] = num_shots, shape[1] = bytes_per_shot (packed, MSB-first) or bits (if unpacked).
func decodeBitArray(b64 string, shape []int, numQubits int) (map[string]int, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("base64: %w", err)
		}
	}

	if len(shape) < 1 {
		return nil, fmt.Errorf("empty shape")
	}
	numShots := shape[0]
	if numShots == 0 {
		return nil, fmt.Errorf("zero shots in shape")
	}

	// Unpacked: one byte per bit — len == numShots * numQubits
	if len(data) == numShots*numQubits {
		counts := make(map[string]int, 1<<numQubits)
		bits := make([]byte, numQubits)
		for i := 0; i < numShots; i++ {
			for j := 0; j < numQubits; j++ {
				if data[i*numQubits+j] != 0 {
					bits[j] = '1'
				} else {
					bits[j] = '0'
				}
			}
			counts[string(bits)]++
		}
		return counts, nil
	}

	// Packed: ceil(numQubits/8) bytes per shot, MSB = qubit 0
	bytesPerShot := (numQubits + 7) / 8
	if len(shape) >= 2 {
		bytesPerShot = shape[1]
	}
	if len(data) != numShots*bytesPerShot {
		return nil, fmt.Errorf("data length %d doesn't match shots=%d × bytes=%d", len(data), numShots, bytesPerShot)
	}

	counts := make(map[string]int, 1<<numQubits)
	bits := make([]byte, numQubits)
	for i := 0; i < numShots; i++ {
		row := data[i*bytesPerShot : (i+1)*bytesPerShot]
		for j := 0; j < numQubits; j++ {
			byteIdx := j / 8
			bitIdx := 7 - (j % 8) // MSB first
			if row[byteIdx]&(1<<uint(bitIdx)) != 0 {
				bits[j] = '1'
			} else {
				bits[j] = '0'
			}
		}
		counts[string(bits)]++
	}
	return counts, nil
}

// hexCountsToStr converts IBM hex counts {"0x0": 512} → {"00": 512}.
func hexCountsToStr(hexCounts map[string]int, numQubits int) map[string]int {
	width := numQubits
	if width < 1 {
		// Infer from max value
		maxVal := 0
		for k := range hexCounts {
			n, _ := strconv.ParseInt(strings.TrimPrefix(k, "0x"), 16, 64)
			if int(n) > maxVal {
				maxVal = int(n)
			}
		}
		if maxVal > 0 {
			width = int(math.Ceil(math.Log2(float64(maxVal+1))))
		}
		if width < 1 {
			width = 1
		}
	}

	counts := make(map[string]int, len(hexCounts))
	for k, v := range hexCounts {
		n, _ := strconv.ParseInt(strings.TrimPrefix(k, "0x"), 16, 64)
		counts[fmt.Sprintf("%0*b", width, n)] = v
	}
	return counts
}

func ibmDo(method, url, token string, body []byte) (*http.Response, error) {
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
	req.Header.Set("IBM-API-Version", "2024-06-14")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from IBM: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
