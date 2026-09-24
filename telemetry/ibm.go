// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// errAllTelemetryRequestsFailed is returned only when every telemetry
// request for a backend failed outright (e.g. bad token, network down) —
// a partial result (one endpoint succeeded, the other didn't) is not an
// error, it's just a BackendTelemetry with fewer fields populated.
var errAllTelemetryRequestsFailed = errors.New("telemetry: no telemetry endpoint returned data")

// ibmTelemetryBase is deliberately its own constant rather than importing
// quell/internal/backends' ibmBase: this package needs to be importable
// from outside the quell module (qubitlabs-platform's scheduler consumes
// it), and internal/ packages can't be — see this package's doc comment.
const ibmTelemetryBase = "https://api.quantum.ibm.com"

// IBMTelemetryClient fetches live backend characteristics from IBM Quantum's
// configuration + properties endpoints — entirely separate from
// quell/internal/backends' RunIBM (job submission/status/results). This is
// its own small client rather than an extension of RunIBM: telemetry
// fetching happens on a schedule independent of any job, and must never
// fail a job submission if the provider's calibration data is temporarily
// unavailable.
//
// Endpoint shapes: IBM's Qiskit Runtime REST API exposes
// GET /runtime/backends/{name}/configuration (n_qubits, basis_gates) and
// GET /runtime/backends/{name}/properties (per-qubit/per-gate calibration,
// classic Qiskit BackendProperties JSON shape: qubits[][{name,value}],
// gates[]{gate,qubits,parameters[]{name,value}}, last_update_date). Both are
// parsed defensively below — an unrecognized shape or a failed request
// leaves the corresponding fields nil rather than erroring the whole fetch.
// Verify these paths against current IBM docs against a real account before
// relying on this in production; the parsing logic itself is covered by
// fixture-based tests independent of that.
type IBMTelemetryClient struct {
	Token   string
	Device  string
	BaseURL string // defaults to ibmTelemetryBase when empty; overridable for tests
}

type qubitParam struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

func (c *IBMTelemetryClient) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return ibmTelemetryBase
}

// FetchTelemetry never returns an error for a partial result — a nil field
// means "this provider call didn't give us this," not "this value is
// zero." It only returns an error if both requests fail outright (e.g. bad
// token), since then there's nothing at all to report.
func (c *IBMTelemetryClient) FetchTelemetry(ctx context.Context) (BackendTelemetry, error) {
	out := BackendTelemetry{Source: "provider_api"}

	cfgOK := c.fetchConfiguration(ctx, &out)
	propsOK := c.fetchProperties(ctx, &out)

	if !cfgOK && !propsOK {
		if out.FailKind == "" {
			out.FailKind = "network"
		}
		return out, errAllTelemetryRequestsFailed
	}
	if !cfgOK || !propsOK {
		out.Partial = true
	}
	return out, nil
}

func (c *IBMTelemetryClient) fetchConfiguration(ctx context.Context, out *BackendTelemetry) bool {
	var cfg struct {
		NumQubits   int      `json:"n_qubits"`
		BasisGates  []string `json:"basis_gates"`
		CouplingMap [][2]int `json:"coupling_map"` // Qiskit BackendConfiguration's real field name/shape: [[control, target], ...], usually listed both directions
	}
	ok, kind := c.getJSON(ctx, c.baseURL()+"/runtime/backends/"+c.Device+"/configuration", &cfg)
	if !ok {
		if kind != "" {
			out.FailKind = kind
		}
		return false
	}
	if cfg.NumQubits > 0 {
		out.Qubits = &cfg.NumQubits
	}
	if len(cfg.BasisGates) > 0 {
		out.NativeGates = cfg.BasisGates
	}
	if len(cfg.CouplingMap) > 0 {
		var pairs [][2]int
		for _, pair := range cfg.CouplingMap {
			if !validQubitIndex(pair[0]) || !validQubitIndex(pair[1]) {
				continue
			}
			pairs = append(pairs, pair)
		}
		if len(pairs) > 0 {
			out.CouplingMap = pairs
		}
	}
	return true
}

