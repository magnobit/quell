// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package optequiv verifies that conservative optimizer transformations
// preserve program semantics. It compares an original IR program to an
// optimized IR program. This is not provider verification, hardware-noise
// verification, or error mitigation.
//
// Tiers:
//   - EXACT_STATEVECTOR — pure-state circuits; states compared up to global phase
//   - EXACT_DISTRIBUTION — exact classical outcome probabilities
//   - STATISTICAL — P2A pooled-multinomial bootstrap, injected by the caller
//   - UNSUPPORTED — semantics cannot be verified safely
//
// STATISTICAL agreement is weaker than exact equivalence. Callers must not
// present it as a mathematical proof. This package does not implement a
// second statistical engine.
package optequiv

import (
	"fmt"
	"math"
	"time"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/simulate"
)

const (
	StatusEquivalent    = "EQUIVALENT"
	StatusNotEquivalent = "NOT_EQUIVALENT"
	StatusInconclusive  = "INCONCLUSIVE"
	StatusUnsupported   = "UNSUPPORTED"

	TierExactStatevector  = "EXACT_STATEVECTOR"
	TierExactDistribution = "EXACT_DISTRIBUTION"
	TierStatistical       = "STATISTICAL"
	TierUnsupported       = "UNSUPPORTED"

	// DefaultTolerance is the decision threshold for both
	// |<ψ|φ>|² ≥ 1−tol and exact probability comparison.
	// Double-precision statevector residual on ≤12 qubits is typically
	// 1e-14–1e-12; 1e-9 is ~1000× that floor and still far below any
	// semantically distinct computational-basis change.
	DefaultTolerance = 1e-9

	// MaxExactQubits is the default ceiling for exact amplitude / probability
	// comparison. The local simulator can hold 24 qubits (256MB), but two
	// full vectors plus comparison is too expensive for on-demand
	// verification. 2^12 amplitudes is 64KiB per vector.
	MaxExactQubits = 12

	// MaxStatisticalQubits caps sampled fallback. Above this the simulator
	// may still run, but on-demand verification refuses the workload.
	MaxStatisticalQubits = 16

	MaxOps           = 4096
	DefaultStatShots = 256
	OptimizerSchema  = "quell-optimizer-v1"
)

// StatisticalEvidence is a P2A evidence payload attached by the caller.
// This package never computes p-values or TVD-under-null itself.
type StatisticalEvidence struct {
	Method              string  `json:"method,omitempty"`
	MethodVersion       string  `json:"methodVersion,omitempty"`
	ReferenceShots      int     `json:"referenceShots,omitempty"`
	CandidateShots      int     `json:"candidateShots,omitempty"`
	ObservedTVD         float64 `json:"observedTvd,omitempty"`
	ExpectedSamplingTVD float64 `json:"expectedSamplingTvd,omitempty"`
	CriticalTVD         float64 `json:"criticalTvd,omitempty"`
	PValue              float64 `json:"pValue,omitempty"`
	Alpha               float64 `json:"alpha,omitempty"`
	Iterations          int     `json:"iterations,omitempty"`
	Seed                int64   `json:"seed,omitempty"`
	Status              string  `json:"status,omitempty"`
	Reason              string  `json:"reason,omitempty"`
	GeneratedAt         string  `json:"generatedAt,omitempty"`
	SupportSize         int     `json:"supportSize,omitempty"`
	ReferenceKind       string  `json:"referenceKind,omitempty"`
}

// StatisticalCompare is the P2A hook. Inputs are sampled count maps.
type StatisticalCompare func(reference, candidate map[string]float64) *StatisticalEvidence

// Options control verification. Zero values use the documented defaults.
type Options struct {
	Tolerance            float64
	MaxExactQubits       int
	MaxStatisticalQubits int
	Params               map[string]float64
	OptimizerVersion     string
	StatisticalCompare   StatisticalCompare
	StatShots            int
	StatSeed             int64
	PassBoundaryDebug    bool
}

