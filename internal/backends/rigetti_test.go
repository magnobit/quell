// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/internal/targetfmt"
)

func TestRunRigetti_MissingAPIKeyRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{Device: "Aspen-M-3", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing api_key")
	}
	if called {
		t.Error("no HTTP call should have been made before validating api_key")
	}
}

func TestRunRigetti_MissingDeviceRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing device")
	}
	if called {
		t.Error("no HTTP call should have been made before validating device")
	}
}

func TestRunRigetti_KnownGoodFixture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "rig-job-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-job-1":
			w.Write([]byte(`{"status": "COMPLETED"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-job-1/results":
			w.Write([]byte(`{"counts": {"00": 6, "11": 4}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	got, err := RunRigetti(cfg, "OPENQASM 3;")
	if err != nil {
		t.Fatalf("RunRigetti: %v", err)
	}
	if got.JobID != "rig-job-1" {
		t.Errorf("JobID = %q, want rig-job-1 (extracted from submit response)", got.JobID)
	}
	if got.Counts["00"] != 6 || got.Counts["11"] != 4 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want {00:6, 11:4}", got.Counts)
	}
}

func TestRunRigetti_SubmitsSharedQuil(t *testing.T) {
	bell := "OPENQASM 3;\nqubit[2] q;\nbit[2] c;\nh q[0];\ncx q[0], q[1];\nc = measure q;\n"
	want, err := targetfmt.RigettiQuil(bell)
	if err != nil {
		t.Fatal(err)
	}
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &posted)
			w.Write([]byte(`{"id": "rig-quil-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-quil-1":
			w.Write([]byte(`{"status": "COMPLETED"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-quil-1/results":
			w.Write([]byte(`{"ro":[[0,0],[1,1]]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	got, err := RunRigetti(cfg, bell)
	if err != nil {
		t.Fatalf("RunRigetti: %v", err)
	}
	program, _ := posted["program"].(map[string]any)
	if program["format"] != "quil" || program["source"] != want {
		t.Fatalf("program = %#v, want format quil and the shared translator output", program)
	}
	if !strings.Contains(want, "H 0") || !strings.Contains(want, "CNOT 0 1") {
		t.Fatalf("quil = %s", want)
	}
	if got.Counts["00"] != 1 || got.Counts["11"] != 1 {
		t.Fatalf("counts = %v", got.Counts)
	}
}

func TestRunRigetti_OpenQASM3SwitchSendsSource(t *testing.T) {
	src := "OPENQASM 3;\nqubit[2] q;\nbit[2] c;\nh q[0];\ncx q[0], q[1];\nccx q[0], q[1], q[0];\n"
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &posted)
			w.Write([]byte(`{"id": "rig-qasm-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-qasm-1":
			w.Write([]byte(`{"status": "COMPLETED"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-qasm-1/results":
			w.Write([]byte(`{"counts": {"00": 1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{
		APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL,
		Extra: map[string]string{"program_format": "openqasm3", "note": "kept"},
	}
	if _, err := RunRigetti(cfg, src); err != nil {
		t.Fatalf("RunRigetti: %v", err)
	}
	program, _ := posted["program"].(map[string]any)
	if program["format"] != "openqasm3" || program["source"] != src {
		t.Fatalf("program = %#v", program)
	}
	if _, leaked := posted["program_format"]; leaked {
		t.Fatal("program_format must select the body, not ride along as a job field")
	}
	if posted["note"] != "kept" {
		t.Fatalf("other extra fields = %#v", posted)
	}
}

func TestRunRigetti_RejectsUnknownProgramFormat(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL, ProgramFormat: "qir"}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil || !strings.Contains(err.Error(), "openqasm3") {
		t.Fatalf("got %v", err)
	}
	if called {
		t.Fatal("an unknown format must not be submitted")
	}
}

func TestRunRigetti_RejectsUntranslatedGateBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 2.0;\nqreg q[2];\nccx q[0], q[1], q[0];\n")
	if err == nil || !strings.Contains(err.Error(), "not translated") {
		t.Fatalf("got %v", err)
	}
	if called {
		t.Fatal("an untranslated circuit must not be submitted")
	}
}

func TestRunRigetti_JobFailedSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "rig-fail-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-fail-1":
			w.Write([]byte(`{"status": "FAILED", "error": "device unreachable"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the job reaches FAILED status")
	}
}

func TestRunRigetti_JobCancelledSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "rig-cancel-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-cancel-1":
			w.Write([]byte(`{"status": "CANCELLED"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the job reaches CANCELLED status")
	}
}

func TestRunRigetti_SubmitHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}

func TestRunRigetti_UnrecognisedResultFormatSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/jobs":
			w.Write([]byte(`{"id": "rig-weird-1"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-weird-1":
			w.Write([]byte(`{"status": "COMPLETED"}`))
		case r.Method == "GET" && r.URL.Path == "/jobs/rig-weird-1/results":
			w.Write([]byte(`{"nothing": "recognisable"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	_, err := RunRigetti(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the result body has no counts field, not a silent success")
	}
}

func TestCancelRigetti_DeletesJob(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.RigettiConfig{APIKey: "key", Device: "Aspen-M-3", BaseURL: srv.URL}
	if err := CancelRigetti(cfg, "rig-job-9"); err != nil {
		t.Fatalf("CancelRigetti: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/jobs/rig-job-9" {
		t.Fatalf("cancel request = %s %s, want DELETE /jobs/rig-job-9", gotMethod, gotPath)
	}
}