func (c *IBMTelemetryClient) fetchProperties(ctx context.Context, out *BackendTelemetry) bool {
	var props struct {
		LastUpdateDate string         `json:"last_update_date"`
		Qubits         [][]qubitParam `json:"qubits"`
		Gates          []struct {
			Gate       string       `json:"gate"`
			Qubits     []int        `json:"qubits"`
			Parameters []qubitParam `json:"parameters"`
		} `json:"gates"`
	}
	ok, kind := c.getJSON(ctx, c.baseURL()+"/runtime/backends/"+c.Device+"/properties", &props)
	if !ok {
		if kind != "" {
			out.FailKind = kind
		}
		return false
	}

	if t := parseRFC3339(props.LastUpdateDate); !t.IsZero() {
		out.CalibratedAt = t
	}

	var readoutErrs []float64
	for i, params := range props.Qubits {
		if i >= maxPersistedQubits {
			break
		}
		qc := QubitCalibration{Qubit: i}
		if v := namedError(params, "readout_error"); v != nil {
			qc.ReadoutError = v
			readoutErrs = append(readoutErrs, *v)
		}
		if v := namedCoherence(params, "T1"); v != nil {
			qc.T1Seconds = v
		}
		if v := namedCoherence(params, "T2"); v != nil {
			qc.T2Seconds = v
		}
		p01 := namedError(params, "prob_meas0_prep1")
		p10 := namedError(params, "prob_meas1_prep0")
		// Both directional assignment probabilities must be present on
		// this same qubit. A scalar readout_error is never promoted.
		if p01 != nil && p10 != nil && ValidAssignmentPair(*p01, *p10) {
			qc.P0Given1 = p01
			qc.P1Given0 = p10
			qc.HasAssignmentMatrix = true
		}
		if qc.ReadoutError != nil || qc.T1Seconds != nil || qc.T2Seconds != nil || qc.HasAssignmentMatrix {
			out.QubitsCal = append(out.QubitsCal, qc)
		}
	}
	out.ReadoutError = average(readoutErrs)

	var oneQErrs, twoQErrs []float64
	for _, g := range props.Gates {
		if len(out.GatesCal) >= maxPersistedGates {
			break
		}
		if !validGateQubits(g.Qubits) {
			continue
		}
		errVal := namedError(g.Parameters, "gate_error")
		out.GatesCal = append(out.GatesCal, GateCalibration{
			Gate:   g.Gate,
			Qubits: append([]int(nil), g.Qubits...),
			Error:  errVal,
		})
		if errVal == nil {
			continue
		}
		if len(g.Qubits) >= 2 {
			twoQErrs = append(twoQErrs, *errVal)
		} else {
			oneQErrs = append(oneQErrs, *errVal)
		}
	}
	out.SingleQubitError = average(oneQErrs)
	out.TwoQubitError = average(twoQErrs)
	return true
}

func namedParam(params []qubitParam, name string) *qubitParam {
	for i := range params {
		if params[i].Name == name {
			return &params[i]
		}
	}
	return nil
}

func namedError(params []qubitParam, name string) *float64 {
	p := namedParam(params, name)
	if p == nil {
		return nil
	}
	return normalizeError(p.Value, p.Unit)
}

func namedCoherence(params []qubitParam, name string) *float64 {
	p := namedParam(params, name)
	if p == nil {
		return nil
	}
	unit := p.Unit
	if unit == "" {
		// IBM BackendProperties historically report T1/T2 in microseconds
		// when unit is omitted on older payloads.
		unit = "us"
	}
	return normalizeTimeToSeconds(p.Value, unit)
}

func validGateQubits(qubits []int) bool {
	if len(qubits) == 0 {
		return false
	}
	for _, q := range qubits {
		if !validQubitIndex(q) {
			return false
		}
	}
	return true
}

func average(vals []float64) *float64 {
	if len(vals) == 0 {
		return nil
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	avg := sum / float64(len(vals))
	return &avg
}

// getJSON returns (ok, failKind). A failed request is "this endpoint
// didn't contribute," not a hard failure of the whole fetch.
func (c *IBMTelemetryClient) getJSON(ctx context.Context, url string, dst any) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, classifyNetErr(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("IBM-API-Version", "2024-06-14")

	resp, err := telemetryHTTPClient.Do(req)
	if err != nil {
		return false, classifyNetErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, classifyHTTPStatus(resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return false, "malformed"
	}
	return true, ""
}
