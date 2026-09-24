// Copyright 2026 Magnobit, Inc. All rights reserved.

package optequiv

import (
	"fmt"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/simulate"
)

type class struct {
	hasMeasure bool
	midCircuit bool
	noisy      bool
	reason     string
}

func classify(p *ir.Program) class {
	var c class
	if p == nil {
		c.reason = "Program is required."
		return c
	}
	if ir.NeedsBind(p) {
		c.reason = fmt.Sprintf("circuit has unbound parameters %v — bind concrete values before claiming equivalence (not a symbolic proof)", ir.UnboundParams(p))
		return c
	}
	if p.NoiseDepolarizing > 0 || p.NoiseAmplitudeDamping > 0 || p.NoisePhaseDamping > 0 || p.NoiseReadout > 0 {
		c.noisy = true
	}
	seenMeasure := false
	var walk func([]ir.Op)
	walk = func(ops []ir.Op) {
		for _, op := range ops {
			switch op.Kind {
			case ir.OpRESET:
				c.reason = "RESET cannot be verified safely (not a unitary semantics the exact tiers support)."
				return
			case ir.OpIF:
				c.reason = "Conditional gates cannot be verified safely."
				return
			case ir.OpWHILE:
				c.reason = "Looping control cannot be verified safely."
				return
			case ir.OpSWITCH:
				c.reason = "SWITCH control cannot be verified safely."
				return
			case ir.OpASSERT:
				c.reason = "ASSERT cannot be verified safely."
				return
			case ir.OpMEASURE:
				c.hasMeasure = true
				seenMeasure = true
			default:
				if seenMeasure && op.Kind != ir.OpBARRIER {
					c.midCircuit = true
					c.reason = "Mid-circuit measurement cannot be verified safely."
					return
				}
			}
			if op.Body != nil {
				walk([]ir.Op{*op.Body})
			}
			if c.reason != "" {
				return
			}
			walk(op.Then)
			if c.reason != "" {
				return
			}
			walk(op.Else)
			for _, arm := range op.Cases {
				walk(arm.Body)
				if c.reason != "" {
					return
				}
			}
		}
	}
	walk(p.Ops)
	if c.reason != "" {
		return c
	}
	if _, err := simulate.EvolveUnitary(stripMeasures(p)); err != nil {
		c.reason = "Unsupported operation: " + err.Error()
	}
	return c
}

func stripMeasures(p *ir.Program) *ir.Program {
	if p == nil {
		return nil
	}
	out := *p
	var ops []ir.Op
	for _, op := range p.Ops {
		if op.Kind == ir.OpMEASURE {
			break
		}
		ops = append(ops, op)
	}
	out.Ops = ops
	return &out
}

func countOps(ops []ir.Op) int {
	n := 0
	var walk func([]ir.Op)
	walk = func(ops []ir.Op) {
		for _, op := range ops {
			n++
			if op.Body != nil {
				walk([]ir.Op{*op.Body})
			}
			walk(op.Then)
			walk(op.Else)
			for _, arm := range op.Cases {
				walk(arm.Body)
			}
		}
	}
	walk(ops)
	return n
}