func (o Options) resolved() Options {
	if o.Tolerance <= 0 || math.IsNaN(o.Tolerance) || math.IsInf(o.Tolerance, 0) {
		o.Tolerance = DefaultTolerance
	}
	if o.MaxExactQubits == 0 {
		o.MaxExactQubits = MaxExactQubits
	}
	if o.MaxStatisticalQubits <= 0 {
		o.MaxStatisticalQubits = MaxStatisticalQubits
	}
	if o.MaxExactQubits > simulate.MaxQubits() {
		o.MaxExactQubits = simulate.MaxQubits()
	}
	if o.MaxStatisticalQubits > simulate.MaxQubits() {
		o.MaxStatisticalQubits = simulate.MaxQubits()
	}
	if o.OptimizerVersion == "" {
		o.OptimizerVersion = OptimizerSchema
	}
	if o.StatShots <= 0 {
		o.StatShots = DefaultStatShots
	}
	if o.StatSeed == 0 {
		o.StatSeed = 42
	}
	return o
}

// Evidence is the structured optimizer-equivalence result. Only fields
// relevant to the selected tier are populated.
type Evidence struct {
	Status              string               `json:"status"`
	VerificationTier    string               `json:"verificationTier"`
	OriginalIRHash      string               `json:"originalIrHash"`
	OptimizedIRHash     string               `json:"optimizedIrHash"`
	OptimizerVersion    string               `json:"optimizerVersion,omitempty"`
	Passes              []string             `json:"passes,omitempty"`
	StateFidelity       *float64             `json:"stateFidelity,omitempty"`
	MaxAmplitudeDelta   *float64             `json:"maxAmplitudeDelta,omitempty"`
	DistributionTVD     *float64             `json:"distributionTvd,omitempty"`
	MaxProbabilityDelta *float64             `json:"maxProbabilityDelta,omitempty"`
	StatisticalEvidence *StatisticalEvidence `json:"statisticalEvidence,omitempty"`
	Tolerance           float64              `json:"tolerance,omitempty"`
	Reason              string               `json:"reason"`
	VerifiedAt          string               `json:"verifiedAt"`
	SuspectedPass       string               `json:"suspectedPass,omitempty"`
	DurationMs          int                  `json:"durationMs,omitempty"`
}

func newEvidence(opt Options, orig, optimized *ir.Program) Evidence {
	ev := Evidence{
		Passes:           append([]string(nil), optimizer.PassNames()...),
		OptimizerVersion: opt.OptimizerVersion,
		Tolerance:        opt.Tolerance,
		VerifiedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if orig != nil {
		ev.OriginalIRHash = ir.Hash(orig)
	}
	if optimized != nil {
		ev.OptimizedIRHash = ir.Hash(optimized)
	}
	return ev
}

func (e *Evidence) finish(status, tier, reason string) Evidence {
	e.Status = status
	e.VerificationTier = tier
	e.Reason = reason
	return *e
}

// VerifyOptimization binds params if needed, runs the conservative
// optimizer, and compares original vs optimized IR.
func VerifyOptimization(p *ir.Program, opt Options) Evidence {
	start := time.Now()
	opt = opt.resolved()
	orig, err := prepare(p, opt.Params)
	if err != nil {
		ev := newEvidence(opt, p, nil)
		ev.DurationMs = int(time.Since(start).Milliseconds())
		return ev.finish(StatusUnsupported, TierUnsupported, err.Error())
	}
	optimized, _ := optimizer.Optimize(orig)
	ev := Compare(orig, optimized, opt)
	ev.DurationMs = int(time.Since(start).Milliseconds())
	if opt.PassBoundaryDebug && ev.Status == StatusNotEquivalent {
		ev.SuspectedPass = suspectPass(orig, opt)
	}
	return ev
}

// Compare verifies two already-built IR programs. optimized is treated as
// the candidate (it is not re-optimized).
func Compare(original, optimized *ir.Program, opt Options) Evidence {
	start := time.Now()
	opt = opt.resolved()
	ev := newEvidence(opt, original, optimized)
	defer func() { ev.DurationMs = int(time.Since(start).Milliseconds()) }()

	if original == nil || optimized == nil {
		return ev.finish(StatusUnsupported, TierUnsupported, "Both original and optimized IR are required.")
	}
	if n := countOps(original.Ops) + countOps(optimized.Ops); n > MaxOps*2 {
		return ev.finish(StatusUnsupported, TierUnsupported, fmt.Sprintf("Circuit is too large to verify (%d ops).", n))
	}

	orig, err := prepare(original, opt.Params)
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, err.Error())
	}
	cand, err := prepare(optimized, opt.Params)
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, err.Error())
	}
	ev.OriginalIRHash = ir.Hash(orig)
	ev.OptimizedIRHash = ir.Hash(cand)

	oc := classify(orig)
	cc := classify(cand)
	if oc.reason != "" {
		return ev.finish(StatusUnsupported, TierUnsupported, oc.reason)
	}
	if cc.reason != "" {
		return ev.finish(StatusUnsupported, TierUnsupported, cc.reason)
	}

	if ev.OriginalIRHash == ev.OptimizedIRHash {
		return ev.finish(StatusEquivalent, TierExactStatevector, "Canonical IR is identical.")
	}

	n := orig.NumQubits
	if cand.NumQubits > n {
		n = cand.NumQubits
	}
	if n < 1 {
		n = 1
	}
	if n > opt.MaxStatisticalQubits {
		return ev.finish(StatusUnsupported, TierUnsupported,
			fmt.Sprintf("%d qubits exceeds the verification ceiling of %d.", n, opt.MaxStatisticalQubits))
	}

	if oc.noisy || cc.noisy || n > opt.MaxExactQubits || opt.MaxExactQubits < 0 {
		return statistical(orig, cand, &ev, opt)
	}

	var out Evidence
	if oc.hasMeasure || cc.hasMeasure {
		out = exactDistribution(orig, cand, oc, cc, &ev, opt)
	} else {
		out = exactStatevector(orig, cand, &ev, opt)
	}
	if opt.PassBoundaryDebug && out.Status == StatusNotEquivalent && out.SuspectedPass == "" {
		out.SuspectedPass = suspectPass(orig, opt)
	}
	return out
}

