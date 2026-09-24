// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/anneal"
	"github.com/magnobit/quell/internal/config"
)

func TestRunDWave_GateModelRejected(t *testing.T) {
	_, err := RunDWave(&config.DWaveConfig{}, "OPENQASM 3.0; qubit q; h q[0];")
	if err == nil {
		t.Fatal("gate Quell/OpenQASM must not succeed on D-Wave")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "qubo") && !strings.Contains(strings.ToLower(err.Error()), "anneal") {
		t.Errorf("error = %v, want it to point at the QUBO/anneal path", err)
	}
}

func TestRunDWaveQUBO_LocalSAWithoutToken(t *testing.T) {
	t.Setenv("DWAVE_API_TOKEN", "")
	p := &anneal.Problem{Linear: map[int]float64{0: 1}, NumVars: 1}
	res, err := RunDWaveQUBO(&config.DWaveConfig{Shots: 20}, p)
	if err != nil {
		t.Fatalf("local SA: %v", err)
	}
	if !res.FellBack {
		t.Fatal("no Leap token must use local fallback — do not claim live annealer success")
	}
	if res.Engine != "local-sa" {
		t.Errorf("engine = %q, want local-sa", res.Engine)
	}
	if res.Requested != "dwave" {
		t.Errorf("requested = %q, want dwave", res.Requested)
	}
}
