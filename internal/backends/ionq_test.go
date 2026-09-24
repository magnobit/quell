// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
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
		case r.Method == "GET" && r.URL.Path == "/jobs/ionq-job-1/results":
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
	if gotMethod != "PUT" || gotPath != "/jobs/ionq-job-9/status" {
		t.Fatalf("cancel request = %s %s, want PUT /jobs/ionq-job-9/status", gotMethod, gotPath)
	}
}
