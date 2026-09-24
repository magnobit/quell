// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/config"
)

func validAzureCfg(baseURL string) *config.AzureConfig {
	return &config.AzureConfig{
		TenantID:       "tenant",
		ClientID:       "client",
		ClientSecret:   "secret",
		SubscriptionID: "sub",
		ResourceGroup:  "rg",
		Workspace:      "ws",
		Target:         "ionq.simulator",
		BaseURL:        baseURL,
	}
}

func TestRunAzure_MissingAADCredentialsRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	cfg.ClientSecret = ""
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing tenant_id/client_id/client_secret")
	}
	if called {
		t.Error("no HTTP call should have been made before validating AAD credentials")
	}
}

func TestRunAzure_MissingWorkspaceFieldsRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	cfg.Workspace = ""
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing subscription_id/resource_group/workspace")
	}
	if called {
		t.Error("no HTTP call should have been made before validating workspace fields")
	}
}

func TestRunAzure_MissingTargetRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	cfg.Target = ""
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing target")
	}
	if called {
		t.Error("no HTTP call should have been made before validating target")
	}
}

// newAzureMux builds a mux with the AAD token endpoint pre-wired, plus a
// /jobs/ handler whose PUT and GET behavior the caller supplies. Azure's job
// ID is generated client-side (time-based) so tests can't know the exact
// path in advance — matching on the "/jobs/" prefix handles any ID.
func newAzureMux(t *testing.T, put, get func(w http.ResponseWriter, r *http.Request, jobID string)) (*httptest.Server, *string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "aad-token-1"}`))
	})
	var lastJobID string
	mux.HandleFunc("/jobs/", func(w http.ResponseWriter, r *http.Request) {
		jobID := strings.TrimPrefix(r.URL.Path, "/jobs/")
		lastJobID = jobID
		switch r.Method {
		case "PUT":
			put(w, r, jobID)
		case "GET":
			get(w, r, jobID)
		}
	})
	srv := httptest.NewServer(mux)
	return srv, &lastJobID
}

// TestRunAzure_KnownGoodFixture exercises the full AAD-auth -> submit ->
// poll -> results flow, including azureSubmit's fallback to its
// locally-generated job ID when the PUT response echoes no "id" field
// (which the real Azure Quantum API sometimes does).
func TestRunAzure_KnownGoodFixture(t *testing.T) {
	pollCount := 0
	var srv *httptest.Server
	srv, jobIDPtr := newAzureMux(t,
		func(w http.ResponseWriter, r *http.Request, jobID string) {
			w.Write([]byte(`{}`)) // no "id" field -> RunAzure must fall back
		},
		func(w http.ResponseWriter, r *http.Request, jobID string) {
			pollCount++
			if pollCount == 1 {
				w.Write([]byte(`{"status": "Succeeded"}`))
				return
			}
			w.Write([]byte(`{"outputDataUri": "` + srv.URL + `/blobs/result.json"}`))
		},
	)
	defer srv.Close()
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/blobs/result.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"histogram": {"00": 3, "11": 5}}`))
	})

	cfg := validAzureCfg(srv.URL)
	got, err := RunAzure(cfg, "OPENQASM 3;")
	if err != nil {
		t.Fatalf("RunAzure: %v", err)
	}
	if *jobIDPtr == "" {
		t.Fatal("no job ID was ever submitted")
	}
	if got.JobID != *jobIDPtr {
		t.Errorf("JobID = %q, want the fallback-generated id %q (submit response had no \"id\" field)", got.JobID, *jobIDPtr)
	}
	if got.Counts["00"] != 3 || got.Counts["11"] != 5 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want {00:3, 11:5}", got.Counts)
	}
}

func TestRunAzure_SubmitResponseIDIsUsedWhenPresent(t *testing.T) {
	pollCount := 0
	var srv *httptest.Server
	srv, _ = newAzureMux(t,
		func(w http.ResponseWriter, r *http.Request, jobID string) {
			w.Write([]byte(`{"id": "explicit-id-1"}`))
		},
		func(w http.ResponseWriter, r *http.Request, jobID string) {
			pollCount++
			if pollCount == 1 {
				w.Write([]byte(`{"status": "Succeeded"}`))
				return
			}
			w.Write([]byte(`{"outputDataUri": "` + srv.URL + `/blobs2/result.json"}`))
		},
	)
	defer srv.Close()
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/blobs2/result.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"histogram": {"01": 1}}`))
	})

	cfg := validAzureCfg(srv.URL)
	got, err := RunAzure(cfg, "OPENQASM 3;")
	if err != nil {
		t.Fatalf("RunAzure: %v", err)
	}
	if got.JobID != "explicit-id-1" {
		t.Errorf("JobID = %q, want explicit-id-1 (the id explicitly returned by the submit response)", got.JobID)
	}
}

func TestRunAzure_JobFailedSurfacesError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "aad-token-1"}`))
	})
	mux.HandleFunc("/jobs/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			w.Write([]byte(`{}`))
		case "GET":
			w.Write([]byte(`{"status": "Failed", "errorData": {"message": "target unavailable"}}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the job reaches Failed status")
	}
}

func TestRunAzure_JobCancelledSurfacesError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "aad-token-1"}`))
	})
	mux.HandleFunc("/jobs/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			w.Write([]byte(`{}`))
		case "GET":
			w.Write([]byte(`{"status": "Cancelled"}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the job reaches Cancelled status")
	}
}

func TestRunAzure_AuthHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid_client", "error_description": "bad secret"}`))
	}))
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the AAD token exchange fails")
	}
}

func TestRunAzure_SubmitHTTPErrorSurfaces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "aad-token-1"}`))
	})
	mux.HandleFunc("/jobs/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": {"message": "invalid target"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := validAzureCfg(srv.URL)
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}
