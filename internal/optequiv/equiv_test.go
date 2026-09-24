// Copyright 2026 Magnobit, Inc. All rights reserved.

package optequiv_test

import (
	"math"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

func mustProg(t *testing.T, src string) *ir.Program {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return ir.Lower(c)
}

func TestIdenticalIR(t *testing.T) {
	p := mustProg(t, "H 0\nCNOT 0 1\nMEASURE")
	ev := optequiv.Compare(p, p, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
	if ev.OriginalIRHash == "" || ev.OriginalIRHash != ev.OptimizedIRHash {
		t.Fatalf("identical IR hashes: %s vs %s", ev.OriginalIRHash, ev.OptimizedIRHash)
	}
}

func TestGlobalPhaseOnly(t *testing.T) {
	// RZ(2π) = −I. |0⟩ vs −|0⟩: naive element-wise equality fails,
	// |<ψ|φ>|² = 1 succeeds.
	orig := mustProg(t, "H 0")
	phased := mustProg(t, "RZ 6.283185307179586 0\nH 0")
	a, err := simulate.EvolveUnitary(orig)
	if err != nil {
		t.Fatal(err)
	}
	b, err := simulate.EvolveUnitary(phased)
	if err != nil {
		t.Fatal(err)
	}
	psi, phi := a.Amplitudes(), b.Amplitudes()
	naive := true
	for i := range psi {
		if real((psi[i]-phi[i])*cmplxConj(psi[i]-phi[i])) > 1e-18 {
			naive = false
			break
		}
	}
	if naive {
		t.Fatal("fixture is not a global-phase-only pair — naive equality unexpectedly held")
	}
	ev := optequiv.Compare(orig, phased, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent || ev.VerificationTier != optequiv.TierExactStatevector {
		t.Fatalf("status=%s tier=%s reason=%s", ev.Status, ev.VerificationTier, ev.Reason)
	}
	if ev.StateFidelity == nil || *ev.StateFidelity < 1-optequiv.DefaultTolerance {
		t.Fatalf("fidelity=%v", ev.StateFidelity)
	}
}

func cmplxConj(z complex128) complex128 { return complex(real(z), -imag(z)) }

func TestXXCancellation(t *testing.T) {
	p := mustProg(t, "X 0\nX 0\nMEASURE")
	ev := optequiv.VerifyOptimization(p, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
	if ev.OriginalIRHash == ev.OptimizedIRHash {
		t.Fatal("X X should optimize to a different IR")
	}
}

func TestRotationMerge(t *testing.T) {
	p := mustProg(t, "PARAM a\nPARAM b\nRZ a 0\nRZ b 0\nMEASURE")
	ev := optequiv.VerifyOptimization(p, optequiv.Options{Params: map[string]float64{"a": 0.5, "b": 0.25}})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
	if ev.Reason == "" || ev.OriginalIRHash == "" {
		t.Fatal("expected hashes and reason")
	}
}

func TestDeadNoopElimination(t *testing.T) {
	p := mustProg(t, "RZ 0 0\nH 0\nMEASURE")
	ev := optequiv.VerifyOptimization(p, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestValidCommutingReorder(t *testing.T) {
	orig := mustProg(t, "H 0\nRZ 0.3 1\nMEASURE")
	reordered := mustProg(t, "RZ 0.3 1\nH 0\nMEASURE")
	ev := optequiv.Compare(orig, reordered, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestDeliberateBadOptimization(t *testing.T) {
	orig := mustProg(t, "H 0\nMEASURE")
	bad := mustProg(t, "X 0\nMEASURE")
	ev := optequiv.Compare(orig, bad, optequiv.Options{})
	if ev.Status != optequiv.StatusNotEquivalent {
		t.Fatalf("status=%s reason=%s — verifier must catch a bad transform", ev.Status, ev.Reason)
	}
}

func TestClassicalMappingMismatch(t *testing.T) {
	orig := mustProg(t, "H 0\nMEASURE 0 -> c[0]")
	wrong := mustProg(t, "H 0\nMEASURE 0 -> c[1]")
	ev := optequiv.Compare(orig, wrong, optequiv.Options{})
	if ev.Status != optequiv.StatusNotEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestMeasurementRemoval(t *testing.T) {
	orig := mustProg(t, "H 0\nMEASURE")
	removed := mustProg(t, "H 0")
	ev := optequiv.Compare(orig, removed, optequiv.Options{})
	if ev.Status != optequiv.StatusNotEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestLogicalRemapEquivalent(t *testing.T) {
	orig := mustProg(t, "H 0\nMEASURE 0 -> c[0]")
	remapped := mustProg(t, "SWAP 0 1\nH 1\nMEASURE 1 -> c[0]")
	ev := optequiv.Compare(orig, remapped, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s tier=%s reason=%s", ev.Status, ev.VerificationTier, ev.Reason)
	}
	if ev.VerificationTier != optequiv.TierExactDistribution {
		t.Fatalf("remap should use exact distribution, got %s", ev.VerificationTier)
	}
}

func TestBoundParameters(t *testing.T) {
	p := mustProg(t, "PARAM theta\nRX theta 0\nMEASURE")
	ev := optequiv.VerifyOptimization(p, optequiv.Options{Params: map[string]float64{"theta": 1.5708}})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestMissingParameters(t *testing.T) {
	p := mustProg(t, "PARAM theta\nRX theta 0\nMEASURE")
	ev := optequiv.VerifyOptimization(p, optequiv.Options{})
	if ev.Status != optequiv.StatusUnsupported {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestMultipleRotationBindings(t *testing.T) {
	p := mustProg(t, "PARAM a\nPARAM b\nRZ a 0\nRZ b 0\nMEASURE")
	for _, pair := range [][2]float64{{0.1, 0.2}, {1.0, -0.25}, {math.Pi / 3, math.Pi / 6}} {
		ev := optequiv.VerifyOptimization(p, optequiv.Options{Params: map[string]float64{"a": pair[0], "b": pair[1]}})
		if ev.Status != optequiv.StatusEquivalent {
			t.Fatalf("a=%v b=%v status=%s reason=%s", pair[0], pair[1], ev.Status, ev.Reason)
		}
	}
}

func TestDynamicUnsupported(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"mid-circuit", "H 0\nMEASURE 0\nX 0\nMEASURE"},
		{"conditional", "H 0\nMEASURE 0\nIF c[0]==1 X 1\nMEASURE"},
		{"reset", "X 0\nRESET 0\nMEASURE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustProg(t, tc.src)
			opt, _ := optimizer.Optimize(p)
			ev := optequiv.Compare(p, opt, optequiv.Options{})
			if ev.Status != optequiv.StatusUnsupported || ev.VerificationTier != optequiv.TierUnsupported {
				t.Fatalf("status=%s tier=%s reason=%s", ev.Status, ev.VerificationTier, ev.Reason)
			}
		})
	}
}

func TestExactQubitLimit(t *testing.T) {
	p := mustProg(t, "H 0\nMEASURE")
	ev := optequiv.Compare(p, p, optequiv.Options{MaxExactQubits: 0})
	// Identical IR short-circuits before the qubit ceiling.
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("identical IR should still be equivalent: %s", ev.Status)
	}
	orig := mustProg(t, "H 0\nH 1\nX 0\nX 0\nMEASURE")
	opt, _ := optimizer.Optimize(orig)
	ev = optequiv.Compare(orig, opt, optequiv.Options{MaxExactQubits: 1})
	if ev.VerificationTier != optequiv.TierStatistical {
		t.Fatalf("tier=%s reason=%s", ev.VerificationTier, ev.Reason)
	}
	if ev.Status != optequiv.StatusInconclusive {
		t.Fatalf("without P2A, statistical must be inconclusive, got %s", ev.Status)
	}
}

func TestUnsupportedGate(t *testing.T) {
	p := &ir.Program{NumQubits: 1, Ops: []ir.Op{{Kind: "FOO", Qubits: []int{0}}, {Kind: ir.OpMEASURE}}}
	ev := optequiv.Compare(p, p, optequiv.Options{})
	if ev.Status != optequiv.StatusUnsupported {
		t.Fatalf("unsupported gate must not be treated as equivalent: %s %s", ev.Status, ev.Reason)
	}
	other := &ir.Program{NumQubits: 1, Ops: []ir.Op{{Kind: "FOO", Qubits: []int{0}}, {Kind: ir.OpX, Qubits: []int{0}}, {Kind: ir.OpMEASURE}}}
	ev = optequiv.Compare(p, other, optequiv.Options{})
	if ev.Status != optequiv.StatusUnsupported {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}

func TestToleranceTinyDriftVsSemanticChange(t *testing.T) {
	orig := mustProg(t, "H 0")
	drift := mustProg(t, "RZ 1e-16 0\nH 0")
	ev := optequiv.Compare(orig, drift, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("tiny drift should pass: %s %s", ev.Status, ev.Reason)
	}
	changed := mustProg(t, "X 0")
	ev = optequiv.Compare(orig, changed, optequiv.Options{})
	if ev.Status != optequiv.StatusNotEquivalent {
		t.Fatalf("semantic change should fail: %s", ev.Status)
	}
}

func TestDeterministicEvidence(t *testing.T) {
	p := mustProg(t, "X 0\nX 0\nMEASURE")
	a := optequiv.VerifyOptimization(p, optequiv.Options{})
	b := optequiv.VerifyOptimization(p, optequiv.Options{})
	if a.Status != b.Status || a.VerificationTier != b.VerificationTier {
		t.Fatalf("status/tier drifted: %+v vs %+v", a, b)
	}
	if a.OriginalIRHash != b.OriginalIRHash || a.OptimizedIRHash != b.OptimizedIRHash {
		t.Fatal("IR hashes are not deterministic")
	}
	if a.OptimizerVersion == "" || len(a.Passes) == 0 {
		t.Fatal("optimizer version/passes must be captured")
	}
}

func TestPassBoundaryDebug(t *testing.T) {
	orig := mustProg(t, "H 0\nMEASURE")
	bad := mustProg(t, "X 0\nMEASURE")
	ev := optequiv.Compare(orig, bad, optequiv.Options{PassBoundaryDebug: true})
	if ev.Status != optequiv.StatusNotEquivalent {
		t.Fatalf("status=%s", ev.Status)
	}
}

func TestStatisticalHookReusesInjectedP2A(t *testing.T) {
	orig := mustProg(t, "H 0\nX 0\nX 0\nMEASURE")
	opt, _ := optimizer.Optimize(orig)
	called := false
	ev := optequiv.Compare(orig, opt, optequiv.Options{
		MaxExactQubits: -1,
		StatisticalCompare: func(ref, cand map[string]float64) *optequiv.StatisticalEvidence {
			called = true
			return &optequiv.StatisticalEvidence{
				Method:        "pooled_multinomial_bootstrap",
				MethodVersion: "v1",
				Status:        "AGREES",
				Reason:        "injected",
			}
		},
	})
	if !called {
		t.Fatal("P2A hook was not used")
	}
	if ev.VerificationTier != optequiv.TierStatistical {
		t.Fatalf("tier=%s", ev.VerificationTier)
	}
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s", ev.Status)
	}
	if ev.Reason != "No statistically significant behavioral difference detected." {
		t.Fatalf("reason=%q", ev.Reason)
	}
	if ev.StatisticalEvidence == nil || ev.StatisticalEvidence.Method != "pooled_multinomial_bootstrap" {
		t.Fatal("P2A evidence must be attached, not recomputed")
	}
}

func TestPerformanceRepresentativeSizes(t *testing.T) {
	cases := []string{
		"X 0\nX 0\nMEASURE",
		"H 0\nCNOT 0 1\nMEASURE",
		"H 0\nH 1\nH 2\nCNOT 0 1\nCNOT 1 2\nMEASURE",
	}
	for _, src := range cases {
		p := mustProg(t, src)
		ev := optequiv.VerifyOptimization(p, optequiv.Options{})
		t.Logf("%q qubits=%d durationMs=%d status=%s tier=%s", src, p.NumQubits, ev.DurationMs, ev.Status, ev.VerificationTier)
		if ev.Status != optequiv.StatusEquivalent && ev.Status != optequiv.StatusUnsupported {
			t.Fatalf("unexpected status %s", ev.Status)
		}
	}
}

func TestOptimizerPreservesNoiseHeader(t *testing.T) {
	p := &ir.Program{NumQubits: 1, NoiseDepolarizing: 0.01, Ops: []ir.Op{{Kind: ir.OpH, Qubits: []int{0}}, {Kind: ir.OpMEASURE}}}
	got, _ := optimizer.Optimize(p)
	if got.NoiseDepolarizing != 0.01 {
		t.Fatalf("noise header dropped: %+v", got)
	}
}
