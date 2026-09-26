// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/magnobit/quell/internal/config"
)

const ionqBase = "https://api.ionq.co/v0.4"

// RunIonQ submits a circuit to IonQ Cloud and returns measurement counts.
// cfg.APIKey is the IonQ API key; cfg.Device is the backend id (for example
// "simulator" or "qpu.forte-1"), not the display name.
func RunIonQ(cfg *config.IonQConfig, qasm3 string, numQubits int) (*RunResult, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("ionq: api_key is required (set ionq.api_key in quell.config.yml or IONQ_API_KEY env var)")
	}
	if cfg.Device == "" {
		return nil, fmt.Errorf("ionq: device is required (e.g. simulator, qpu.forte-1)")
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1024
	}
	qubits, gates, err := toIonQQIS(qasm3, numQubits)
	if err != nil {
		return nil, fmt.Errorf("ionq: %w", err)
	}
	base := ionqBase
	if cfg.BaseURL != "" {
		base = cfg.BaseURL
	}

	jobID, err := ionqSubmit(base, cfg.APIKey, cfg.Device, qubits, gates, shots, cfg.Extra)
	if err != nil {
		return nil, fmt.Errorf("ionq: submit: %w", err)
	}
	notifySubmitted(cfg.OnSubmitted, jobID)
	fmt.Printf("  IonQ job submitted: %s\n", jobID)

	if err := ionqPoll(base, cfg.APIKey, jobID); err != nil {
		return nil, fmt.Errorf("ionq: %w", err)
	}

	counts, err := ionqResults(base, cfg.APIKey, jobID, shots, qubits)
	if err != nil {
		return nil, fmt.Errorf("ionq: results: %w", err)
	}

	return &RunResult{
		JobID:   jobID,
		Backend: "IonQ / " + cfg.Device,
		Shots:   shots,
		Counts:  counts,
	}, nil
}

func ionqSubmit(base, apiKey, device string, qubits int, gates []ionqGate, shots int, extra map[string]string) (string, error) {
	payload := map[string]any{}
	mergeExtra(payload, extra)
	payload["type"] = "ionq.circuit.v1"
	payload["backend"] = device
	payload["shots"] = shots
	payload["input"] = map[string]any{
		"qubits":  qubits,
		"gateset": "qis",
		"circuit": gates,
	}
	body, _ := json.Marshal(payload)

	resp, err := ionqDo("POST", base+"/jobs", apiKey, body)
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

func ionqPoll(base, apiKey, jobID string) error {
	url := fmt.Sprintf("%s/jobs/%s", base, jobID)
	return pollUntil("ionq", jobID, func() (pollTick, error) {
		resp, err := ionqDo("GET", url, apiKey, nil)
		if err != nil {
			return pollTick{}, err
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return pollTick{}, readErr
		}
		tick, err := ionqJobTick(raw)
		if err != nil {
			return pollTick{}, err
		}
		return tick, nil
	})
}

// ionqJobTick reads the job document IonQ shows for a circuit run.
// status "ready" means the circuit was accepted and is waiting; it is not a failure.
// The console renders type "circuit" and target "simulator" even though submit uses
// type "ionq.circuit.v1" and backend "simulator".
func ionqJobTick(raw []byte) (pollTick, error) {
	var job struct {
		Status  string          `json:"status"`
		Failure json.RawMessage `json:"failure"`
	}
	if err := json.Unmarshal(raw, &job); err != nil || strings.TrimSpace(job.Status) == "" {
		return pollTick{}, &ProviderError{Provider: "ionq", Class: ClassInvalidRequest, Message: "malformed job status"}
	}
	status := strings.ToLower(strings.TrimSpace(job.Status))
	switch status {
	case "completed":
		return pollTick{Done: true, Status: status}, nil
	case "failed", "canceled", "cancelled":
		return pollTick{Failed: true, Status: status, Message: ionqFailureText(job.Failure)}, nil
	case "submitted", "ready", "started", "running", "queued":
		return pollTick{Status: status}, nil
	default:
		return pollTick{Status: status}, nil
	}
}

func ionqFailureText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var obj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil && (obj.Message != "" || obj.Code != "") {
		if obj.Code == "" {
			return obj.Message
		}
		if obj.Message == "" {
			return obj.Code
		}
		return obj.Code + ": " + obj.Message
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return string(raw)
}

// ionqResults fetches the completed job's probability histogram and
// converts it to shot counts. IonQ keys the histogram by the decimal value
// of the qubit register where bit i corresponds to qubit i (LSB = qubit 0).
// We render qubit i at bitstring position i to match the convention used
// elsewhere in this package (ibm.go, google.go): position 0 = qubit 0.
func ionqResults(base, apiKey, jobID string, shots, numQubits int) (map[string]int, error) {
	url := fmt.Sprintf("%s/jobs/%s/results/probabilities", base, jobID)
	resp, err := ionqDo("GET", url, apiKey, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	counts, err := ionqCounts(raw, shots, numQubits)
	if err != nil {
		return nil, fmt.Errorf("decode results: %w (body: %s)", err, string(raw))
	}
	return counts, nil
}

// ionqCounts converts IonQ's probability map into shot counts.
// Keys are decimal states where bit i is qubit i, so 0 is 00 and 3 is 11.
func ionqCounts(raw []byte, shots, numQubits int) (map[string]int, error) {
	var probs map[string]float64
	if err := json.Unmarshal(raw, &probs); err != nil {
		return nil, err
	}
	if len(probs) == 0 {
		return nil, fmt.Errorf("probability map is empty")
	}

	counts := make(map[string]int, len(probs))
	for stateStr, p := range probs {
		state, err := strconv.ParseInt(stateStr, 10, 64)
		if err != nil {
			continue
		}
		bits := make([]byte, numQubits)
		for i := 0; i < numQubits; i++ {
			if state&(1<<uint(i)) != 0 {
				bits[i] = '1'
			} else {
				bits[i] = '0'
			}
		}
		counts[string(bits)] = int(math.Round(p * float64(shots)))
	}
	if len(counts) == 0 {
		return nil, fmt.Errorf("probability map has no numeric states")
	}
	return counts, nil
}

// CancelIonQ asks IonQ Cloud to cancel an in-flight job.
func CancelIonQ(cfg *config.IonQConfig, providerJobID string) error {
	if cfg == nil || cfg.APIKey == "" {
		return fmt.Errorf("ionq: api_key is required to cancel")
	}
	if providerJobID == "" {
		return fmt.Errorf("ionq: provider job id is required to cancel")
	}
	base := ionqBase
	if cfg.BaseURL != "" {
		base = cfg.BaseURL
	}
	resp, err := ionqDo("PUT", base+"/jobs/"+providerJobID+"/status/cancel", cfg.APIKey, nil)
	if err != nil {
		return fmt.Errorf("ionq: cancel: %w", err)
	}
	resp.Body.Close()
	return nil
}

func ionqDo(method, url, apiKey string, body []byte) (*http.Response, error) {
	return doJSON("ionq", method, url, "apiKey "+apiKey, nil, body)
}
