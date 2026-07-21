// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/internal/topology"
)

// OptimizeResult is Quell source after conservative IR passes, plus deltas.
type OptimizeResult struct {
	Original  string         `json:"original"`
	Optimized string         `json:"optimized"`
	Optimizer OptimizerDelta `json:"optimizer"`
	Circuit   CircuitStats   `json:"circuit"`
	OptimizedCircuit CircuitStats `json:"optimizedCircuit"`
}

// OptimizeSource parses Quell, runs the IR optimizer, and emits Quell again.
func OptimizeSource(src string) (*OptimizeResult, error) {
	return OptimizeSourceOpts(src, OptimizeOpts{})
}

// OptimizeOpts configures OptimizeSourceOpts.
type OptimizeOpts struct {
	Coupling   string
	NoiseAware bool
}

// OptimizeSourceOpts is OptimizeSource with optional coupling / noise-aware passes.
func OptimizeSourceOpts(src string, o OptimizeOpts) (*OptimizeResult, error) {
	circ, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	prog := ir.Lower(circ)
	before := StatsFromProgram(prog)
	opts := optimizer.Options{NoiseAware: o.NoiseAware || o.Coupling != ""}
	if o.Coupling != "" {
		cm, cerr := topology.Preset(o.Coupling)
		if cerr != nil {
			return nil, cerr
		}
		opts.Coupling = cm
	}
	optProg, notes := optimizer.OptimizeWithOptions(prog, opts)
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
	out := ToQuell(optProg)
	if strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("optimize produced empty circuit")
	}
	return &OptimizeResult{
		Original:         src,
		Optimized:        out,
		Optimizer:        delta,
		Circuit:          before,
		OptimizedCircuit: after,
	}, nil
}
