// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnobit/quell/internal/config"
)

func TestRunIBM_MissingTokenRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected an error for missing token")
	}
	if called {
		t.Error("no HTTP call should have been made before validating token")
	}
}

func TestRunIBM_MissingDeviceRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected an error for missing device")
	}
	if called {
		t.Error("no HTTP call should have been made before validating device")
	}
}

func TestRunIBM_KnownGoodFixture_CircuitRunnerFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-cr-1"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-cr-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-cr-1/results":
			w.Write([]byte(`{"results": [{"data": {"counts": {"0x0": 7, "0x3": 3}}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	got, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	if got.JobID != "job-cr-1" {
		t.Errorf("JobID = %q, want job-cr-1 (extracted from submit response)", got.JobID)
	}
	if got.Backend != "IBM Quantum / ibm_test" {
		t.Errorf("Backend = %q", got.Backend)
	}
	want := map[string]int{"00": 7, "11": 3}
	if len(got.Counts) != len(want) || got.Counts["00"] != 7 || got.Counts["11"] != 3 {
		t.Errorf("Counts = %v, want %v", got.Counts, want)
	}
}

func TestRunIBM_KnownGoodFixture_SamplerV2Format(t *testing.T) {
	// 2 qubits, 2 shots: shot0 -> qubit0=0,qubit1=1 ("01"); shot1 -> qubit0=1,qubit1=0 ("10").
	// Packed MSB-first, 1 byte per shot (bytesPerShot = ceil(2/8) = 1).
	packed := []byte{0x40, 0x80} // 0b0100_0000, 0b1000_0000
	b64 := base64.StdEncoding.EncodeToString(packed)

	respBody, _ := json.Marshal(map[string]any{
		"results": []map[string]any{
			{
				"data": map[string]any{
					"c": map[string]any{
						"array": b64,
						"shape": []int{2, 1},
					},
				},
				"metadata": map[string]any{"num_qubits": 2},
			},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-sv2-1"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-sv2-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-sv2-1/results":
			w.Write(respBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	got, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	want := map[string]int{"01": 1, "10": 1}
	if got.Counts["01"] != 1 || got.Counts["10"] != 1 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want %v (Sampler V2 BitArray format)", got.Counts, want)
	}
}

func TestRunIBM_JobFailedSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-fail-1"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-fail-1":
			w.Write([]byte(`{"status": "Failed", "error": {"message": "calibration drift"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error when the job reaches Failed status")
	}
}

func TestRunIBM_JobCancelledSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-cancel-1"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-cancel-1":
			w.Write([]byte(`{"status": "Cancelled"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error when the job reaches Cancelled status")
	}
}

func TestRunIBM_SubmitHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors": [{"message": "invalid token"}]}`))
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "bad-tok", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}

func TestRunIBM_UnrecognisedResultFormatSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-weird-1"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-weird-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-weird-1/results":
			w.Write([]byte(`{"nothing": "recognisable"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error for an unrecognised result format, not a silent success")
	}
}

func TestCancelIBM_PostsRuntimeCancel(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	if err := CancelIBM(cfg, "job-to-cancel"); err != nil {
		t.Fatalf("CancelIBM: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/runtime/jobs/job-to-cancel/cancel" {
		t.Fatalf("cancel request = %s %s, want POST /runtime/jobs/job-to-cancel/cancel", gotMethod, gotPath)
	}
}

func TestRunIBM_OnSubmittedFiresAfterSubmitBeforePoll(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-hook-1"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-hook-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/runtime/jobs/job-hook-1/results":
			w.Write([]byte(`{"results": [{"data": {"counts": {"0x0": 1}}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL, OnSubmitted: func(id string) {
		seen = append(seen, id)
	}}
	if _, err := RunIBM(cfg, "OPENQASM 3;", 1); err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	if len(seen) != 1 || seen[0] != "job-hook-1" {
		t.Fatalf("OnSubmitted ids = %v, want [job-hook-1]", seen)
	}
}
