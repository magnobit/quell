// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnobit/quell/internal/config"
)

// serviceAccountJSON builds a fake-but-structurally-valid Google service
// account key whose token_uri points at tokenURL — this is what lets tests
// exercise RunGoogle's real JWT-signing auth path (googleAccessToken /
// makeServiceAccountJWT) without a live Google credential: the mock OAuth
// endpoint below never verifies the JWT, only decodes the request.
func serviceAccountJSON(t *testing.T, tokenURL string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8 key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	sa := map[string]string{
		"type":         "service_account",
		"project_id":   "test-project",
		"client_email": "quell-test@test-project.iam.gserviceaccount.com",
		"private_key":  string(pemKey),
		"token_uri":    tokenURL,
	}
	b, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("marshal service account JSON: %v", err)
	}
	return string(b)
}

// googleDecodeBitsB64 is the test-side inverse of googleDecodeBits: packs
// numShots single-bit results (0/1) MSB-first into base64, matching what
// Google Quantum Engine's qubitMeasurementResults.results field carries.
func googleDecodeBitsB64(shotBits []byte) string {
	data := make([]byte, (len(shotBits)+7)/8)
	for i, b := range shotBits {
		if b != 0 {
			data[i/8] |= 1 << uint(7-(i%8))
		}
	}
	return base64.StdEncoding.EncodeToString(data)
}

func TestRunGoogle_MissingProjectRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.GCPConfig{Processor: "weber", KeyFile: "{}", BaseURL: srv.URL}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing project")
	}
	if called {
		t.Error("no HTTP call should have been made before validating project")
	}
}

func TestRunGoogle_MissingProcessorRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.GCPConfig{Project: "proj", KeyFile: "{}", BaseURL: srv.URL}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing processor")
	}
	if called {
		t.Error("no HTTP call should have been made before validating processor")
	}
}

func TestRunGoogle_MissingKeyFileRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.GCPConfig{Project: "proj", Processor: "weber", BaseURL: srv.URL}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing key_file")
	}
	if called {
		t.Error("no HTTP call should have been made before validating key_file")
	}
}

func TestRunGoogle_KnownGoodFixture(t *testing.T) {
	// 2 qubits, 2 shots: qubit c[0] bits {0,1}, qubit c[1] bits {1,0} ->
	// shot0 = "01", shot1 = "10".
	q0 := googleDecodeBitsB64([]byte{0, 1})
	q1 := googleDecodeBitsB64([]byte{1, 0})

	resultRaw, _ := json.Marshal(map[string]any{
		"sweepResults": []map[string]any{
			{
				"parameterizedResults": []map[string]any{
					{
						"numRepetitions": 2,
						"measurementResults": []map[string]any{
							{
								"key": "c",
								"qubitMeasurementResults": []map[string]any{
									{"results": q0},
									{"results": q1},
								},
							},
						},
					},
				},
			},
		},
	})

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "gcp-token-1"}`))
	})
	mux.HandleFunc("/projects/test-project/programs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Write([]byte(`{"name": "projects/test-project/programs/prog-1"}`))
	})
	mux.HandleFunc("/projects/test-project/programs/prog-1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Write([]byte(`{"name": "projects/test-project/programs/prog-1/jobs/job-1"}`))
	})
	mux.HandleFunc("/projects/test-project/programs/prog-1/jobs/job-1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"executionStatus": {"state": "SUCCESS"}, "result": ` + string(resultRaw) + `}`))
	})

	cfg := &config.GCPConfig{
		Project:   "test-project",
		Processor: "weber",
		KeyFile:   serviceAccountJSON(t, srv.URL+"/oauth/token"),
		BaseURL:   srv.URL,
	}
	got, err := RunGoogle(cfg, "OPENQASM 3;")
	if err != nil {
		t.Fatalf("RunGoogle: %v", err)
	}
	if got.JobID != "projects/test-project/programs/prog-1/jobs/job-1" {
		t.Errorf("JobID = %q, want the job name extracted from the create-job response", got.JobID)
	}
	if got.Counts["01"] != 1 || got.Counts["10"] != 1 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want {01:1, 10:1}", got.Counts)
	}
}

func TestRunGoogle_JobFailureSurfacesError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "gcp-token-1"}`))
	})
	mux.HandleFunc("/projects/p/programs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "projects/p/programs/prog-1"}`))
	})
	mux.HandleFunc("/projects/p/programs/prog-1/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "projects/p/programs/prog-1/jobs/job-fail-1"}`))
	})
	mux.HandleFunc("/projects/p/programs/prog-1/jobs/job-fail-1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"executionStatus": {"state": "FAILURE"}, "failure": {"error": "processor offline"}}`))
	})

	cfg := &config.GCPConfig{
		Project:   "p",
		Processor: "weber",
		KeyFile:   serviceAccountJSON(t, srv.URL+"/oauth/token"),
		BaseURL:   srv.URL,
	}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the job reaches FAILURE state")
	}
}

func TestRunGoogle_JobCancelledSurfacesError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "gcp-token-1"}`))
	})
	mux.HandleFunc("/projects/p/programs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "projects/p/programs/prog-1"}`))
	})
	mux.HandleFunc("/projects/p/programs/prog-1/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name": "projects/p/programs/prog-1/jobs/job-cancel-1"}`))
	})
	mux.HandleFunc("/projects/p/programs/prog-1/jobs/job-cancel-1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"executionStatus": {"state": "CANCELLED"}}`))
	})

	cfg := &config.GCPConfig{
		Project:   "p",
		Processor: "weber",
		KeyFile:   serviceAccountJSON(t, srv.URL+"/oauth/token"),
		BaseURL:   srv.URL,
	}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the job reaches CANCELLED state")
	}
}

func TestRunGoogle_CreateProgramHTTPErrorSurfaces(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "gcp-token-1"}`))
	})
	mux.HandleFunc("/projects/p/programs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "permission denied"}`))
	})

	cfg := &config.GCPConfig{
		Project:   "p",
		Processor: "weber",
		KeyFile:   serviceAccountJSON(t, srv.URL+"/oauth/token"),
		BaseURL:   srv.URL,
	}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for a non-2xx create-program response")
	}
}

func TestRunGoogle_AuthFailureSurfacesError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid_grant", "error_description": "bad JWT"}`))
	})

	cfg := &config.GCPConfig{
		Project:   "p",
		Processor: "weber",
		KeyFile:   serviceAccountJSON(t, srv.URL+"/oauth/token"),
		BaseURL:   srv.URL,
	}
	_, err := RunGoogle(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the OAuth2 token exchange fails")
	}
}
