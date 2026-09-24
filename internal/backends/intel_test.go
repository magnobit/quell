// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"testing"

	"github.com/magnobit/quell/internal/config"
)

func TestRunIntel_FallsBackToLocalStatevector(t *testing.T) {
	t.Setenv("QUELL_INTEL_SDK", "")
	t.Setenv("QUELL_INTEL_REQUIRE_SDK", "")
	res, err := RunIntel(&config.IntelConfig{
		Shots: 32,
		Extra: map[string]string{"quell_source": "H 0\nMEASURE\n"},
	}, "")
	if err != nil {
		t.Fatalf("local fallback: %v", err)
	}
	if !res.FellBack {
		t.Fatal("placeholder Intel path must record FellBack")
	}
	if res.Engine != "local-statevector" {
		t.Errorf("engine = %q, want local-statevector", res.Engine)
	}
}

func TestRunIntel_RequireSDKDoesNotFakeNativeSuccess(t *testing.T) {
	t.Setenv("QUELL_INTEL_REQUIRE_SDK", "1")
	t.Setenv("QUELL_INTEL_SDK", "1")
	_, err := RunIntel(&config.IntelConfig{
		Shots: 8,
		Extra: map[string]string{"quell_source": "H 0\nMEASURE\n"},
	}, "")
	if err == nil {
		t.Fatal("must not claim Intel Quantum SDK success — tryIntelSDK is a placeholder")
	}
}
