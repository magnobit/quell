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
)

const azureWS = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Quantum/workspaces/ws"

func validAzureCfg(baseURL string) *config.AzureConfig {
	return &config.AzureConfig{
		TenantID:       "tenant",
		ClientID:       "client",
		ClientSecret:   "secret",
		SubscriptionID: "sub",
		ResourceGroup:  "rg",
		Workspace:      "ws",
		Location:       "eastus",
		Target:         "quantinuum.sim.h2-1e",
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
	cfg := validAzureCfg("")
	cfg.Workspace = ""
	if _, err := RunAzure(cfg, "OPENQASM 3;"); err == nil {
		t.Fatal("expected an error for missing subscription_id/resource_group/workspace")
	}
}

func TestRunAzure_MissingLocationRejectedBeforeHTTP(t *testing.T) {
	cfg := validAzureCfg("")
	cfg.Location = ""
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil || !strings.Contains(err.Error(), "location") {
		t.Fatalf("got %v, want location error", err)
	}
}

func TestRunAzure_MissingTargetRejectedBeforeHTTP(t *testing.T) {
	cfg := validAzureCfg("")
	cfg.Target = ""
	if _, err := RunAzure(cfg, "OPENQASM 3;"); err == nil {
		t.Fatal("expected an error for missing target")
	}
}

func TestAzureSASURI_ExplainsStorageIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"ManagedIdentityForbiddenStorageAccess","message":"cannot access storage"}}`))
	}))
	defer srv.Close()
	cfg := validAzureCfg(srv.URL)
	_, err := azureSASURI("tok", cfg, "job-1", "inputData")
	if err == nil || !strings.Contains(err.Error(), "linked storage account") {
		t.Fatalf("got %v", err)
	}
}

func TestRunAzure_ProviderWithoutQASMFormatRejected(t *testing.T) {
	cfg := validAzureCfg("")
	cfg.Target = "ionq.simulator"
	_, err := RunAzure(cfg, "OPENQASM 3;")
	if err == nil || !strings.Contains(err.Error(), "inputDataFormat") {
		t.Fatalf("got %v, want inputDataFormat hint", err)
	}
}

// azureFake is an in-memory Azure Quantum workspace: AAD, storage SAS, blob
// upload, job create/get, and result blob.
type azureFake struct {
	srv        *httptest.Server
	uploaded   string
	job        map[string]any
	jobID      string
	status     string
	errMsg     string
	resultBody string
	putStatus  int
}

func newAzureFake(t *testing.T) *azureFake {
	t.Helper()
	f := &azureFake{status: "Succeeded", resultBody: `{"c": ["00", "11", "11"]}`}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token": "aad-token-1"}`))
	})
	mux.HandleFunc(azureWS+"/storage/sasUri", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer aad-token-1" || r.URL.Query().Get("api-version") != azureAPIVersion {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			ContainerName string `json:"containerName"`
			BlobName      string `json:"blobName"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		u := f.srv.URL + "/blob/" + req.ContainerName
		if req.BlobName != "" {
			u += "/" + req.BlobName
		}
		json.NewEncoder(w).Encode(map[string]string{"sasUri": u + "?sig=SAS"})
	})
	mux.HandleFunc("/blob/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT" && r.Header.Get("x-ms-blob-type") == "BlockBlob":
			b, _ := io.ReadAll(r.Body)
			f.uploaded = string(b)
			w.WriteHeader(http.StatusCreated)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/rawOutputData"):
			w.Write([]byte(f.resultBody))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	mux.HandleFunc(azureWS+"/jobs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, azureWS+"/jobs/")
		switch r.Method {
		case "PUT":
			if f.putStatus != 0 {
				w.WriteHeader(f.putStatus)
				w.Write([]byte(`{"error": {"message": "invalid target"}}`))
				return
			}
			f.jobID = id
			json.NewDecoder(r.Body).Decode(&f.job)
			w.Write([]byte(`{}`))
		case "GET":
			json.NewEncoder(w).Encode(map[string]any{
				"status":        f.status,
				"outputDataUri": f.srv.URL + "/blob/job-" + id + "/rawOutputData?sig=SAS",
				"errorData":     map[string]string{"message": f.errMsg},
			})
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func TestRunAzure_UploadsInputThenCreatesJob(t *testing.T) {
	f := newAzureFake(t)
	got, err := RunAzure(validAzureCfg(f.srv.URL), "OPENQASM 2.0; qreg q[2];")
	if err != nil {
		t.Fatalf("RunAzure: %v", err)
	}
	if !strings.Contains(f.uploaded, "OPENQASM 2.0;") || !strings.Contains(f.uploaded, "qreg q[2];") || !strings.Contains(f.uploaded, "measure q -> c;") {
		t.Errorf("uploaded blob = %q", f.uploaded)
	}
	if len(f.jobID) != 36 || got.JobID != f.jobID {
		t.Errorf("job id = %q (result %q), want a UUID", f.jobID, got.JobID)
	}
	if f.job["providerId"] != "quantinuum" || f.job["itemType"] != "Job" || f.job["inputDataFormat"] != "honeywell.openqasm.v1" {
		t.Errorf("job body = %v", f.job)
	}
	if !strings.Contains(f.job["containerUri"].(string), "/blob/job-"+f.jobID+"?") ||
		!strings.Contains(f.job["inputDataUri"].(string), "/blob/job-"+f.jobID+"/inputData?") {
		t.Errorf("container/input URIs = %v / %v", f.job["containerUri"], f.job["inputDataUri"])
	}
	if _, inline := f.job["inputData"]; inline {
		t.Error("circuit must be uploaded, not inlined")
	}
	if got.Counts["00"] != 1 || got.Counts["11"] != 2 {
		t.Errorf("Counts = %v", got.Counts)
	}
}

func TestRunAzure_ExtraOverridesFormats(t *testing.T) {
	f := newAzureFake(t)
	f.resultBody = `{"histogram": {"0": 0.25, "1": 0.75}}`
	cfg := validAzureCfg(f.srv.URL)
	cfg.Target = "ionq.simulator"
	cfg.Shots = 100
	cfg.Extra = map[string]string{"inputDataFormat": "ionq.circuit.v1", "outputDataFormat": "ionq.quantum-results.v1"}
	got, err := RunAzure(cfg, "{}")
	if err != nil {
		t.Fatalf("RunAzure: %v", err)
	}
	if f.job["inputDataFormat"] != "ionq.circuit.v1" || f.job["outputDataFormat"] != "ionq.quantum-results.v1" {
		t.Errorf("formats = %v / %v", f.job["inputDataFormat"], f.job["outputDataFormat"])
	}
	params, _ := f.job["inputParams"].(map[string]any)
	if _, leaked := params["inputDataFormat"]; leaked {
		t.Error("format overrides must not be sent as inputParams")
	}
	if got.Counts["0"] != 25 || got.Counts["1"] != 75 {
		t.Errorf("probability histogram counts = %v", got.Counts)
	}
	if f.uploaded != "{}" {
		t.Errorf("prepared body was rewritten: %q", f.uploaded)
	}
}

func TestRunAzure_TranslatesRigettiAndPasqal(t *testing.T) {
	bell := "OPENQASM 3;\nqubit[2] q;\nbit[2] c;\nh q[0];\ncx q[0], q[1];\nc = measure q;\n"
	f := newAzureFake(t)
	cfg := validAzureCfg(f.srv.URL)
	cfg.Target = "rigetti.sim.qvm"
	if _, err := RunAzure(cfg, bell); err != nil {
		t.Fatalf("rigetti: %v", err)
	}
	if f.job["inputDataFormat"] != "rigetti.quil.v1" || f.job["outputDataFormat"] != "rigetti.quil-results.v1" {
		t.Fatalf("rigetti formats = %v / %v", f.job["inputDataFormat"], f.job["outputDataFormat"])
	}
	if !strings.Contains(f.uploaded, "DECLARE ro BIT[2]") || !strings.Contains(f.uploaded, "H 0") || !strings.Contains(f.uploaded, "CNOT 0 1") {
		t.Fatalf("rigetti body = %q", f.uploaded)
	}

	f = newAzureFake(t)
	f.resultBody = `{"counter": {"00": 1, "11": 3}}`
	cfg = validAzureCfg(f.srv.URL)
	cfg.Target = "pasqal.sim.emu-free"
	got, err := RunAzure(cfg, bell)
	if err != nil {
		t.Fatalf("pasqal: %v", err)
	}
	if f.job["inputDataFormat"] != "pasqal.pulser.v1" || f.job["outputDataFormat"] != "pasqal.pulser-results.v1" {
		t.Fatalf("pasqal formats = %v / %v", f.job["inputDataFormat"], f.job["outputDataFormat"])
	}
	if !strings.Contains(f.uploaded, `"sequence_builder"`) || !strings.Contains(f.uploaded, `"rydberg_local"`) {
		t.Fatalf("pasqal body = %q", f.uploaded)
	}
	if got.Counts["00"] != 1 || got.Counts["11"] != 3 {
		t.Fatalf("pasqal counts = %v", got.Counts)
	}
}

func TestParseAzureResults_RigettiShots(t *testing.T) {
	counts, err := parseAzureResults([]byte(`{"ro":[[0,0],[1,1],[1,1]]}`), 3)
	if err != nil {
		t.Fatal(err)
	}
	if counts["00"] != 1 || counts["11"] != 2 {
		t.Fatalf("counts = %v", counts)
	}
}

func TestRunAzure_JobFailedSurfacesError(t *testing.T) {
	f := newAzureFake(t)
	f.status, f.errMsg = "Failed", "target unavailable"
	_, err := RunAzure(validAzureCfg(f.srv.URL), "OPENQASM 2.0;")
	if err == nil {
		t.Fatal("expected an error when the job reaches Failed status")
	}
}

func TestRunAzure_JobCancelledSurfacesError(t *testing.T) {
	f := newAzureFake(t)
	f.status = "Cancelled"
	if _, err := RunAzure(validAzureCfg(f.srv.URL), "OPENQASM 2.0;"); err == nil {
		t.Fatal("expected an error when the job reaches Cancelled status")
	}
}

func TestRunAzure_AuthHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid_client", "error_description": "bad secret"}`))
	}))
	defer srv.Close()
	if _, err := RunAzure(validAzureCfg(srv.URL), "OPENQASM 2.0;"); err == nil {
		t.Fatal("expected an error when the AAD token exchange fails")
	}
}

func TestRunAzure_SubmitHTTPErrorSurfaces(t *testing.T) {
	f := newAzureFake(t)
	f.putStatus = http.StatusBadRequest
	if _, err := RunAzure(validAzureCfg(f.srv.URL), "OPENQASM 2.0;"); err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}

func TestAzureWorkspaceBase_UsesRegionalEndpoint(t *testing.T) {
	cfg := validAzureCfg("")
	want := "https://eastus.quantum.azure.com" + azureWS
	if got := azureWorkspaceBase(cfg); got != want {
		t.Fatalf("base = %s, want %s", got, want)
	}
}
