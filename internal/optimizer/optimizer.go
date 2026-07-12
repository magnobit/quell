// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package optimizer implements conservative, correctness-preserving passes
// over the Quell IR (internal/ir). Every pass refuses to reorder or cancel
// operations across a MEASURE, BARRIER, or RESET that touches any of the
// qubits involved — those are treated as hard synchronisation points that
// nothing may cross.
package optimizer

import (
	"fmt"
	"math"
	"strings"

	"github.com/magnobit/quell/internal/ir"
)

// angleEpsilon is the tolerance used when deciding whether a rotation angle
// is a no-op after reducing modulo 2π. Angles are floating point (and PI
// arithmetic in the parser, e.g. "2*PI", does not always land on exactly
// 0), so we treat anything within this tolerance of a multiple of 2π as
// zero.
const angleEpsilon = 1e-9

// Optimize runs all conservative optimization passes over p, in order, and
// returns the optimized program along with human-readable notes describing
// what changed. p itself is never mutated; Optimize always returns a new
// *ir.Program built from a copy of p.Ops.
//
// Passes, in order:
//  1. Zero-angle rotation elimination
//  2. Adjacent self-inverse cancellation
//  3. Rotation fusion (with a follow-up zero-angle sweep on the result)
func Optimize(p *ir.Program) (*ir.Program, []string) {
	var notes []string

	cur := &ir.Program{NumQubits: p.NumQubits, Ops: append([]ir.Op(nil), p.Ops...)}

	cur, n := dropZeroAngleRotations(cur)
	notes = append(notes, n...)

	cur, n = cancelSelfInverse(cur)
	notes = append(notes, n...)

	cur, n = fuseRotations(cur)
	notes = append(notes, n...)

	// Fusing rotations can produce a fresh zero-angle op (e.g. RZ(1.5)
	// followed by RZ(-1.5) fuses to RZ(0)) — sweep once more to drop those.
	cur, n = dropZeroAngleRotations(cur)
	notes = append(notes, n...)

	return cur, notes
}

// touchedQubits returns the qubits an op reads or writes. MEASURE and
// BARRIER with no explicit qubit list act on every qubit in the circuit.
func touchedQubits(op ir.Op, numQubits int) []int {
	if len(op.Qubits) == 0 && (op.Kind == ir.OpMEASURE || op.Kind == ir.OpBARRIER) {
		all := make([]int, numQubits)
		for i := range all {
			all[i] = i
		}
		return all
	}
	return op.Qubits
}

func qubitList(qs []int) string {
	if len(qs) == 0 {
		return "(all)"
	}
	parts := make([]string, len(qs))
	for i, q := range qs {
		parts[i] = fmt.Sprintf("%d", q)
	}
	return strings.Join(parts, ", ")
}

// qubitWord renders "qubit N" or "qubits N, M, ..." depending on count.
func qubitWord(qs []int) string {
	if len(qs) == 1 {
		return fmt.Sprintf("qubit %d", qs[0])
	}
	return fmt.Sprintf("qubits %s", qubitList(qs))
}

// ── Pass 1: zero-angle rotation elimination ────────────────────────────────

var rotationKinds = map[ir.OpKind]bool{
	ir.OpRX:  true,
	ir.OpRY:  true,
	ir.OpRZ:  true,
	ir.OpP:   true,
	ir.OpCRX: true,
	ir.OpCRY: true,
	ir.OpCRZ: true,
}

// isZeroAngle reports whether angle is a no-op rotation, i.e. a multiple of
// 2π within angleEpsilon.
func isZeroAngle(angle float64) bool {
	const twoPi = 2 * math.Pi
	m := math.Mod(angle, twoPi)
	if m < 0 {
		m += twoPi
	}
	return m < angleEpsilon || twoPi-m < angleEpsilon
}

func dropZeroAngleRotations(p *ir.Program) (*ir.Program, []string) {
	var notes []string
	kept := make([]ir.Op, 0, len(p.Ops))
	for _, op := range p.Ops {
		if rotationKinds[op.Kind] && len(op.Args) > 0 && isZeroAngle(op.Args[0]) {
			notes = append(notes, fmt.Sprintf("removed zero-angle %s on %s", op.Kind, qubitWord(op.Qubits)))
			continue
		}
		kept = append(kept, op)
	}
	return &ir.Program{NumQubits: p.NumQubits, Ops: kept}, notes
}

// ── Pass 2: adjacent self-inverse cancellation ─────────────────────────────

// selfInverseSameKind lists gates that cancel against another instance of
// themselves when applied to the exact same qubit list with nothing in
// between touching those qubits. ISWAP is intentionally excluded: iSWAP
// applied twice is not the identity, so it must never be cancelled.
var selfInverseSameKind = map[ir.OpKind]bool{
	ir.OpX:     true,
	ir.OpY:     true,
	ir.OpZ:     true,
	ir.OpH:     true,
	ir.OpCNOT:  true,
	ir.OpCZ:    true,
	ir.OpSWAP:  true,
	ir.OpCCX:   true,
	ir.OpCSWAP: true,
}

