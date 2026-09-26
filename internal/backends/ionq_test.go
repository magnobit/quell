// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnobit/quell/internal/config"
)

func TestRunIonQ_MissingAPIKeyRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{Device: "simulator", BaseURL: srv.URL}
	_, err := RunIonQ(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected an error for missing api_key")
	}
	if called {
		t.Error("no HTTP call should have been made before validating api_key")
	}
}

func TestRunIonQ_MissingDeviceRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "key", BaseURL: srv.URL}
	_, err := RunIonQ(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected an error for missing device")
	}
	if called {
		t.Error("no HTTP call should have been made before validating device")
	}
}

func TestRunIonQ_KnownGoodFixture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "ionq-job-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-job-1":
			w.Write([]byte(`{"status": "completed"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-job-1/results/probabilities":
			// state "0" (binary 00) and state "3" (binary 11) each at 50%,
			// 4 shots -> 2 each.
			w.Write([]byte(`{"0": 0.5, "3": 0.5}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "key", Device: "simulator", Shots: 4, BaseURL: srv.URL}
	got, err := RunIonQ(cfg, "OPENQASM 3;", 2)
	if err != nil {
		t.Fatalf("RunIonQ: %v", err)
	}
	if got.JobID != "ionq-job-1" {
		t.Errorf("JobID = %q, want ionq-job-1 (extracted from submit response)", got.JobID)
	}
	if got.Counts["00"] != 2 || got.Counts["11"] != 2 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want {00:2, 11:2}", got.Counts)
	}
}

func TestRunIonQ_JobFailedSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "ionq-fail-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-fail-1":
			w.Write([]byte(`{"status": "failed", "failure": {"message": "queue timeout"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "key", Device: "simulator", BaseURL: srv.URL}
	_, err := RunIonQ(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error when the job reaches failed status")
	}
}

func TestRunIonQ_JobCanceledSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "ionq-cancel-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-cancel-1":
			w.Write([]byte(`{"status": "canceled"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "key", Device: "simulator", BaseURL: srv.URL}
	_, err := RunIonQ(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error when the job reaches canceled status")
	}
}

func TestRunIonQ_SubmitHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "quota exceeded"}`))
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "key", Device: "simulator", BaseURL: srv.URL}
	_, err := RunIonQ(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}

func TestCancelIonQ_PutsCanceledStatus(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "key", Device: "simulator", BaseURL: srv.URL}
	if err := CancelIonQ(cfg, "ionq-job-9"); err != nil {
		t.Fatalf("CancelIonQ: %v", err)
	}
	if gotMethod != "PUT" || gotPath != "/jobs/ionq-job-9/status/cancel" {
		t.Fatalf("cancel request = %s %s, want PUT /jobs/ionq-job-9/status/cancel", gotMethod, gotPath)
	}
}

func TestRunIonQ_SubmitsQISCircuit(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("body: %v", err)
			}
			w.Write([]byte(`{"id": "ionq-job-qis"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-job-qis":
			w.Write([]byte(`{"status": "completed"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-job-qis/results/probabilities":
			w.Write([]byte(`{"0": 1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	const bell = `OPENQASM 3.0;
include "stdgates.inc";
qubit[2] q;
bit[2] c;
h q[0];
cx q[0], q[1];
c[0] = measure q[0];
c[1] = measure q[1];
`
	cfg := &config.IonQConfig{APIKey: "key", Device: "simulator", Shots: 8, BaseURL: srv.URL}
	if _, err := RunIonQ(cfg, bell, 2); err != nil {
		t.Fatalf("RunIonQ: %v", err)
	}
	if body["type"] != "ionq.circuit.v1" || body["backend"] != "simulator" {
		t.Fatalf("job header = %#v", body)
	}
	if _, ok := body["target"]; ok {
		t.Fatal("submit must not use the retired target field")
	}
	input, _ := body["input"].(map[string]any)
	if input["gateset"] != "qis" || input["qubits"] != float64(2) {
		t.Fatalf("input = %#v", input)
	}
	circuit, _ := input["circuit"].([]any)
	if len(circuit) != 2 {
		t.Fatalf("circuit = %#v", circuit)
	}
	h, _ := circuit[0].(map[string]any)
	cnot, _ := circuit[1].(map[string]any)
	if h["gate"] != "h" || h["target"] != float64(0) {
		t.Fatalf("h = %#v", h)
	}
	if cnot["gate"] != "cnot" || cnot["control"] != float64(0) || cnot["target"] != float64(1) {
		t.Fatalf("cnot = %#v", cnot)
	}
	if _, ok := input["format"]; ok {
		t.Fatal("input must not be an OpenQASM blob")
	}
}

func TestIonQJobTick_ReadyConsoleDocument(t *testing.T) {
	const body = `{
	  "id": "01a0dc2f-c705-7223-bf6d-5f8734d29ae4",
	  "submitted_by": "aa1d4748-031f-482c-a7dc-763aa5f1ae9c",
	  "status": "ready",
	  "target": "simulator",
	  "type": "circuit",
	  "qubits": 2,
	  "circuits": 1,
	  "dry_run": false,
	  "cost_model": "2QGE_operations",
	  "gate_counts": {"1q": 1, "2q": 1},
	  "project_id": "ff8af893-61c0-4be8-b352-bba4fadf966b",
	  "request": 1790400513,
	  "noise": {"model": "ideal"},
	  "error_mitigation": {"debias": false},
	  "children": []
	}`
	tick, err := ionqJobTick([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if tick.Done || tick.Failed || tick.Status != "ready" {
		t.Fatalf("ready job = %+v, want in-progress ready", tick)
	}
}

func TestIonQCounts_BellPairProbabilities(t *testing.T) {
	counts, err := ionqCounts([]byte(`{"0": 0.5, "3": 0.5}`), 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	if counts["00"] != 4 || counts["11"] != 4 || len(counts) != 2 {
		t.Fatalf("counts = %v, want 00:4 11:4", counts)
	}
}

func TestIonQJobTick_FailureCode(t *testing.T) {
	tick, err := ionqJobTick([]byte(`{"status":"failed","failure":{"code":"UnsupportedGate","message":"gate not supported"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !tick.Failed || tick.Message != "UnsupportedGate: gate not supported" {
		t.Fatalf("failure tick = %+v", tick)
	}
}

func TestToIonQQIS_RotationIsTurns(t *testing.T) {
	const src = "OPENQASM 3.0;\nqubit[1] q;\nrx(pi) q[0];\n"
	_, gates, err := toIonQQIS(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(gates) != 1 || gates[0].Gate != "rx" || gates[0].Rotation == nil || *gates[0].Rotation != 0.5 {
		t.Fatalf("gates = %#v, want rx rotation 0.5 turns", gates)
	}
}
