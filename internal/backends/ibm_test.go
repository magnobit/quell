// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/config"
)

const testIBMCRN = "crn:v1:bluemix:public:quantum-computing:us-east:a/acct:inst::"

// newIBMServer answers IAM token requests itself (API key "tok" or
// "bad-tok" both succeed) and hands every other request to h.
func newIBMServer(h http.Handler) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/token" {
			w.Write([]byte(`{"access_token":"iam-bearer","expires_in":3600}`))
			return
		}
		h.ServeHTTP(w, r)
	}))
}

func TestRunIBM_LegacyInstanceRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: "ibm-q/open/main", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil || !strings.Contains(err.Error(), "CRN") {
		t.Fatalf("got %v, want CRN error", err)
	}
	if called {
		t.Error("no HTTP call should have been made for a hub/group/project instance")
	}
}

func TestRunIBM_UsesIAMBearerAndServiceCRN(t *testing.T) {
	iamCalls := 0
	var submitBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/token" {
			iamCalls++
			r.ParseForm()
			if r.Form.Get("apikey") != "tok" || r.Form.Get("grant_type") != "urn:ibm:params:oauth:grant-type:apikey" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"access_token":"iam-bearer","expires_in":3600}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer iam-bearer" || r.Header.Get("Service-CRN") != testIBMCRN || r.Header.Get("IBM-API-Version") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			json.NewDecoder(r.Body).Decode(&submitBody)
			w.Write([]byte(`{"id": "job-new-1"}`))
		case r.URL.Path == "/api/v1/jobs/job-new-1":
			w.Write([]byte(`{"status": "Completed", "state": {"status": "Completed"}}`))
		case r.URL.Path == "/api/v1/jobs/job-new-1/results":
			w.Write([]byte(`{"results": [{"data": {"counts": {"0x1": 16}}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, Shots: 16, BaseURL: srv.URL}
	got, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	if got.Counts["1"] != 16 {
		t.Errorf("Counts = %v", got.Counts)
	}
	if iamCalls != 1 {
		t.Errorf("IAM called %d times, want 1 (token must be reused)", iamCalls)
	}
	if _, has := submitBody["instance"]; has {
		t.Error("job body must not carry instance; the CRN goes in Service-CRN")
	}
	if submitBody["program_id"] != "sampler" || submitBody["backend"] != "ibm_test" {
		t.Errorf("submit body = %v", submitBody)
	}
}

func TestRunIBM_IAMRejectionIsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorCode":"BXNIM0415E","errorMessage":"Provided API key could not be found."}`))
	}))
	defer srv.Close()
	cfg := &config.IBMConfig{Token: "wrong-key-xyz", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil || !strings.Contains(err.Error(), "API key could not be found") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "wrong-key-xyz") {
		t.Fatalf("API key leaked: %v", err)
	}
}

