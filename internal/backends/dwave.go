// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/magnobit/quell/anneal"
	"github.com/magnobit/quell/internal/config"
)

// RunDWave always errors for gate-model OpenQASM — use RunDWaveQUBO.
func RunDWave(cfg *config.DWaveConfig, qasm3 string) (*RunResult, error) {
	_ = cfg
	_ = qasm3
	return nil, fmt.Errorf("dwave: gate Quell/OpenQASM cannot run on an annealer — submit QUBO via RunDWaveQUBO / `quell anneal run` / Cloud execute with kind=qubo")
}

// RunDWaveQUBO submits a QUBO problem. Prefer Leap when a token is set and
// Ocean is installed; otherwise uses classical simulated annealing so the
// pipeline is always exercisable end-to-end.
func RunDWaveQUBO(cfg *config.DWaveConfig, problem *anneal.Problem) (*RunResult, error) {
	if cfg == nil {
		cfg = &config.DWaveConfig{}
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 100
	}
	token := cfg.APIToken
	if token == "" {
		token = os.Getenv("DWAVE_API_TOKEN")
	}

	var (
		res *anneal.Result
		err error
	)
	if token != "" {
		res, err = anneal.SampleLeap(token, cfg.Solver, problem, shots)
		if err != nil {
			if os.Getenv("QUELL_DWAVE_REQUIRE_LEAP") == "1" {
				return nil, err
			}
			local, lerr := anneal.SampleLocal(problem, shots, 0)
			if lerr != nil {
				return nil, fmt.Errorf("%v; local fallback also failed: %w", err, lerr)
			}
			local.Info = fmt.Sprintf("Leap unavailable (%v) — %s", err, local.Info)
			res = local
		}
	} else {
		res, err = anneal.SampleLocal(problem, shots, 0)
		if err != nil {
			return nil, err
		}
	}

	backend := "D-Wave / local-SA"
	if res != nil {
		if strings.Contains(res.Info, "Leap") && !strings.Contains(res.Info, "unavailable") {
			backend = "D-Wave / " + res.Info
		} else {
			backend = "D-Wave / " + res.Info
		}
	}

	return &RunResult{
		JobID:   "dwave-" + strconv.Itoa(shots),
		Backend: backend,
		Shots:   shots,
		Counts:  res.CountsMap(),
	}, nil
}
