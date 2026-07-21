// Copyright 2026 Magnobit, Inc. All rights reserved.

package optimizer

import (
	"fmt"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/topology"
)

// Options configures optional Phase-3 passes.
type Options struct {
	// Coupling, when set, inserts SWAPs so two-qubit gates land on edges.
	Coupling *topology.CouplingMap
	// NoiseAware appends a rough fidelity-cost score note after routing.
	NoiseAware bool
}

// OptimizeWithOptions runs conservative passes, then optional routing / noise scoring.
func OptimizeWithOptions(p *ir.Program, opts Options) (*ir.Program, []string) {
	cur, notes := Optimize(p)
	if opts.Coupling != nil {
		var n []string
		cur, n = routeToCoupling(cur, opts.Coupling)
		notes = append(notes, n...)
	}
	if opts.NoiseAware {
		notes = append(notes, noiseScoreNote(cur)...)
	}
	return cur, notes
}

// routeToCoupling inserts SWAP chains so CNOT/CZ/SWAP targets are adjacent.
func routeToCoupling(p *ir.Program, m *topology.CouplingMap) (*ir.Program, []string) {
	if m == nil || p == nil {
		return p, nil
	}
	var notes []string
	var out []ir.Op
	swaps := 0

	// Logical → physical mapping (identity start).
	nq := p.NumQubits
	phys := make([]int, nq) // phys[logical] = physical
	for i := 0; i < nq; i++ {
		phys[i] = i
	}
	inv := make([]int, nq) // inv[physical] = logical
	copy(inv, phys)

	remap := func(qs []int) []int {
		r := make([]int, len(qs))
		for i, q := range qs {
			if q >= 0 && q < len(phys) {
				r[i] = phys[q]
			} else {
				r[i] = q
			}
		}
		return r
	}

	swapPhys := func(a, b int) {
		la, lb := inv[a], inv[b]
		phys[la], phys[lb] = b, a
		inv[a], inv[b] = lb, la
	}

	for _, op := range p.Ops {
		kind := op.Kind
		needsRoute := kind == ir.OpCNOT || kind == ir.OpCZ || kind == ir.OpSWAP ||
			kind == ir.OpCRX || kind == ir.OpCRY || kind == ir.OpCRZ
		if !needsRoute || len(op.Qubits) < 2 {
			cp := op
			cp.Qubits = remap(op.Qubits)
			out = append(out, cp)
			continue
		}
		c, t := op.Qubits[0], op.Qubits[1]
		pc, pt := phys[c], phys[t]
		if m.Connected(pc, pt) {
			cp := op
			cp.Qubits = []int{pc, pt}
			if len(op.Qubits) > 2 {
				cp.Qubits = append(cp.Qubits, remap(op.Qubits[2:])...)
			}
			out = append(out, cp)
			continue
		}
		path := m.ShortestPath(pc, pt)
		if path == nil || len(path) < 2 {
			notes = append(notes, fmt.Sprintf(
				"routing: no path on %s between physical %d and %d — left CNOT unmapped", m.Name, pc, pt))
			cp := op
			cp.Qubits = remap(op.Qubits)
			out = append(out, cp)
			continue
		}
		// Bubble target toward control along path with SWAPs.
		for i := len(path) - 1; i > 1; i-- {
			a, b := path[i], path[i-1]
			out = append(out, ir.Op{Kind: ir.OpSWAP, Qubits: []int{a, b}})
			swapPhys(a, b)
			swaps++
		}
		pc, pt = phys[c], phys[t]
		cp := op
		cp.Qubits = []int{pc, pt}
		out = append(out, cp)
	}

	if swaps > 0 {
		notes = append(notes, fmt.Sprintf(
			"routing (%s): inserted %d SWAP(s) for coupling constraints", m.Name, swaps))
	} else {
		notes = append(notes, fmt.Sprintf("routing (%s): circuit already coupling-compliant", m.Name))
	}
	return &ir.Program{
		NumQubits:             p.NumQubits,
		Ops:                   out,
		Params:                append([]string(nil), p.Params...),
		NoiseDepolarizing:     p.NoiseDepolarizing,
		NoiseAmplitudeDamping: p.NoiseAmplitudeDamping,
		NoisePhaseDamping:     p.NoisePhaseDamping,
		NoiseReadout:          p.NoiseReadout,
	}, notes
}

func noiseScoreNote(p *ir.Program) []string {
	if p == nil {
		return nil
	}
	depth := 0
	twoQ := 0
	swaps := 0
	// Approximate depth = gate count layers (quick proxy).
	qubitLayer := make([]int, p.NumQubits)
	for _, op := range p.Ops {
		qs := touchedQubits(op, p.NumQubits)
		maxD := 0
		for _, q := range qs {
			if q >= 0 && q < len(qubitLayer) && qubitLayer[q] > maxD {
				maxD = qubitLayer[q]
			}
		}
		for _, q := range qs {
			if q >= 0 && q < len(qubitLayer) {
				qubitLayer[q] = maxD + 1
			}
		}
		if op.Kind == ir.OpCNOT || op.Kind == ir.OpCZ || op.Kind == ir.OpSWAP ||
			op.Kind == ir.OpCRX || op.Kind == ir.OpCRY || op.Kind == ir.OpCRZ ||
			op.Kind == ir.OpCCX || op.Kind == ir.OpCSWAP {
			twoQ++
		}
		if op.Kind == ir.OpSWAP {
			swaps++
		}
	}
	for _, d := range qubitLayer {
		if d > depth {
			depth = d
		}
	}
	// Educational score: lower is better. Weight SWAPs heavily (noise).
	score := float64(depth) + 2.5*float64(twoQ) + 4.0*float64(swaps)
	return []string{fmt.Sprintf(
		"noise-aware score ≈ %.1f (depth=%d twoQ=%d swaps=%d; lower is better)",
		score, depth, twoQ, swaps)}
}
