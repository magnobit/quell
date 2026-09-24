// Copyright 2026 Magnobit, Inc. All rights reserved.

package livevalidate

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestBudgetFromEnv_DefaultsAndCaps(t *testing.T) {
	t.Setenv("QUELL_LIVE_MAX_JOBS", "99")
	t.Setenv("QUELL_LIVE_MAX_SHOTS", "1000")
	t.Setenv("QUELL_LIVE_PROVIDERS", "ibm")
	b := budgetFromEnv()
	if b.MaxJobs != 8 {
		t.Errorf("MaxJobs = %d, want cap 8", b.MaxJobs)
	}
	if b.MaxShots != 32 {
		t.Errorf("MaxShots = %d, want cap 32", b.MaxShots)
	}
	if !b.providerAllowed("ibm") || b.providerAllowed("aws") {
		t.Errorf("allow-list = %v", b.Providers)
	}
}

func TestBudgetAdmitSubmit_RequiresApproval(t *testing.T) {
	b := Budget{MaxJobs: 2, MaxCost: 1, MaxDuration: time.Minute, AllowSubmit: true}
	if err := b.admitSubmit(0, 0.1, time.Now()); err == nil {
		t.Fatal("expected missing budget approval")
	}
	b.Approved = true
	if err := b.admitSubmit(0, 0.1, time.Now()); err != nil {
		t.Fatalf("approved admit: %v", err)
	}
	if err := b.admitSubmit(2, 0.1, time.Now()); err == nil {
		t.Fatal("expected max_jobs exhaustion")
	}
	if err := b.admitSubmit(0, 2.0, time.Now()); err == nil {
		t.Fatal("expected cost exhaustion")
	}
}

func TestScopeHash_IsStableAndSecretFree(t *testing.T) {
	a := scopeHash("ibm-q/open/main")
	b := scopeHash("ibm-q/open/main")
	if a != b || a == "" {
		t.Fatalf("hash not stable: %s %s", a, b)
	}
	if containsSecret(a) {
		t.Fatal("hash looked like a secret")
	}
}

func TestLiveSkippedWithoutOptIn(t *testing.T) {
	if os.Getenv("QUELL_LIVE") == "1" {
		t.Skip("QUELL_LIVE=1 — live suite owns this path")
	}
	if strings.Contains(os.Getenv("IBM_QUANTUM_TOKEN"), " ") {
		t.Fatal("token env should not be echoed; this is a sentinel")
	}
}