func TestRunIBM_CancelledRanTooLongIsFailure(t *testing.T) {
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-long-1"}`))
		case r.URL.Path == "/api/v1/jobs/job-long-1":
			w.Write([]byte(`{"status": "Cancelled - Ran too long", "state": {"status": "Cancelled", "reason": "Ran too long"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	if _, err := RunIBM(cfg, "OPENQASM 3;", 1); err == nil {
		t.Fatal("expected an error for Cancelled - Ran too long")
	}
}

func TestRunIBM_MissingTokenRejectedBeforeHTTP(t *testing.T) {
	called := false
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
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
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-cr-1"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-cr-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-cr-1/results":
			w.Write([]byte(`{"results": [{"data": {"counts": {"0x0": 7, "0x3": 3}}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
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

	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-sv2-1"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-sv2-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-sv2-1/results":
			w.Write(respBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	got, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	want := map[string]int{"01": 1, "10": 1}
	if got.Counts["01"] != 1 || got.Counts["10"] != 1 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want %v (Sampler V2 BitArray format)", got.Counts, want)
	}
}

func TestRunIBM_DecodesHexSamples(t *testing.T) {
	const body = `{"results": [{"data": {"c": {"samples": ["0x0", "0x2", "0x0", "0x0", "0x0", "0x0", "0x3", "0x3"], "num_bits": 2}}, "metadata": {"circuit_metadata": {}}}], "metadata": {"version": 2}}`
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-samples"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-samples":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-samples/results":
			w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, Shots: 8, BaseURL: srv.URL}
	got, err := RunIBM(cfg, "OPENQASM 3;\nqubit[2] q;\nbit[2] c;\nh q[0];\ncx q[0], q[1];\nc = measure q;\n", 2)
	if err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	// 0x0 → 00 (5), 0x2 → 10 (1), 0x3 → 11 (2). Same hex mapping as circuit-runner counts.
	if got.Counts["00"] != 5 || got.Counts["10"] != 1 || got.Counts["11"] != 2 || len(got.Counts) != 3 {
		t.Fatalf("Counts = %v, want 00:5 10:1 11:2", got.Counts)
	}
}

func TestRunIBM_JobFailedSurfacesError(t *testing.T) {
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-fail-1"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-fail-1":
			w.Write([]byte(`{"status": "Failed", "error": {"message": "calibration drift"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error when the job reaches Failed status")
	}
}

func TestRunIBM_JobCancelledSurfacesError(t *testing.T) {
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-cancel-1"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-cancel-1":
			w.Write([]byte(`{"status": "Cancelled"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error when the job reaches Cancelled status")
	}
}

func TestRunIBM_SubmitHTTPErrorSurfaces(t *testing.T) {
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors": [{"message": "invalid token"}]}`))
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "bad-tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}

func TestRunIBM_UnrecognisedResultFormatSurfacesError(t *testing.T) {
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-weird-1"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-weird-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-weird-1/results":
			w.Write([]byte(`{"nothing": "recognisable"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 2)
	if err == nil {
		t.Fatal("expected an error for an unrecognised result format, not a silent success")
	}
}

func TestCancelIBM_PostsRuntimeCancel(t *testing.T) {
	var gotPath, gotMethod string
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	if err := CancelIBM(cfg, "job-to-cancel"); err != nil {
		t.Fatalf("CancelIBM: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/jobs/job-to-cancel/cancel" {
		t.Fatalf("cancel request = %s %s, want POST /api/v1/jobs/job-to-cancel/cancel", gotMethod, gotPath)
	}
}

func TestRunIBM_OnSubmittedFiresAfterSubmitBeforePoll(t *testing.T) {
	var seen []string
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			w.Write([]byte(`{"id": "job-hook-1"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-hook-1":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/job-hook-1/results":
			w.Write([]byte(`{"results": [{"data": {"counts": {"0x0": 1}}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL, OnSubmitted: func(id string) {
		seen = append(seen, id)
	}}
	if _, err := RunIBM(cfg, "OPENQASM 3;", 1); err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	if len(seen) != 1 || seen[0] != "job-hook-1" {
		t.Fatalf("OnSubmitted ids = %v, want [job-hook-1]", seen)
	}
}

func TestRunIBM_SubmitsISACircuitAndDecodesSamplerV2(t *testing.T) {
	var submitted string
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/backends/ibm_test/configuration":
			w.Write([]byte(`{"n_qubits": 5, "basis_gates": ["cz", "id", "rz", "sx", "x"], "coupling_map": [[0, 1], [1, 0], [1, 2]]}`))
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs":
			var body struct {
				Params struct {
					Pubs [][]any `json:"pubs"`
				} `json:"params"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			submitted, _ = body.Params.Pubs[0][0].(string)
			w.Write([]byte(`{"id": "job-isa"}`))
		case r.URL.Path == "/api/v1/jobs/job-isa":
			w.Write([]byte(`{"status": "Completed"}`))
		case r.URL.Path == "/api/v1/jobs/job-isa/results":
			w.Write([]byte(samplerV2Result(t, "c", [][]byte{{0}, {1}, {1}, {1}}, 1)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, Shots: 4, BaseURL: srv.URL}
	got, err := RunIBM(cfg, "OPENQASM 3;\nqubit[1] q;\nbit[1] c;\nh q[0];\nc = measure q;\n", 1)
	if err != nil {
		t.Fatalf("RunIBM: %v", err)
	}
	if strings.Contains(submitted, "h q[0]") || strings.Contains(submitted, "OPENQASM 3") || !strings.Contains(submitted, "sx q[0];") || !strings.HasPrefix(submitted, "OPENQASM 2.0;\n") || !strings.Contains(submitted, `include "qelib1.inc";`) || !strings.Contains(submitted, "measure q -> c;") {
		t.Errorf("submitted circuit is not OpenQASM 2 ISA:\n%s", submitted)
	}
	if got.Counts["0"] != 1 || got.Counts["1"] != 3 {
		t.Errorf("Counts = %v", got.Counts)
	}
}

func TestRunIBM_UncoupledCircuitRejectedBeforeSubmit(t *testing.T) {
	submitted := false
	srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/backends/ibm_test/configuration":
			w.Write([]byte(`{"n_qubits": 3, "basis_gates": ["cz", "rz", "sx", "x"], "coupling_map": [[0, 1], [1, 2]]}`))
		case r.Method == "POST":
			submitted = true
			w.Write([]byte(`{"id": "job-x"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;\nqubit[3] q;\ncx q[0], q[2];\n", 3)
	if err == nil || !strings.Contains(err.Error(), "not coupled") {
		t.Fatalf("got %v, want coupling error", err)
	}
	if submitted {
		t.Error("a circuit that cannot run must not be submitted")
	}
}
