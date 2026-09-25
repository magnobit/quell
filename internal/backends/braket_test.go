// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnobit/quell/internal/config"
)

// clearAWSEnv ensures RunBraket's env-var fallback (AWS_ACCESS_KEY_ID etc.)
// can't mask a missing-config test result if the host running these tests
// happens to have real AWS credentials exported.
func clearAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
}

func TestRunBraket_MissingCredentialsRejectedBeforeHTTP(t *testing.T) {
	clearAWSEnv(t)
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{S3Bucket: "bucket", BaseURL: srv.URL}
	_, err := RunBraket(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing access_key_id/secret_access_key")
	}
	if called {
		t.Error("no HTTP call should have been made before validating credentials")
	}
}

func TestRunBraket_MissingS3BucketRejectedBeforeHTTP(t *testing.T) {
	clearAWSEnv(t)
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{AccessKeyID: "AKIA...", SecretAccessKey: "secret", BaseURL: srv.URL}
	_, err := RunBraket(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for missing s3_bucket")
	}
	if called {
		t.Error("no HTTP call should have been made before validating s3_bucket")
	}
}

func TestRunBraket_KnownGoodFixture(t *testing.T) {
	clearAWSEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/quantum-task":
			w.Write([]byte(`{"quantumTaskArn": "arn:aws:braket:task-1"}`))
		case r.Method == "GET" && r.URL.Path == "/quantum-task/arn:aws:braket:task-1":
			w.Write([]byte(`{"status": "COMPLETED", "outputS3Bucket": "my-bucket", "outputS3Directory": "results-dir"}`))
		case r.Method == "GET" && r.URL.Path == "/my-bucket/results-dir/results.json":
			w.Write([]byte(`{"measurements": [[0, 1], [1, 0], [0, 1]]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{
		AccessKeyID:     "AKIA...",
		SecretAccessKey: "secret",
		S3Bucket:        "my-bucket",
		BaseURL:         srv.URL,
	}
	got, err := RunBraket(cfg, "OPENQASM 3;")
	if err != nil {
		t.Fatalf("RunBraket: %v", err)
	}
	if got.JobID != "arn:aws:braket:task-1" {
		t.Errorf("JobID = %q, want the task ARN extracted from submit response", got.JobID)
	}
	if got.Counts["01"] != 2 || got.Counts["10"] != 1 || len(got.Counts) != 2 {
		t.Errorf("Counts = %v, want {01:2, 10:1}", got.Counts)
	}
}

func TestRunBraket_TaskFailedSurfacesError(t *testing.T) {
	clearAWSEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/quantum-task":
			w.Write([]byte(`{"quantumTaskArn": "arn-fail-1", "outputS3Bucket": "b", "outputS3Directory": "d"}`))
		case r.Method == "GET" && r.URL.Path == "/quantum-task/arn-fail-1":
			w.Write([]byte(`{"status": "FAILED", "failureReason": "simulator error"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{AccessKeyID: "AKIA...", SecretAccessKey: "secret", S3Bucket: "b", BaseURL: srv.URL}
	_, err := RunBraket(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the task reaches FAILED status")
	}
}

func TestRunBraket_TaskCancelledSurfacesError(t *testing.T) {
	clearAWSEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/quantum-task":
			w.Write([]byte(`{"quantumTaskArn": "arn-cancel-1", "outputS3Bucket": "b", "outputS3Directory": "d"}`))
		case r.Method == "GET" && r.URL.Path == "/quantum-task/arn-cancel-1":
			w.Write([]byte(`{"status": "CANCELLED"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{AccessKeyID: "AKIA...", SecretAccessKey: "secret", S3Bucket: "b", BaseURL: srv.URL}
	_, err := RunBraket(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error when the task reaches CANCELLED status")
	}
}

func TestRunBraket_SubmitHTTPErrorSurfaces(t *testing.T) {
	clearAWSEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message": "invalid device ARN"}`))
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{AccessKeyID: "AKIA...", SecretAccessKey: "secret", S3Bucket: "b", BaseURL: srv.URL}
	_, err := RunBraket(cfg, "OPENQASM 3;")
	if err == nil {
		t.Fatal("expected an error for a non-2xx submit response")
	}
}

func TestRunBraket_SendsClientTokenAndEscapesTaskArn(t *testing.T) {
	clearAWSEnv(t)
	const arn = "arn:aws:braket:us-east-1:123456789012:quantum-task/abc-123"
	var submit map[string]any
	var rawPollPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/quantum-task":
			json.NewDecoder(r.Body).Decode(&submit)
			w.Write([]byte(`{"quantumTaskArn": "` + arn + `"}`))
		case r.Method == "GET" && r.URL.Path == "/quantum-task/"+arn:
			rawPollPath = r.URL.EscapedPath()
			w.Write([]byte(`{"status": "COMPLETED", "outputS3Bucket": "b", "outputS3Directory": "quell-results/abc-123"}`))
		case r.Method == "GET" && r.URL.Path == "/b/quell-results/abc-123/results.json":
			w.Write([]byte(`{"measurements": [[1]]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{AccessKeyID: "AKIA", SecretAccessKey: "secret", S3Bucket: "b", BaseURL: srv.URL}
	if _, err := RunBraket(cfg, "OPENQASM 3;"); err != nil {
		t.Fatalf("RunBraket: %v", err)
	}
	if tok, _ := submit["clientToken"].(string); len(tok) != 36 {
		t.Errorf("clientToken = %v, want a UUID", submit["clientToken"])
	}
	if submit["outputS3KeyPrefix"] != "quell-results" {
		t.Errorf("outputS3KeyPrefix = %v", submit["outputS3KeyPrefix"])
	}
	if rawPollPath != "/quantum-task/arn%3Aaws%3Abraket%3Aus-east-1%3A123456789012%3Aquantum-task%2Fabc-123" {
		t.Errorf("poll path sent as %q", rawPollPath)
	}
}

func TestAWSCanonicalPath_DoubleEncodesExceptS3(t *testing.T) {
	sent := "/quantum-task/" + awsEscapeSegment("arn:aws:braket:us-east-1:1:quantum-task/x")
	got := awsCanonicalPath(sent, "braket")
	want := "/quantum-task/arn%253Aaws%253Abraket%253Aus-east-1%253A1%253Aquantum-task%252Fx"
	if got != want {
		t.Errorf("braket canonical = %s\nwant %s", got, want)
	}
	if got := awsCanonicalPath("/bucket/quell-results/x/results.json", "s3"); got != "/bucket/quell-results/x/results.json" {
		t.Errorf("s3 canonical = %s", got)
	}
	if got := awsCanonicalPath("", "braket"); got != "/" {
		t.Errorf("empty path canonical = %s", got)
	}
}

func TestCancelBraket_PutsTaskCancel(t *testing.T) {
	clearAWSEnv(t)
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.AWSConfig{AccessKeyID: "AKIA...", SecretAccessKey: "secret", S3Bucket: "b", BaseURL: srv.URL}
	if err := CancelBraket(cfg, "arn-task-9"); err != nil {
		t.Fatalf("CancelBraket: %v", err)
	}
	if gotMethod != "PUT" {
		t.Fatalf("cancel method = %s, want PUT", gotMethod)
	}
	if gotPath != "/quantum-task/arn-task-9/cancel" {
		t.Fatalf("cancel path = %s, want /quantum-task/arn-task-9/cancel", gotPath)
	}
}
