// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package estimate implements Quell's Quantum Digital Twin: local circuit
// stats, IR optimizer before/after metrics, teaching noise fidelity proxies,
// and educational multi-provider cost / queue / runtime estimates — without
// submitting to hardware.
package estimate

import (
	"fmt"
	"math"
	"sort"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/internal/topology"
	"github.com/magnobit/quell/simulate"
)

const Disclaimer = "Estimates only — not invoices or hardware SLAs. Costs use educational list proxies (" + CostModelVersion + "). Fidelity is a local noisy-sim overlap vs ideal, not device calibration."

// OptimizerDelta is before/after IR optimizer stats.
type OptimizerDelta struct {
	BeforeGates       int      `json:"beforeGates"`
	AfterGates        int      `json:"afterGates"`
	BeforeDepth       int      `json:"beforeDepth"`
	AfterDepth        int      `json:"afterDepth"`
	GateReductionPct  float64  `json:"gateReductionPct"`
	DepthReductionPct float64  `json:"depthReductionPct"`
	Notes             []string `json:"notes"`
}

// ProviderEstimate is one backend row in the digital-twin table.
type ProviderEstimate struct {
	Backend             string  `json:"backend"`
	EstimatedCostUSD    float64 `json:"estimatedCostUsd"`
	EstimatedQueueSec   float64 `json:"estimatedQueueSec"`
	EstimatedRuntimeSec float64 `json:"estimatedRuntimeSec"`
	EstimatedFidelity   float64 `json:"estimatedFidelity"`
	NoiseNote           string  `json:"noiseNote"`
	CostModelVersion    string  `json:"costModelVersion"`
}

// Recommendation ranks backends for cost, fidelity, and a balanced score.
type Recommendation struct {
	Cheapest     string `json:"cheapest"`
	BestFidelity string `json:"bestFidelity"`
	Balanced     string `json:"balanced"`
	Reason       string `json:"reason"`
}

// Report is the full digital-twin response.
type Report struct {
	Shots            int                `json:"shots"`
	Circuit          CircuitStats       `json:"circuit"`
	OptimizedCircuit CircuitStats       `json:"optimizedCircuit"`
	Optimizer        OptimizerDelta     `json:"optimizer"`
	Providers        []ProviderEstimate `json:"providers"`
	Recommendation   Recommendation     `json:"recommendation"`
	Disclaimer       string             `json:"disclaimer"`
}

// Options configures Analyze.
type Options struct {
	Shots      int
	Backends   []string // empty → GateBackends
	Seed       int64    // for reproducible noisy fidelity
	Coupling   string   // topology preset name, e.g. "heavyhex-toy", "linear-5"
	NoiseAware bool     // append noise-aware score note
}

// Analyze parses Quell source and builds a digital-twin estimate report.
func Analyze(src string, opt Options) (*Report, error) {
	circ, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	shots := opt.Shots
	if shots <= 0 {
		shots = 1000
	}
	backends := opt.Backends
	if len(backends) == 0 {
		backends = append([]string(nil), GateBackends...)
	}

	prog := ir.Lower(circ)
	before := StatsFromProgram(prog)
	optOpts := optimizer.Options{NoiseAware: opt.NoiseAware}
	if opt.Coupling != "" {
		cm, cerr := topology.Preset(opt.Coupling)
		if cerr != nil {
			return nil, cerr
		}
		optOpts.Coupling = cm
	}
	optProg, notes := optimizer.OptimizeWithOptions(prog, optOpts)
	after := StatsFromProgram(optProg)
	if notes == nil {
		notes = []string{}
	}

	delta := OptimizerDelta{
		BeforeGates: before.GateCount,
		AfterGates:  after.GateCount,
		BeforeDepth: before.Depth,
		AfterDepth:  after.Depth,
		Notes:       notes,
	}
	if before.GateCount > 0 {
		delta.GateReductionPct = pctDrop(before.GateCount, after.GateCount)
	}
	if before.Depth > 0 {
		delta.DepthReductionPct = pctDrop(before.Depth, after.Depth)
	}

	// Ideal probabilities once (cheap for ≤12 qubits).
	ideal, err := simulate.RunProgramOpts(optProg, simulate.Options{Shots: 1, Seed: opt.Seed})
	if err != nil {
		return nil, fmt.Errorf("ideal sim: %w", err)
	}

	providers := make([]ProviderEstimate, 0, len(backends))
	for _, b := range backends {
		m, ok := ModelFor(b)
		if !ok {
			continue
		}
		fid, ferr := fidelityProxy(optProg, ideal.Probs, m, opt.Seed)
		if ferr != nil {
			return nil, ferr
		}
		providers = append(providers, ProviderEstimate{
			Backend:             b,
			EstimatedCostUSD:    m.EstimateCostUSD(shots, after),
			EstimatedQueueSec:   m.EstimateQueueSec(),
			EstimatedRuntimeSec: m.EstimateRuntimeSec(shots),
			EstimatedFidelity:   fid,
			NoiseNote:           m.NoiseNote,
			CostModelVersion:    CostModelVersion,
		})
	}

	return &Report{
		Shots:            shots,
		Circuit:          before,
		OptimizedCircuit: after,
		Optimizer:        delta,
		Providers:        providers,
		Recommendation:   recommend(providers),
		Disclaimer:       Disclaimer,
	}, nil
}

