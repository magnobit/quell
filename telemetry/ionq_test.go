// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIonQTelemetryClient_ParsesKnownGoodResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backends/qpu.test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{
			"qubits": 36,
			"average_queue_time": 120,
			"native_gate_set": ["gpi", "gpi2", "ms"],
			"last_calibrated": "2026-08-20T00:00:00Z",
			"average_fidelity": 0.995
		}`))
	}))
	defer srv.Close()

	c := &IonQTelemetryClient{APIKey: "test-key", Device: "qpu.test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}

	if got.Source != "provider_api" {
		t.Errorf("Source = %q, want provider_api", got.Source)
	}
	if got.Qubits == nil || *got.Qubits != 36 {
		t.Errorf("Qubits = %v, want 36", got.Qubits)
	}
	if got.QueueWaitSeconds == nil || *got.QueueWaitSeconds != 120 {
		t.Errorf("QueueWaitSeconds = %v, want 120", got.QueueWaitSeconds)
	}
	if len(got.NativeGates) != 3 {
		t.Errorf("NativeGates = %v, want 3 entries", got.NativeGates)
	}
	if got.AverageFidelity == nil || *got.AverageFidelity != 0.995 {
		t.Fatalf("AverageFidelity = %v, want raw provider 0.995", got.AverageFidelity)
	}
	if got.TwoQubitError == nil {
		t.Fatalf("TwoQubitError should be derived from average_fidelity")
	}
	if want := 0.005; *got.TwoQubitError < want-1e-9 || *got.TwoQubitError > want+1e-9 {
		t.Errorf("TwoQubitError = %v, want %v (1 - average_fidelity)", *got.TwoQubitError, want)
	}
	if !got.TwoQubitErrorDerived || got.TwoQubitErrorDerivation != "1 - average_fidelity" {
		t.Errorf("derived proxy not labeled: derived=%v formula=%q", got.TwoQubitErrorDerived, got.TwoQubitErrorDerivation)
	}
	if got.ReadoutError != nil || len(got.QubitsCal) != 0 {
		t.Errorf("IonQ must not fabricate readout, got readout=%v qubits=%+v", got.ReadoutError, got.QubitsCal)
	}
	if got.CalibratedAt.IsZero() {
		t.Errorf("CalibratedAt should be parsed from last_calibrated")
	}
}

func TestIonQTelemetryClient_MissingFieldsStayNilNotFabricated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &IonQTelemetryClient{APIKey: "test-key", Device: "qpu.test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if got.Qubits != nil || got.QueueWaitSeconds != nil || got.TwoQubitError != nil {
		t.Errorf("expected all optional fields nil for an empty response, got %+v", got)
	}
}

func TestIonQTelemetryClient_RequestFailureIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &IonQTelemetryClient{APIKey: "bad-key", Device: "qpu.test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err == nil {
		t.Errorf("expected an error when the telemetry endpoint fails")
	}
	if got.FailKind != "auth" {
		t.Errorf("FailKind = %q, want auth", got.FailKind)
	}
}

func TestIonQTelemetryClient_ExplicitZeroQueueIsKnown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"qubits": 8, "average_queue_time": 0}`))
	}))
	defer srv.Close()

	c := &IonQTelemetryClient{APIKey: "k", Device: "qpu.test", BaseURL: srv.URL}
	got, err := c.FetchTelemetry(context.Background())
	if err != nil {
		t.Fatalf("FetchTelemetry: %v", err)
	}
	if got.QueueWaitSeconds == nil || *got.QueueWaitSeconds != 0 {
		t.Errorf("QueueWaitSeconds = %v, want known 0 (not unknown)", got.QueueWaitSeconds)
	}
	if got.TwoQubitError != nil || got.AverageFidelity != nil {
		t.Errorf("missing fidelity must stay unknown, got %+v", got)
	}
}
