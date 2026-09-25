// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package backends

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magnobit/quell/internal/config"
)

func TestP5B_ClassifyHTTP_FullMatrix(t *testing.T) {
	cases := map[int]string{
		401: ClassAuth,
		403: ClassPermission,
		404: ClassJobNotFound,
		408: ClassTimeout,
		429: ClassRateLimit,
		500: ClassProviderUnavailable,
		502: ClassProviderUnavailable,
		503: ClassProviderUnavailable,
		504: ClassTimeout,
		400: ClassInvalidRequest,
		418: ClassInvalidRequest,
		200: ClassProviderError,
	}
	for code, want := range cases {
		if got := classifyHTTP(code); got != want {
			t.Errorf("classifyHTTP(%d)=%s want %s", code, got, want)
		}
	}
}

func TestP5B_IBM_FaultMatrix_ContractOnly(t *testing.T) {
	type tc struct {
		name   string
		status int
		body   string
		class  string
		secret string
	}
	cases := []tc{
		{"401", 401, `{"errors":[{"message":"Bearer super-secret"}]}`, ClassAuth, "super-secret"},
		{"403", 403, `{"error":"nope"}`, ClassPermission, ""},
		{"404", 404, `{"error":"missing"}`, ClassJobNotFound, ""},
		{"429", 429, `{"error":"slow"}`, ClassRateLimit, ""},
		{"500", 500, `{"error":"boom"}`, ClassProviderUnavailable, ""},
		{"502", 502, `bad gateway`, ClassProviderUnavailable, ""},
		{"503", 503, ``, ClassProviderUnavailable, ""},
		{"malformed", 200, `{not-json`, "", ""},
		{"empty", 200, ``, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newIBMServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.status >= 400 {
					w.WriteHeader(c.status)
					_, _ = w.Write([]byte(c.body))
					return
				}
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			cfg := &config.IBMConfig{Token: "tok-secret", Device: "ibm_test", Instance: testIBMCRN, BaseURL: srv.URL}
			_, err := RunIBM(cfg, "OPENQASM 3;", 1)
			if err == nil {
				t.Fatal("expected error")
			}
			if c.class != "" && ClassOf(err) != c.class {
				t.Errorf("ClassOf=%q want %s err=%v", ClassOf(err), c.class, err)
			}
			msg := err.Error()
			if strings.Contains(msg, "tok-secret") {
				t.Fatalf("token leaked: %v", err)
			}
			if c.secret != "" && strings.Contains(msg, c.secret) {
				t.Fatalf("secret leaked: %v", err)
			}
		})
	}
}

func TestP5B_PollBackoff_Bounded(t *testing.T) {
	for i := 1; i <= 12; i++ {
		d := pollBackoff(i)
		if d < pollInterval || d > 30*time.Second && d > pollInterval {
			// pollBackoff caps at 30s
		}
		if d > 30*time.Second {
			t.Fatalf("pollBackoff(%d)=%s exceeds 30s", i, d)
		}
		if d < 0 {
			t.Fatalf("negative backoff")
		}
	}
}
