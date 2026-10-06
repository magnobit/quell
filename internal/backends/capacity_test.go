// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"testing"

	"github.com/magnobit/quell/simulate"
)

func TestDiscoverSimulatorCapacity_IntelUsesLocalCeiling(t *testing.T) {
	t.Setenv("QUELL_INTEL_REQUIRE_SDK", "")
	got := DiscoverSimulatorCapacity("intel")
	if !got.Known || got.Engine != "local-statevector" || got.Qubits != simulate.MaxQubits() {
		t.Fatalf("intel capacity = %+v, want local-statevector %d", got, simulate.MaxQubits())
	}
}

func TestDiscoverSimulatorCapacity_IntelSDKRequiredStaysUnknown(t *testing.T) {
	t.Setenv("QUELL_INTEL_REQUIRE_SDK", "1")
	got := DiscoverSimulatorCapacity("intel")
	if got.Known || got.Qubits != 0 {
		t.Fatalf("required Intel SDK must not invent a ceiling: %+v", got)
	}
}

func TestDiscoverSimulatorCapacity_NVIDIAWithoutCUDAQUsesSameCeiling(t *testing.T) {
	prev := cudaqImportCheck
	cudaqImportCheck = func() bool { return false }
	t.Cleanup(func() { cudaqImportCheck = prev })
	t.Setenv("QUELL_NVIDIA_REQUIRE_CUDAQ", "")
	got := DiscoverSimulatorCapacity("nvidia")
	if !got.Known || got.Engine != "local-statevector" || got.Qubits != simulate.MaxQubits() {
		t.Fatalf("nvidia fallback = %+v, want local-statevector %d", got, simulate.MaxQubits())
	}
}

func TestDiscoverSimulatorCapacity_CUDAQDoesNotInventCeiling(t *testing.T) {
	prev := cudaqImportCheck
	cudaqImportCheck = func() bool { return true }
	t.Cleanup(func() { cudaqImportCheck = prev })
	got := DiscoverSimulatorCapacity("nvidia")
	if got.Known || got.Qubits != 0 || got.Engine != "cudaq" {
		t.Fatalf("cudaq must stay unknown: %+v", got)
	}
}

func TestDiscoverSimulatorCapacity_CloudProviderStaysUnknown(t *testing.T) {
	got := DiscoverSimulatorCapacity("ibm")
	if got.Known || got.Qubits != 0 || got.Engine != "" {
		t.Fatalf("ibm is not a local simulator: %+v", got)
	}
}