// pairInverse maps a gate Kind to the Kind that cancels it (S/SDG and
// T/TDG are inverses of each other, not of themselves).
var pairInverse = map[ir.OpKind]ir.OpKind{
	ir.OpS:   ir.OpSDG,
	ir.OpSDG: ir.OpS,
	ir.OpT:   ir.OpTDG,
	ir.OpTDG: ir.OpT,
}

func isCancelCandidate(k ir.OpKind) bool {
	return selfInverseSameKind[k] || pairInverse[k] != ""
}

// cancelsWith reports whether an op of kind b immediately following an op
// of kind a (on the same qubits) cancels both out.
func cancelsWith(a, b ir.OpKind) bool {
	if selfInverseSameKind[a] && a == b {
		return true
	}
	if inv, ok := pairInverse[a]; ok && inv == b {
		return true
	}
	return false
}

func qubitsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cancelSelfInverse drops adjacent pairs of self-inverse gates (X X, H H,
// S SDG, CNOT a b + CNOT a b, ...) that act on identical qubit lists with no
// other operation touching any of those qubits in between. "In between" is
// tracked per-qubit via a stack of the most recent op index seen for that
// qubit, so cancellation correctly cascades (X X X X fully cancels) while
// never crossing an unrelated op — including MEASURE/BARRIER/RESET, which
// are pushed onto every qubit stack they touch just like any other op and
// so always block cancellation across them.
func cancelSelfInverse(p *ir.Program) (*ir.Program, []string) {
	type node struct {
		op      ir.Op
		removed bool
	}
	nodes := make([]node, len(p.Ops))
	for i, op := range p.Ops {
		nodes[i] = node{op: op}
	}

	stacks := make(map[int][]int) // qubit → stack of node indices, most-recent last
	var notes []string

	for i := range nodes {
		op := nodes[i].op

		if isCancelCandidate(op.Kind) && len(op.Qubits) > 0 {
			cand := -1
			if s := stacks[op.Qubits[0]]; len(s) > 0 {
				cand = s[len(s)-1]
			}
			matched := cand != -1
			if matched {
				for _, q := range op.Qubits {
					s := stacks[q]
					if len(s) == 0 || s[len(s)-1] != cand {
						matched = false
						break
					}
				}
			}
			if matched {
				candOp := nodes[cand].op
				if qubitsEqual(candOp.Qubits, op.Qubits) && cancelsWith(candOp.Kind, op.Kind) {
					nodes[cand].removed = true
					nodes[i].removed = true
					for _, q := range op.Qubits {
						s := stacks[q]
						stacks[q] = s[:len(s)-1]
					}
					notes = append(notes, fmt.Sprintf("removed 2 redundant gate(s) on %s", qubitWord(op.Qubits)))
					continue
				}
			}
		}

		for _, q := range touchedQubits(op, p.NumQubits) {
			stacks[q] = append(stacks[q], i)
		}
	}

	kept := make([]ir.Op, 0, len(nodes))
	for _, n := range nodes {
		if !n.removed {
			kept = append(kept, n.op)
		}
	}
	return &ir.Program{NumQubits: p.NumQubits, Ops: kept}, notes
}

// ── Pass 3: rotation fusion ─────────────────────────────────────────────────

var fusableRotation = map[ir.OpKind]bool{
	ir.OpRX: true,
	ir.OpRY: true,
	ir.OpRZ: true,
	ir.OpP:  true,
}

// fuseRotations merges consecutive same-kind single-qubit rotations
// (RX+RX, RY+RY, RZ+RZ, P+P) on the exact same qubit, with no intervening
// op on that qubit, into one op whose angle is the sum. Fusion cascades:
// three consecutive RZ ops on the same qubit fuse into a single RZ.
func fuseRotations(p *ir.Program) (*ir.Program, []string) {
	type node struct {
		op      ir.Op
		removed bool
	}
	nodes := make([]node, len(p.Ops))
	for i, op := range p.Ops {
		nodes[i] = node{op: op}
	}

	lastTouch := make(map[int]int) // qubit → index of the node most recently touching it
	fusedInto := make(map[int]int) // surviving node index → count of ops merged into it

	for i := range nodes {
		op := nodes[i].op

		if fusableRotation[op.Kind] && len(op.Qubits) == 1 {
			q := op.Qubits[0]
			if last, ok := lastTouch[q]; ok {
				lastOp := nodes[last].op
				if !nodes[last].removed && lastOp.Kind == op.Kind && len(lastOp.Qubits) == 1 && lastOp.Qubits[0] == q {
					nodes[last].op.Args = []float64{lastOp.Args[0] + op.Args[0]}
					nodes[i].removed = true
					fusedInto[last]++
					continue
				}
			}
		}

		for _, q := range touchedQubits(op, p.NumQubits) {
			lastTouch[q] = i
		}
	}

	var notes []string
	kept := make([]ir.Op, 0, len(nodes))
	for i, n := range nodes {
		if n.removed {
			continue
		}
		kept = append(kept, n.op)
		if c := fusedInto[i]; c > 0 {
			notes = append(notes, fmt.Sprintf(
				"fused %d consecutive %s op(s) into one on %s",
				c+1, n.op.Kind, qubitWord(n.op.Qubits)))
		}
	}
	return &ir.Program{NumQubits: p.NumQubits, Ops: kept}, notes
}
