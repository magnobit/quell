// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"testing"

	"github.com/magnobit/quell/internal/config"
)

func TestRunNVIDIA_FallsBackWithoutCUDAQ(t *testing.T) {
	t.Setenv("QUELL_NVIDIA_REQUIRE_CUDAQ", "")
	t.Setenv("PATH", "") // force tryCUDAQ to fail even if CUDA-Q is installed
	res, err := RunNVIDIA(&config.NVIDIAConfig{
		Shots: 32,
		Extra: map[string]string{"quell_source": "H 0\nMEASURE\n"},
	}, "")
	if err != nil {
		t.Fatalf("local fallback: %v", err)
	}
	if !res.FellBack {
		t.Fatal("missing CUDA-Q must record FellBack — do not claim native GPU success")
	}
	if res.Engine != "local-statevector" {
		t.Errorf("engine = %q, want local-statevector", res.Engine)
	}
}

func TestRunNVIDIA_RequireCUDAQDoesNotFakeSuccess(t *testing.T) {
	t.Setenv("QUELL_NVIDIA_REQUIRE_CUDAQ", "1")
	t.Setenv("PATH", "")
	_, err := RunNVIDIA(&config.NVIDIAConfig{
		Shots: 8,
		Extra: map[string]string{"quell_source": "H 0\nMEASURE\n"},
	}, "")
	if err == nil {
		t.Fatal("QUELL_NVIDIA_REQUIRE_CUDAQ=1 must not invent a CUDA-Q result")
	}
}