func prepare(p *ir.Program, params map[string]float64) (*ir.Program, error) {
	if p == nil {
		return nil, fmt.Errorf("program is required")
	}
	if ir.NeedsBind(p) {
		if len(params) == 0 {
			return nil, fmt.Errorf("circuit has unbound parameters %v — bind concrete values before claiming equivalence (not a symbolic proof)", ir.UnboundParams(p))
		}
		bound, err := ir.Bind(p, params)
		if err != nil {
			return nil, fmt.Errorf("%v — missing required binding is not treated as equivalence", err)
		}
		return bound, nil
	}
	return p, nil
}

func exactStatevector(orig, cand *ir.Program, ev *Evidence, opt Options) Evidence {
	a, err := simulate.EvolveUnitary(orig)
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, "Original circuit cannot be simulated exactly: "+err.Error())
	}
	b, err := simulate.EvolveUnitary(cand)
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, "Optimized circuit cannot be simulated exactly: "+err.Error())
	}
	fid, maxAmp := stateFidelity(a.Amplitudes(), b.Amplitudes())
	ev.StateFidelity = &fid
	ev.MaxAmplitudeDelta = &maxAmp
	if fid+opt.Tolerance >= 1 {
		return ev.finish(StatusEquivalent, TierExactStatevector, "States are equivalent up to global phase.")
	}
	return ev.finish(StatusNotEquivalent, TierExactStatevector, "Optimized program differs from original semantics.")
}

