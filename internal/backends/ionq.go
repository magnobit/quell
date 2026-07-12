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
	"time"

	"github.com/magnobit/quell/internal/config"
)

const ionqBase = "https://api.ionq.co/v0.3"

// RunIonQ submits a circuit to IonQ Cloud and returns measurement counts.
// cfg.APIKey is the IonQ API key; cfg.Device is the target name (e.g.
// "simulator" or "qpu.harmony").
func RunIonQ(cfg *config.IonQConfig, qasm3 string, numQubits int) (*RunResult, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("ionq: api_key is required (set ionq.api_key in quell.config.yml or IONQ_API_KEY env var)")
	}
	if cfg.Device == "" {
		return nil, fmt.Errorf("ionq: device is required (e.g. simulator, qpu.harmony)")
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1024
	}

	jobID, err := ionqSubmit(cfg.APIKey, cfg.Device, qasm3, shots, cfg.Extra)
	if err != nil {
		return nil, fmt.Errorf("ionq: submit: %w", err)
	}
	fmt.Printf("  IonQ job submitted: %s\n", jobID)

	if err := ionqPoll(cfg.APIKey, jobID); err != nil {
		return nil, fmt.Errorf("ionq: %w", err)
	}

	counts, err := ionqResults(cfg.APIKey, jobID, shots, numQubits)
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

func ionqSubmit(apiKey, device, qasm3 string, shots int, extra map[string]string) (string, error) {
	payload := map[string]any{
		"target": device,
		"shots":  shots,
		"input": map[string]any{
			"format": "openqasm",
			"data":   qasm3,
		},
	}
	mergeExtra(payload, extra)
	body, _ := json.Marshal(payload)

	resp, err := ionqDo("POST", ionqBase+"/jobs", apiKey, body)
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

func ionqPoll(apiKey, jobID string) error {
	url := fmt.Sprintf("%s/jobs/%s", ionqBase, jobID)
	for {
		resp, err := ionqDo("GET", url, apiKey, nil)
		if err != nil {
			return err
		}
		var r struct {
			Status  string `json:"status"`
			Failure struct {
				Message string `json:"message"`
			} `json:"failure"`
		}
		json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()

		switch r.Status {
		case "completed":
			fmt.Print("\n")
			return nil
		case "failed", "canceled":
			msg := r.Failure.Message
			if msg == "" {
				msg = r.Status
			}
			return fmt.Errorf("job %s: %s", jobID, msg)
		default:
			fmt.Printf("\r  IonQ job status: %-10s (job: %s)", r.Status, jobID)
			time.Sleep(4 * time.Second)
		}
	}
}

// ionqResults fetches the completed job's probability histogram and
// converts it to shot counts. IonQ keys the histogram by the decimal value
// of the qubit register where bit i corresponds to qubit i (LSB = qubit 0).
// We render qubit i at bitstring position i to match the convention used
// elsewhere in this package (ibm.go, google.go): position 0 = qubit 0.
func ionqResults(apiKey, jobID string, shots, numQubits int) (map[string]int, error) {
	url := fmt.Sprintf("%s/jobs/%s/results", ionqBase, jobID)
	resp, err := ionqDo("GET", url, apiKey, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var probs map[string]float64
	if err := json.Unmarshal(raw, &probs); err != nil {
		return nil, fmt.Errorf("decode results: %w (body: %s)", err, string(raw))
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
	return counts, nil
}

func ionqDo(method, url, apiKey string, body []byte) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "apiKey "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from IonQ: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}
