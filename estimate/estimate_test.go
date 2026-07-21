// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "testing"

func TestAnalyzeBell(t *testing.T) {
	src := "H 0\nCNOT 0 1\nMEASURE\n"
	r, err := Analyze(src, Options{Shots: 500, Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if r.Circuit.NumQubits != 2 {
		t.Fatalf("qubits=%d", r.Circuit.NumQubits)
	}
	if r.Circuit.GateCount < 2 {
		t.Fatalf("gates=%d", r.Circuit.GateCount)
	}
	if len(r.Providers) != 4 {
		t.Fatalf("providers=%d", len(r.Providers))
	}
	if r.Recommendation.Balanced == "" {
		t.Fatal("missing balanced recommendation")
	}
	for _, p := range r.Providers {
		if p.EstimatedFidelity <= 0 || p.EstimatedFidelity > 1 {
			t.Fatalf("%s fidelity=%v", p.Backend, p.EstimatedFidelity)
		}
		if p.EstimatedCostUSD < 0 {
			t.Fatalf("%s cost=%v", p.Backend, p.EstimatedCostUSD)
		}
	}
}

func TestOptimizerDelta(t *testing.T) {
	// X X cancels → fewer gates after optimize
	src := "X 0\nX 0\nH 0\nMEASURE\n"
	r, err := Analyze(src, Options{Shots: 100, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.Optimizer.AfterGates >= r.Optimizer.BeforeGates {
		t.Fatalf("expected gate reduction: before=%d after=%d notes=%v",
			r.Optimizer.BeforeGates, r.Optimizer.AfterGates, r.Optimizer.Notes)
	}
	if r.Optimizer.GateReductionPct <= 0 {
		t.Fatalf("expected positive gateReductionPct, got %v", r.Optimizer.GateReductionPct)
	}
}