func exactDistribution(orig, cand *ir.Program, oc, cc class, ev *Evidence, opt Options) Evidence {
	if oc.hasMeasure != cc.hasMeasure {
		return ev.finish(StatusNotEquivalent, TierExactDistribution, "Measurement was added or removed; optimized program differs from original semantics.")
	}
	if oc.midCircuit || cc.midCircuit {
		return ev.finish(StatusUnsupported, TierUnsupported, "Mid-circuit measurement cannot be verified exactly.")
	}
	op, err := classicalProbs(orig)
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, err.Error())
	}
	cp, err := classicalProbs(cand)
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, err.Error())
	}
	tvd, maxD := distDistance(op, cp)
	ev.DistributionTVD = &tvd
	ev.MaxProbabilityDelta = &maxD

	// Same measurement plan + same NumQubits: also compare the unitary
	// prefix up to global phase so a relative-phase-only cheat cannot hide
	// behind identical Z-basis probabilities when that is the intended check.
	// When the plan remaps classical bits, distribution is the authority.
	samePlan := measurementKey(orig) == measurementKey(cand) && orig.NumQubits == cand.NumQubits
	if samePlan {
		if a, e1 := simulate.EvolveUnitary(orig); e1 == nil {
			if b, e2 := simulate.EvolveUnitary(cand); e2 == nil {
				fid, maxAmp := stateFidelity(a.Amplitudes(), b.Amplitudes())
				ev.StateFidelity = &fid
				ev.MaxAmplitudeDelta = &maxAmp
				if fid+opt.Tolerance < 1 {
					return ev.finish(StatusNotEquivalent, TierExactStatevector, "Optimized program differs from original semantics.")
				}
				if tvd <= opt.Tolerance && maxD <= opt.Tolerance {
					return ev.finish(StatusEquivalent, TierExactStatevector, "States are equivalent up to global phase.")
				}
			}
		}
	}
	if tvd <= opt.Tolerance && maxD <= opt.Tolerance {
		return ev.finish(StatusEquivalent, TierExactDistribution, "Exact outcome probabilities match over the union of measured bitstrings.")
	}
	return ev.finish(StatusNotEquivalent, TierExactDistribution, "Optimized program differs from original semantics.")
}

func statistical(orig, cand *ir.Program, ev *Evidence, opt Options) Evidence {
	if opt.StatisticalCompare == nil {
		return ev.finish(StatusInconclusive, TierStatistical, "Exact comparison is unavailable; statistical fallback requires the P2A engine.")
	}
	a, err := simulate.RunProgramOpts(orig, simulate.Options{Shots: opt.StatShots, Seed: opt.StatSeed})
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, "Original circuit cannot be sampled: "+err.Error())
	}
	b, err := simulate.RunProgramOpts(cand, simulate.Options{Shots: opt.StatShots, Seed: opt.StatSeed + 1})
	if err != nil {
		return ev.finish(StatusUnsupported, TierUnsupported, "Optimized circuit cannot be sampled: "+err.Error())
	}
	ref := countsToFloat(a.Counts)
	candCounts := countsToFloat(b.Counts)
	se := opt.StatisticalCompare(ref, candCounts)
	ev.StatisticalEvidence = se
	if se == nil {
		return ev.finish(StatusInconclusive, TierStatistical, "P2A returned no statistical evidence.")
	}
	switch se.Status {
	case "AGREES":
		return ev.finish(StatusEquivalent, TierStatistical, "No statistically significant behavioral difference detected.")
	case "DISAGREES":
		return ev.finish(StatusNotEquivalent, TierStatistical, "Optimized program differs from original semantics.")
	case "INSUFFICIENT_EVIDENCE":
		return ev.finish(StatusInconclusive, TierStatistical, "Statistical evidence is insufficient to decide optimizer equivalence.")
	default:
		return ev.finish(StatusInconclusive, TierStatistical, "Statistical comparison is inconclusive; this is not exact equivalence.")
	}
}

func countsToFloat(in map[string]int) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = float64(v)
	}
	return out
}

func suspectPass(orig *ir.Program, opt Options) string {
	// Apply the same four conservative steps and compare after each.
	// The first failing boundary is a suspicion, not a proven cause.
	type step struct {
		name string
		fn   func(*ir.Program) (*ir.Program, []string)
	}
	// Use exported Optimize pieces via successive Optimize of prefixes
	// is not available; reconstruct by calling Optimize incrementally
	// is wrong. The pass functions are unexported. Compare after the
	// full Optimize only unless we export a debug helper.
	//
	// PassBoundary is implemented via OptimizeDebugSteps.
	cur := orig
	for _, s := range debugSteps() {
		next, _ := s.fn(cur)
		probe := Compare(orig, next, Options{
			Tolerance:            opt.Tolerance,
			MaxExactQubits:       opt.MaxExactQubits,
			MaxStatisticalQubits: opt.MaxStatisticalQubits,
			OptimizerVersion:     opt.OptimizerVersion,
		})
		if probe.Status == StatusNotEquivalent {
			return "first suspected pass boundary: " + s.name + " (not verified as the cause)"
		}
		cur = next
	}
	return "first suspected pass boundary: unknown (not verified as the cause)"
}
