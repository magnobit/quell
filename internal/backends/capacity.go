// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/magnobit/quell/simulate"
)

// SimulatorCapacity is the qubit ceiling of the engine a local-fallback
// backend will actually run in this process. Known is false when that
// engine cannot report a limit. A zero value is not a capacity.
type SimulatorCapacity struct {
	Backend string
	Engine  string
	Qubits  int
	Known   bool
}

// cudaqImportCheck reports whether this process can import CUDA-Q.
// Tests replace it. Production caches one probe per process.
var cudaqImportCheck = cachedCUDAQImport

var (
	cudaqOnce sync.Once
	cudaqOK   bool
)

func cachedCUDAQImport() bool {
	cudaqOnce.Do(func() { cudaqOK = probeCUDAQImport() })
	return cudaqOK
}

func probeCUDAQImport() bool {
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return false
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, "-c", "import cudaq")
	return cmd.Run() == nil
}

// DiscoverSimulatorCapacity asks the runtime which engine RunNVIDIA or
// RunIntel will use here, and the qubit ceiling that engine publishes.
// Cloud providers are not simulators: Known stays false. CUDA-Q and the
// Intel SDK bridge do not publish a ceiling, so those engines stay unknown
// rather than borrowing the local statevector limit.
func DiscoverSimulatorCapacity(backend string) SimulatorCapacity {
	switch backend {
	case "intel":
		return intelCapacity()
	case "nvidia":
		return nvidiaCapacity()
	default:
		return SimulatorCapacity{Backend: backend}
	}
}

func intelCapacity() SimulatorCapacity {
	// QUELL_INTEL_REQUIRE_SDK refuses the local fallback. The SDK bridge
	// does not report a qubit ceiling.
	if os.Getenv("QUELL_INTEL_REQUIRE_SDK") == "1" {
		return SimulatorCapacity{Backend: "intel", Engine: "intel-sdk"}
	}
	return localStatevectorCapacity("intel")
}

func nvidiaCapacity() SimulatorCapacity {
	if cudaqImportCheck() || os.Getenv("QUELL_NVIDIA_REQUIRE_CUDAQ") == "1" {
		// RunNVIDIA prefers CUDA-Q whenever it imports. That engine does
		// not publish a qubit ceiling. Do not substitute the local
		// statevector limit for a CUDA-Q run.
		if cudaqImportCheck() {
			return SimulatorCapacity{Backend: "nvidia", Engine: "cudaq"}
		}
		return SimulatorCapacity{Backend: "nvidia"}
	}
	return localStatevectorCapacity("nvidia")
}

func localStatevectorCapacity(backend string) SimulatorCapacity {
	n := simulate.MaxQubits()
	if n < 1 {
		return SimulatorCapacity{Backend: backend}
	}
	return SimulatorCapacity{
		Backend: backend,
		Engine:  "local-statevector",
		Qubits:  n,
		Known:   true,
	}
}
