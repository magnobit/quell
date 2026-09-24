// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/magnobit/quell/internal/config"
)

func TestRedactSecrets_StripsTokensAndKeys(t *testing.T) {
	in := `Authorization: Bearer super-secret-token api_key=abc123 {"token":"ibm-tok","password":"pw"}`
	got := redactSecrets(in)
	if strings.Contains(got, "super-secret-token") || strings.Contains(got, "ibm-tok") || strings.Contains(got, "abc123") || strings.Contains(got, `"pw"`) {
		t.Fatalf("secret leaked in %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", got)
	}
}

func TestClassifyHTTP_StableClasses(t *testing.T) {
	cases := map[int]string{
		401: ClassAuth,
		403: ClassPermission,
		404: ClassJobNotFound,
		429: ClassRateLimit,
		402: ClassQuota,
		503: ClassProviderUnavailable,
		400: ClassInvalidRequest,
	}
	for code, want := range cases {
		if got := classifyHTTP(code); got != want {
			t.Errorf("classifyHTTP(%d)=%s want %s", code, got, want)
		}
	}
}

func TestRunIBM_AuthErrorClassAndNoSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors":[{"message":"invalid token Bearer leaked-token"}]}`))
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "leaked-token", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if ClassOf(err) != ClassAuth {
		t.Errorf("ClassOf = %q, want %s", ClassOf(err), ClassAuth)
	}
	if strings.Contains(err.Error(), "leaked-token") || strings.Contains(err.Error(), "Bearer leaked") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestRunIBM_RateLimitClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"slow down"}`))
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if ClassOf(err) != ClassRateLimit {
		t.Errorf("ClassOf = %q, want %s", ClassOf(err), ClassRateLimit)
	}
}

func TestRunIBM_PollTimeoutDoesNotClaimCancel(t *testing.T) {
	oldI, oldT := pollInterval, pollTimeout
	pollInterval = 5 * time.Millisecond
	pollTimeout = 40 * time.Millisecond
	defer func() {
		pollInterval = oldI
		pollTimeout = oldT
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/jobs":
			w.Write([]byte(`{"id": "job-timeout-1"}`))
		default:
			w.Write([]byte(`{"status": "Queued"}`))
		}
	}))
	defer srv.Close()

	cfg := &config.IBMConfig{Token: "tok", Device: "ibm_test", BaseURL: srv.URL}
	_, err := RunIBM(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected poll timeout")
	}
	if ClassOf(err) != ClassTimeout {
		t.Errorf("ClassOf = %q, want %s", ClassOf(err), ClassTimeout)
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "cancel") {
		t.Fatalf("timeout must not claim provider cancellation: %v", err)
	}
	if !strings.Contains(msg, "may still be running") {
		t.Fatalf("timeout should say the provider job may still be running: %v", err)
	}
}

func TestRunIonQ_AuthErrorClass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"no permission"}}`))
	}))
	defer srv.Close()

	cfg := &config.IonQConfig{APIKey: "k", Device: "simulator", BaseURL: srv.URL}
	_, err := RunIonQ(cfg, "OPENQASM 3;", 1)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if ClassOf(err) != ClassPermission {
		t.Errorf("ClassOf = %q, want %s", ClassOf(err), ClassPermission)
	}
}
