// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"

	"github.com/magnobit/quell/internal/config"
)

// RunDWave always returns an error for gate-model OpenQASM. D-Wave builds
// quantum annealers (QUBO/Ising). Author annealer problems via package
// github.com/magnobit/quell/anneal (QUBO text format) — Leap submission is
// not wired yet. This adapter remains so config/CLI/Cloud lists stay complete.
func RunDWave(cfg *config.DWaveConfig, qasm3 string) (*RunResult, error) {
	_ = cfg
	_ = qasm3
	return nil, fmt.Errorf("dwave: D-Wave is an annealer — gate Quell/.qasm cannot run here; define a QUBO with package quell/anneal (ParseQUBO), then submit via Leap once annealer execution ships")
}
