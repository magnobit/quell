// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"

	"github.com/magnobit/quell/internal/config"
)

// RunDWave always returns an error. D-Wave builds quantum annealers, which
// solve QUBO/Ising optimization problems by relaxing a system into its
// ground state — they have no gate-model execution path at all, so a
// compiled OpenQASM 3 gate circuit cannot be submitted to a D-Wave solver.
// Supporting D-Wave properly would require a separate QUBO/Ising program
// representation (and a corresponding Quell surface syntax to author them),
// which is out of scope for the gate-based Quell compiler today. This
// adapter exists so the config surface (config.DWaveConfig) and CLI backend
// list are complete, without pretending a working submission is possible.
func RunDWave(cfg *config.DWaveConfig, qasm3 string) (*RunResult, error) {
	return nil, fmt.Errorf("dwave: D-Wave is a quantum annealer (QUBO/Ising), not a gate-model device — a compiled .quell gate circuit cannot run on it as-is; annealer support needs a separate QUBO/Ising program representation, not yet implemented")
}