func pctDrop(before, after int) float64 {
	if before <= 0 {
		return 0
	}
	v := 100 * float64(before-after) / float64(before)
	return math.Round(v*10) / 10
}

func fidelityProxy(prog *ir.Program, ideal []float64, m ProviderModel, seed int64) (float64, error) {
	noise := simulate.NoiseModel{
		Depolarizing: m.Depolarizing,
		ReadoutError: m.Readout,
	}
	// Use enough shots for a stable histogram overlap; cap for speed.
	shots := 512
	noisy, err := simulate.RunProgramOpts(prog, simulate.Options{Shots: shots, Noise: noise, Seed: seed + int64(len(m.Backend)*17)})
	if err != nil {
		return 0, fmt.Errorf("%s noisy sim: %w", m.Backend, err)
	}
	// Convert counts → empirical probs and Hellinger fidelity vs ideal.
	emp := make([]float64, len(ideal))
	for bit, n := range noisy.Counts {
		// bitstring as integer index
		idx := 0
		for i := 0; i < len(bit); i++ {
			idx = idx*2 + int(bit[i]-'0')
		}
		if idx >= 0 && idx < len(emp) {
			emp[idx] = float64(n) / float64(shots)
		}
	}
	return hellingerFidelity(ideal, emp), nil
}

func hellingerFidelity(p, q []float64) float64 {
	n := len(p)
	if len(q) < n {
		n = len(q)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += math.Sqrt(math.Max(0, p[i]) * math.Max(0, q[i]))
	}
	f := sum * sum
	if f > 1 {
		f = 1
	}
	return math.Round(f*1000) / 1000
}

func recommend(providers []ProviderEstimate) Recommendation {
	if len(providers) == 0 {
		return Recommendation{Reason: "no providers"}
	}
	byCost := append([]ProviderEstimate(nil), providers...)
	sort.Slice(byCost, func(i, j int) bool {
		if byCost[i].EstimatedCostUSD == byCost[j].EstimatedCostUSD {
			return byCost[i].EstimatedFidelity > byCost[j].EstimatedFidelity
		}
		return byCost[i].EstimatedCostUSD < byCost[j].EstimatedCostUSD
	})
	byFid := append([]ProviderEstimate(nil), providers...)
	sort.Slice(byFid, func(i, j int) bool {
		return byFid[i].EstimatedFidelity > byFid[j].EstimatedFidelity
	})

	// Balanced: maximize fidelity / (1 + cost) with a soft queue penalty.
	best := providers[0]
	bestScore := -1.0
	for _, p := range providers {
		score := p.EstimatedFidelity / (1 + p.EstimatedCostUSD) / (1 + p.EstimatedQueueSec/300)
		if score > bestScore {
			bestScore = score
			best = p
		}
	}

	return Recommendation{
		Cheapest:     byCost[0].Backend,
		BestFidelity: byFid[0].Backend,
		Balanced:     best.Backend,
		Reason: fmt.Sprintf(
			"Cheapest ≈ $%.2f (%s); best fidelity ≈ %.1f%% (%s); balanced pick %s",
			byCost[0].EstimatedCostUSD, byCost[0].Backend,
			byFid[0].EstimatedFidelity*100, byFid[0].Backend,
			best.Backend,
		),
	}
}
