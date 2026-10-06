// Copyright 2026 Magnobit, Inc. All rights reserved.

package engine

import (
	"fmt"
	"os/exec"

	"github.com/magnobit/quell/internal/cudaq"
	"github.com/magnobit/quell/simulate"
)

const (
	LocalStatevector = "local-statevector"
	CUDAQ            = "cudaq"
	TensorNetwork    = "tensor-network"
	Stabilizer       = "stabilizer"
	DensityMatrix    = "density-matrix"
	Auto             = "auto"

	StatusAvailable   = "AVAILABLE"
	StatusUnavailable = "UNAVAILABLE"
	StatusPartial     = "PARTIAL"
)

// Result is a local run. Engine is never relabeled.
type Result struct {
	Engine string
	Counts map[string]int
}

// Info is one engine capability. A named engine that only returns an error
// is UNAVAILABLE, not AVAILABLE.
type Info struct {
	Name   string
	Status string
	Detail string
}

// Capabilities reports which engines can run in this process.
func Capabilities() []Info {
	cuda := StatusUnavailable
	detail := "CUDA-Q Python package is not importable; the adapter can still emit a kernel"
	if CUDAQInstalled() {
		cuda = StatusPartial
		detail = "CUDA-Q qpp-cpu target only; this is not a GPU claim"
	}
	tensor, density := externalStatus()
	return []Info{
		{Name: LocalStatevector, Status: StatusAvailable, Detail: "pure-Go statevector"},
		{Name: CUDAQ, Status: cuda, Detail: detail},
		{Name: Stabilizer, Status: StatusAvailable, Detail: "Clifford tableau only (H, S, SDG, CNOT, CZ, X, Y, Z, measure), at most 32 qubits; non-Clifford gates are rejected"},
		tensor,
		density,
	}
}

// CUDAQInstalled reports whether python can import cudaq.
func CUDAQInstalled() bool {
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return false
		}
	}
	cmd := exec.Command(py, "-c", "import cudaq")
	return cmd.Run() == nil
}

// Run selects an engine. auto is the pure-Go statevector.
// Unavailable engines return an error instead of pretending to be GPU.
func Run(name, src string, shots int) (*Result, error) {
	if name == "" || name == Auto {
		name = LocalStatevector
	}
	switch name {
	case LocalStatevector:
		res, err := simulate.Run(src, shots)
		if err != nil {
			return nil, err
		}
		return &Result{Engine: LocalStatevector, Counts: res.Counts}, nil
	case Stabilizer:
		counts, err := RunStabilizer(src, shots, 1)
		if err != nil {
			return nil, err
		}
		return &Result{Engine: Stabilizer, Counts: counts}, nil
	case CUDAQ:
		if !CUDAQInstalled() {
			return nil, fmt.Errorf("engine cudaq is unavailable: CUDA-Q is not installed; local-statevector was not substituted")
		}
		counts, err := cudaq.Run(src, shots)
		if err != nil {
			return nil, fmt.Errorf("engine cudaq: %w", err)
		}
		return &Result{Engine: CUDAQ, Counts: counts}, nil
	case TensorNetwork:
		counts, err := runTensor(src, shots)
		if err != nil {
			return nil, err
		}
		return &Result{Engine: TensorNetwork, Counts: counts}, nil
	case DensityMatrix:
		counts, err := runDensity(src, shots)
		if err != nil {
			return nil, err
		}
		return &Result{Engine: DensityMatrix, Counts: counts}, nil
	default:
		return nil, fmt.Errorf("engine %s is unavailable", name)
	}
}
