// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIBMTelemetryClient_ParsesKnownGoodResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/runtime/backends/ibm_test/configuration":
			w.Write([]byte(`{"n_qubits": 27, "basis_gates": ["cx", "id", "rz", "sx", "x"], "coupling_map": [[0,1],[1,0],[1,2],[2,1]]}`))
		case r.URL.Path == "/runtime/backends/ibm_test/properties":
			w.Write([]byte(`{
				"last_update_date": "2026-08-20T12:00:00Z",
				"qubits": [
					[
						{"name": "readout_error", "value": 0.02},
						{"name": "T1", "value": 100, "unit": "us"},
						{"name": "T2", "value": 80, "unit": "us"},
						{"name": "prob_meas0_prep1", "value": 0.03},
						{"name": "prob_meas1_prep0", "value": 0.01}
					],
					[
						{"name": "readout_error", "value": 0.04},
						{"name": "T1", "value": 50, "unit": "us"},
						{"name": "T2", "value": 40, "unit": "us"},
						{"name": "prob_meas0_prep1", "value": 0.05},
						{"name": "prob_meas1_prep0", "value": 0.02}
					]
				],
				"gates": [
					{"gate": "sx", "qubits": [0], "parameters": [{"name": "gate_error", "value": 0.001}]},
					{"gate": "cx", "qubits": [0, 1], "parameters": [{"name": "gate_error", "value": 0.01}]}
				]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "test-token", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}

	if got.Source != "provider_api" {
		t.Errorf("Source = %q, want provider_api", got.Source)
	}
	if got.Qubits == nil || *got.Qubits != 27 {
		t.Errorf("Qubits = %v, want 27", got.Qubits)
	}
	if len(got.NativeGates) != 5 {
		t.Errorf("NativeGates = %v, want 5 entries", got.NativeGates)
	}
	if got.ReadoutError == nil || *got.ReadoutError <= 0.02 || *got.ReadoutError >= 0.04 {
		t.Errorf("ReadoutError = %v, want the average of 0.02 and 0.04", got.ReadoutError)
	}
	if got.SingleQubitError == nil || *got.SingleQubitError != 0.001 {
		t.Errorf("SingleQubitError = %v, want 0.001", got.SingleQubitError)
	}
	if got.TwoQubitError == nil || *got.TwoQubitError != 0.01 {
		t.Errorf("TwoQubitError = %v, want 0.01", got.TwoQubitError)
	}
	if got.CalibratedAt.IsZero() {
		t.Errorf("CalibratedAt should be parsed from last_update_date")
	}
	if len(got.QubitsCal) != 2 {
		t.Fatalf("QubitsCal = %d, want 2", len(got.QubitsCal))
	}
	if got.QubitsCal[0].ReadoutError == nil || *got.QubitsCal[0].ReadoutError != 0.02 {
		t.Errorf("qubit 0 readout = %v, want 0.02", got.QubitsCal[0].ReadoutError)
	}
	if got.QubitsCal[0].T1Seconds == nil || *got.QubitsCal[0].T1Seconds < 99.9e-6 || *got.QubitsCal[0].T1Seconds > 100.1e-6 {
		t.Errorf("qubit 0 T1 = %v, want 100us in seconds", got.QubitsCal[0].T1Seconds)
	}
	if !got.QubitsCal[0].HasAssignmentMatrix || got.QubitsCal[0].P0Given1 == nil || *got.QubitsCal[0].P0Given1 != 0.03 {
		t.Errorf("qubit 0 assignment = %+v", got.QubitsCal[0])
	}
	if len(got.GatesCal) != 2 || got.GatesCal[1].Gate != "cx" || len(got.GatesCal[1].Qubits) != 2 {
		t.Errorf("GatesCal = %+v, want sx + cx with qubit scope", got.GatesCal)
	}
	wantCoupling := [][2]int{{0, 1}, {1, 0}, {1, 2}, {2, 1}}
	if len(got.CouplingMap) != len(wantCoupling) {
		t.Fatalf("CouplingMap = %v, want %v", got.CouplingMap, wantCoupling)
	}
	for i, pair := range wantCoupling {
		if got.CouplingMap[i] != pair {
			t.Errorf("CouplingMap[%d] = %v, want %v", i, got.CouplingMap[i], pair)
		}
	}
}

func TestIBMTelemetryClient_MissingFieldsStayNilNotFabricated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both endpoints return valid-but-empty JSON — simulates a real
		// response that just doesn't carry the fields we look for.
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "test-token", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if got.CouplingMap != nil {
		t.Errorf("CouplingMap = %v, want nil when the field is absent — never fabricate", got.CouplingMap)
	}
	if got.Qubits != nil {
		t.Errorf("Qubits = %v, want nil when the field is absent — never fabricate", got.Qubits)
	}
	if got.ReadoutError != nil || got.SingleQubitError != nil || got.TwoQubitError != nil {
		t.Errorf("error-rate fields should stay nil when absent, got %+v", got)
	}
}

func TestIBMTelemetryClient_BothEndpointsFailingIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "bad-token", Device: "ibm_test", BaseURL: srv.URL}
	_, err := c.FetchTelemetry(context.Background())
	if err == nil {
		t.Errorf("expected an error when every telemetry endpoint fails")
	}
}

func TestIBMTelemetryClient_PartialFailureIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime/backends/ibm_test/configuration" {
			w.Write([]byte(`{"n_qubits": 5, "basis_gates": ["cx", "x"]}`))
			return
		}
		// properties endpoint is down/unauthorized — configuration alone
		// should still be treated as a successful (partial) fetch.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "test-token", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("a partial success (one of two endpoints) should not error, got: %v", err)
	}
	if got.Qubits == nil || *got.Qubits != 5 {
		t.Errorf("Qubits = %v, want 5 from the endpoint that did succeed", got.Qubits)
	}
	if got.ReadoutError != nil {
		t.Errorf("ReadoutError should stay nil since properties never returned")
	}
	if !got.Partial {
		t.Errorf("partial success should set Partial")
	}
}

func TestIBMTelemetryClient_ScalarReadoutIsNotAMatrix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime/backends/ibm_test/configuration" {
			w.Write([]byte(`{"n_qubits": 2}`))
			return
		}
		w.Write([]byte(`{
			"last_update_date": "2026-08-20T12:00:00Z",
			"qubits": [
				[{"name": "readout_error", "value": 0.02}],
				[{"name": "readout_error", "value": 0.04}]
			]
		}`))
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "t", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if got.ReadoutError == nil {
		t.Fatalf("expected backend-average readout")
	}
	if len(got.QubitsCal) != 2 {
		t.Fatalf("QubitsCal = %d, want 2 per-qubit scalars", len(got.QubitsCal))
	}
	for i, q := range got.QubitsCal {
		if q.HasAssignmentMatrix || q.P0Given1 != nil || q.P1Given0 != nil {
			t.Errorf("qubit %d: scalar readout must not become an assignment matrix: %+v", i, q)
		}
	}
}

func TestIBMTelemetryClient_RejectsInvalidNumericValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime/backends/ibm_test/configuration" {
			w.Write([]byte(`{"n_qubits": 1, "coupling_map": [[0,-1]]}`))
			return
		}
		w.Write([]byte(`{
			"last_update_date": "not-a-date",
			"qubits": [[{"name": "readout_error", "value": 1.5}, {"name": "T1", "value": -3, "unit": "us"}]],
			"gates": [{"gate": "sx", "qubits": [0], "parameters": [{"name": "gate_error", "value": "nan"}]}]
		}`))
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "t", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("partial/malformed properties should not fail the fetch: %v", err)
	}
	if got.ReadoutError != nil {
		t.Errorf("invalid probability 1.5 must not be ingested, got %v", got.ReadoutError)
	}
	if len(got.QubitsCal) != 0 {
		t.Errorf("invalid T1/readout must not persist, got %+v", got.QubitsCal)
	}
	if got.CalibratedAt.IsZero() == false && got.CalibratedAt.Year() < 2000 {
		t.Errorf("invalid timestamp should stay zero, got %v", got.CalibratedAt)
	}
	if got.CouplingMap != nil {
		t.Errorf("invalid coupling edge must not persist, got %v", got.CouplingMap)
	}
}

func TestIBMTelemetryClient_OneSidedAssignmentIsNotAMatrix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "configuration") {
			w.Write([]byte(`{"n_qubits": 1}`))
			return
		}
		w.Write([]byte(`{"qubits": [[{"name": "readout_error", "value": 0.02}, {"name": "prob_meas0_prep1", "value": 0.03}]]}`))
	}))
	defer srv.Close()
	c := &IBMTelemetryClient{Token: "t", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if len(got.QubitsCal) != 1 || got.QubitsCal[0].HasAssignmentMatrix {
		t.Fatalf("one-sided assignment must not become a matrix: %+v", got.QubitsCal)
	}
}

func TestIBMTelemetryClient_SingularAssignmentRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "configuration") {
			w.Write([]byte(`{"n_qubits": 1}`))
			return
		}
		w.Write([]byte(`{"qubits": [[{"name": "prob_meas0_prep1", "value": 0.5}, {"name": "prob_meas1_prep0", "value": 0.5}]]}`))
	}))
	defer srv.Close()
	c := &IBMTelemetryClient{Token: "t", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if len(got.QubitsCal) != 0 {
		t.Fatalf("singular assignment must not persist: %+v", got.QubitsCal)
	}
}

func TestIBMTelemetryClient_PartialPropertiesKeepValidFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "configuration") {
			w.Write([]byte(`{"n_qubits": 2, "coupling_map": [[0,1],[1,0]]}`))
			return
		}
		w.Write([]byte(`{"last_update_date": "2026-09-13T12:00:00Z", "qubits": [[{"name": "T1", "value": 80, "unit": "us"}]]}`))
	}))
	defer srv.Close()
	c := &IBMTelemetryClient{Token: "t", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if len(got.CouplingMap) != 2 {
		t.Fatalf("topology should persist from configuration: %v", got.CouplingMap)
	}
	if len(got.QubitsCal) != 1 || got.QubitsCal[0].T1Seconds == nil || got.QubitsCal[0].T2Seconds != nil || got.QubitsCal[0].ReadoutError != nil {
		t.Fatalf("only valid T1 should persist, got %+v", got.QubitsCal)
	}
	if got.ReadoutError != nil {
		t.Fatal("missing readout must stay unknown, not fabricated")
	}
}

func TestIBMTelemetryClient_AuthFailureKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "bad", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if got.FailKind != "auth" {
		t.Errorf("FailKind = %q, want auth", got.FailKind)
	}
}

func TestIBMTelemetryClient_PermissionFailureKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := &IBMTelemetryClient{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got.FailKind != "permission" {
		t.Errorf("FailKind = %q, want permission", got.FailKind)
	}
}
