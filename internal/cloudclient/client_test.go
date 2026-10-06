// Copyright 2026 Magnobit, Inc. All rights reserved.

package cloudclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubmitAndRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "" {
			http.Error(w, "missing", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/backends":
			_ = json.NewEncoder(w).Encode(map[string]any{"backends": []map[string]any{{"id": "intel", "qubits_state": "unknown"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs":
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(body, &req)
			if req["language"] != "quell" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "UNSUPPORTED_LANGUAGE", "message": "no"}})
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Job{ID: "job-1", Status: "queued", Language: "quell", Backend: "intel"})
		case r.URL.Path == "/api/v1/jobs/job-1/result":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "succeeded", "result": map[string]any{"counts": map[string]any{"00": 128}}})
		case r.URL.Path == "/api/v1/jobs/job-1/provenance":
			_ = json.NewEncoder(w).Encode(map[string]any{"source_language": "quell"})
		case r.URL.Path == "/api/v1/jobs/job-1/verify":
			_ = json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := &Client{Endpoint: srv.URL, APIKey: "qbl_test", HTTP: srv.Client()}
	backs, err := c.ListBackends()
	if err != nil || backs[0]["qubits_state"] != "unknown" {
		t.Fatalf("backends: %v %v", backs, err)
	}
	job, err := c.Submit("H 0\nMEASURE\n", "quell", "auto", 128, "req-1")
	if err != nil || job.ID != "job-1" || job.Backend != "intel" {
		t.Fatalf("submit: %+v %v", job, err)
	}
	result, err := c.Result(job.ID)
	if err != nil || result["status"] != "succeeded" {
		t.Fatalf("result: %v %v", result, err)
	}
	if _, err := c.Provenance(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Verify(job.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnsupportedLanguageCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "UNSUPPORTED_LANGUAGE", "message": "no", "request_id": "req-1"}})
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, APIKey: "qbl_test", HTTP: srv.Client()}
	_, err := c.Submit("kernel", "cudaq", "auto", 10, "")
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != "UNSUPPORTED_LANGUAGE" {
		t.Fatalf("got %v", err)
	}
}
