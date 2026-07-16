// Copyright 2026 Magnobit, Inc. All rights reserved.

package simulate

import (
	"math"
	"testing"

	"github.com/magnobit/quell/internal/ir"
)

func almostEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestRun_XGate_Deterministic(t *testing.T) {
	res, err := Run("X 0\nMEASURE\n", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Counts) != 1 || res.Counts["1"] != 200 {
		t.Errorf("counts = %v, want exactly {\"1\": 200}", res.Counts)
	}
}

func TestRun_BellState_OnlyCorrelatedOutcomes(t *testing.T) {
	res, err := Run("H 0\nCNOT 0 1\nMEASURE\n", 2000)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range res.Counts {
		if k != "00" && k != "11" {
			t.Errorf("unexpected outcome %q (%d shots) — a Bell pair must only ever measure 00 or 11", k, v)
		}
	}
	total := res.Counts["00"] + res.Counts["11"]
	if total != 2000 {
		t.Errorf("total counts = %d, want 2000", total)
	}
	// Roughly 50/50 — generous tolerance since this is a real random sample.
	frac := float64(res.Counts["00"]) / 2000
	if !almostEqual(frac, 0.5, 0.1) {
		t.Errorf("|00> fraction = %.3f, want ~0.5", frac)
	}
}

func TestRun_GHZState_ThreeQubitCorrelation(t *testing.T) {
	res, err := Run("H 0\nCNOT 0 1\nCNOT 0 2\nMEASURE\n", 2000)
	if err != nil {
		t.Fatal(err)
	}
	for k := range res.Counts {
		if k != "000" && k != "111" {
			t.Errorf("unexpected GHZ outcome %q", k)
		}
	}
}

// TestRun_GroverExample matches the exact circuit shown on the Quell
// marketing page (Quell.jsx) — a real cross-check against what users are
// actually told to expect, not just an abstract unit test.
func TestRun_GroverExample_FindsTargetState(t *testing.T) {
	src := `H 0
H 1
CZ 0 1
H 0
H 1
X 0
X 1
CZ 0 1
X 0
X 1
H 0
H 1
MEASURE
`
	res, err := Run(src, 1000)
	if err != nil {
		t.Fatal(err)
	}
	frac := float64(res.Counts["11"]) / 1000
	if frac < 0.95 {
		t.Errorf("|11> fraction = %.3f, want ~1.0 (Grover's search on this 4-item oracle should find |11> with near-certainty)", frac)
	}
}

func TestRun_RZ_DoesNotChangeMeasurementProbabilities(t *testing.T) {
	// RZ only changes phase — measuring immediately after should still be
	// 50/50, same as H alone, since a Z-axis rotation doesn't move
	// probability mass between |0> and |1>.
	res, err := Run("H 0\nRZ 1.23 0\nMEASURE\n", 2000)
	if err != nil {
		t.Fatal(err)
	}
	frac := float64(res.Counts["1"]) / 2000
	if !almostEqual(frac, 0.5, 0.1) {
		t.Errorf("|1> fraction after H+RZ = %.3f, want ~0.5 (RZ shouldn't affect measurement probabilities)", frac)
	}
}

func TestRun_Reset_UnentangledQubit_ForcesZero(t *testing.T) {
	res, err := Run("X 0\nRESET 0\nMEASURE\n", 200)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["0"] != 200 {
		t.Errorf("counts = %v, want exactly {\"0\": 200} after X then RESET", res.Counts)
	}
}

func TestRun_Barrier_IsNoop(t *testing.T) {
	res, err := Run("X 0\nBARRIER\nMEASURE\n", 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["1"] != 100 {
		t.Errorf("counts = %v, want exactly {\"1\": 100} — BARRIER must not affect the state", res.Counts)
	}
}

func TestRun_TooManyQubits_Errors(t *testing.T) {
	p := &ir.Program{NumQubits: maxQubits + 1}
	if _, err := RunProgram(p, 100); err == nil {
		t.Fatal("expected an error for a qubit count above the simulator's limit")
	}
}

func TestRun_ParseError_Propagates(t *testing.T) {
	if _, err := Run("NOT_A_REAL_GATE 0\n", 10); err == nil {
		t.Fatal("expected a parse error for an unknown gate")
	}
}

func TestSample_ProbabilitiesSumToOne(t *testing.T) {
	sv := New(3)
	sv.H(0)
	sv.H(1)
	sv.CNOT(0, 2)
	sum := 0.0
	for _, p := range sv.Probs() {
		sum += p
	}
	if !almostEqual(sum, 1.0, 1e-9) {
		t.Errorf("probabilities sum to %v, want 1.0", sum)
	}
}
