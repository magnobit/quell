// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"github.com/magnobit/quell/internal/ir"
)

// CircuitStats summarizes a lowered IR program for digital-twin estimates.
type CircuitStats struct {
	NumQubits     int `json:"numQubits"`
	GateCount     int `json:"gateCount"`
	TwoQubitGates int `json:"twoQubitGates"`
	Depth         int `json:"depth"`
}

// StatsFromProgram computes gate count, two-qubit count, and critical-path depth.
func StatsFromProgram(p *ir.Program) CircuitStats {
	if p == nil {
		return CircuitStats{}
	}
	two := 0
	for _, op := range p.Ops {
		if isTwoQubit(op.Kind) {
			two++
		}
		if op.Kind == ir.OpIF && op.Body != nil && isTwoQubit(op.Body.Kind) {
			two++
		}
	}
	return CircuitStats{
		NumQubits:     p.NumQubits,
		GateCount:     len(p.Ops),
		TwoQubitGates: two,
		Depth:         Depth(p),
	}
}

// Depth is the critical-path depth (longest sequential gate chain across qubits).
func Depth(p *ir.Program) int {
	if p == nil {
		return 0
	}
	nq := p.NumQubits
	if nq <= 0 {
		return len(p.Ops)
	}
	qubitDepth := make([]int, nq)
	for _, op := range p.Ops {
		qs := touched(op, nq)
		if len(qs) == 0 {
			maxD := 0
			for _, d := range qubitDepth {
				if d > maxD {
					maxD = d
				}
			}
			for i := range qubitDepth {
				qubitDepth[i] = maxD + 1
			}
			continue
		}
		maxD := 0
		for _, q := range qs {
			if q >= 0 && q < nq && qubitDepth[q] > maxD {
				maxD = qubitDepth[q]
			}
		}
		for _, q := range qs {
			if q >= 0 && q < nq {
				qubitDepth[q] = maxD + 1
			}
		}
	}
	maxD := 0
	for _, d := range qubitDepth {
		if d > maxD {
			maxD = d
		}
	}
	return maxD
}

func touched(op ir.Op, nq int) []int {
	if op.Kind == ir.OpIF {
		out := make([]int, nq)
		for i := range out {
			out[i] = i
		}
		return out
	}
	if len(op.Qubits) == 0 && (op.Kind == ir.OpMEASURE || op.Kind == ir.OpBARRIER) {
		out := make([]int, nq)
		for i := range out {
			out[i] = i
		}
		return out
	}
	return op.Qubits
}

func isTwoQubit(k ir.OpKind) bool {
	switch k {
	case ir.OpCNOT, ir.OpCZ, ir.OpSWAP, ir.OpISWAP, ir.OpCRX, ir.OpCRY, ir.OpCRZ, ir.OpCCX, ir.OpCSWAP:
		return true
	default:
		return false
	}
}
