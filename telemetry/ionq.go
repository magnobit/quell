// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
)

// ionqTelemetryBase is its own constant rather than importing
// quell/internal/backends' ionqBase — see ibm.go's identical note.
const ionqTelemetryBase = "https://api.ionq.co/v0.3"

// IonQTelemetryClient fetches live backend characteristics from IonQ
// Cloud's backend-characterization endpoint — entirely separate from
// quell/internal/backends' RunIonQ (job submission/status/results).
//
// Endpoint shape: GET /v0.3/backends/{backend} returns per-backend fields
// including qubits, average_queue_time (seconds), status, and (where
// published) a native_gate_set/average fidelity figure. Parsed defensively:
// an unrecognized/missing field leaves that BackendTelemetry field nil.
// Verify this path against current IonQ docs against a real account before
// relying on this in production.
type IonQTelemetryClient struct {
	APIKey  string
	Device  string
	BaseURL string // defaults to ionqTelemetryBase when empty; overridable for tests
}

func (c *IonQTelemetryClient) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return ionqTelemetryBase
}

func (c *IonQTelemetryClient) FetchTelemetry(ctx context.Context) (BackendTelemetry, error) {
	out := BackendTelemetry{Source: "provider_api"}

	var resp struct {
		Qubits           int      `json:"qubits"`
		AverageQueueTime *float64 `json:"average_queue_time"` // seconds; pointer so 0 is known
		NativeGates      []string `json:"native_gate_set"`
		LastCalibrated   string   `json:"last_calibrated"`
		AverageFidelity  *float64 `json:"average_fidelity"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+"/backends/"+c.Device, nil)
	if err != nil {
		out.FailKind = classifyNetErr(err)
		return out, err
	}
	req.Header.Set("Authorization", "apiKey "+c.APIKey)
	req.Header.Set("Accept", "application/json")

	httpResp, err := telemetryHTTPClient.Do(req)
	if err != nil {
		out.FailKind = classifyNetErr(err)
		return out, errAllTelemetryRequestsFailed
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode >= 400 {
		out.FailKind = classifyHTTPStatus(httpResp.StatusCode)
		return out, errAllTelemetryRequestsFailed
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		out.FailKind = "malformed"
		return out, errAllTelemetryRequestsFailed
	}

	if resp.Qubits > 0 {
		q := resp.Qubits
		out.Qubits = &q
	}
	if len(resp.NativeGates) > 0 {
		out.NativeGates = resp.NativeGates
	}
	if resp.AverageQueueTime != nil && !math.IsNaN(*resp.AverageQueueTime) && !math.IsInf(*resp.AverageQueueTime, 0) && *resp.AverageQueueTime >= 0 {
		wait := int(*resp.AverageQueueTime)
		out.QueueWaitSeconds = &wait
	}
	if resp.AverageFidelity != nil {
		if fid := normalizeError(*resp.AverageFidelity, ""); fid != nil {
			out.AverageFidelity = fid
			// Derived proxy — not a provider-observed two-qubit gate error.
			errRate := 1 - *fid
			out.TwoQubitError = &errRate
			out.TwoQubitErrorDerived = true
			out.TwoQubitErrorDerivation = "1 - average_fidelity"
		}
	}
	if t := parseRFC3339(resp.LastCalibrated); !t.IsZero() {
		out.CalibratedAt = t
	}

	return out, nil
}
