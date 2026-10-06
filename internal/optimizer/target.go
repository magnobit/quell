// Copyright 2026 Magnobit, Inc. All rights reserved.

package optimizer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/topology"
)

const targetPassVersion = "1"

// TargetProfile is the hardware picture a pass is allowed to see.
// Error metrics are optional. A missing metric stays absent.
type TargetProfile struct {
	Native      []string
	Coupling    *topology.CouplingMap
	MaxQubits   int
	Dynamic     bool
	Calibrated  string
	ErrorMetric map[string]float64
}

// PassRecord is one executed pass. Latency is process time, not queue time.
type PassRecord struct {
	Name     string
	Version  string
	In       int
	Out      int
	Latency  time.Duration
	Snapshot string
}

// OptimizeForTarget runs basis lowering, commutation, two-qubit cancellation,
// swap cancellation, depth scheduling, and coupling routing when a map is set.
// Dynamic ops stay barriers. The snapshot hash covers the profile, not the circuit.
func OptimizeForTarget(p *ir.Program, tp TargetProfile) (*ir.Program, []PassRecord) {
	if p == nil {
		return &ir.Program{}, nil
	}
	snap := SnapshotHash(tp)
	cur := withOps(p, append([]ir.Op(nil), p.Ops...))
	var recs []PassRecord
	run := func(name string, fn func(*ir.Program) *ir.Program) {
		in := len(cur.Ops)
		start := time.Now()
		cur = fn(cur)
		recs = append(recs, PassRecord{
			Name: name, Version: targetPassVersion, In: in, Out: len(cur.Ops),
			Latency: time.Since(start), Snapshot: snap,
		})
	}
	native := nativeSet(tp.Native)
	run("basis_decomposition", func(p *ir.Program) *ir.Program { return basisDecompose(p, native) })
	run("native_gate_lowering", func(p *ir.Program) *ir.Program { return basisDecompose(p, native) })
	run("commutation_simplification", commuteZControl)
	run("two_qubit_reduction", cancelTwoQubit)
	run("swap_minimization", cancelAdjacentSwap)
	run("depth_reduction", scheduleDepth)
	if tp.Coupling != nil {
		run("routing", func(p *ir.Program) *ir.Program {
			out, _ := routeToCoupling(p, tp.Coupling)
			return out
		})
	}
	return cur, recs
}

// SnapshotHash is stable for the same profile fields.
func SnapshotHash(tp TargetProfile) string {
	native := append([]string(nil), tp.Native...)
	sort.Strings(native)
	var edges []string
	if tp.Coupling != nil {
		edges = append(edges, tp.Coupling.Name)
	}
	payload := struct {
		Native     []string           `json:"native"`
		Edges      []string           `json:"edges"`
		MaxQubits  int                `json:"maxQubits"`
		Dynamic    bool               `json:"dynamic"`
		Calibrated string             `json:"calibrated"`
		Errors     map[string]float64 `json:"errors,omitempty"`
	}{native, edges, tp.MaxQubits, tp.Dynamic, tp.Calibrated, tp.ErrorMetric}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func nativeSet(names []string) map[ir.OpKind]bool {
	if len(names) == 0 {
		return nil
	}
	out := map[ir.OpKind]bool{}
	for _, n := range names {
		out[ir.OpKind(n)] = true
	}
	return out
}

func basisDecompose(p *ir.Program, native map[ir.OpKind]bool) *ir.Program {
	if len(native) == 0 {
		return p
	}
	var out []ir.Op
	for _, op := range p.Ops {
		if native[op.Kind] || barrierKind(op.Kind) {
			out = append(out, op)
			continue
		}
		switch op.Kind {
		case ir.OpSDG:
			if native[ir.OpS] {
				for i := 0; i < 3; i++ {
					out = append(out, ir.Op{Kind: ir.OpS, Qubits: append([]int(nil), op.Qubits...)})
				}
				continue
			}
		case ir.OpTDG:
			if native[ir.OpT] {
				for i := 0; i < 7; i++ {
					out = append(out, ir.Op{Kind: ir.OpT, Qubits: append([]int(nil), op.Qubits...)})
				}
				continue
			}
		}
		out = append(out, op)
	}
	return withOps(p, out)
}

func barrierKind(k ir.OpKind) bool {
	switch k {
	case ir.OpMEASURE, ir.OpRESET, ir.OpBARRIER, ir.OpIF, ir.OpWHILE, ir.OpSWITCH, ir.OpASSERT, ir.OpPAR:
		return true
	default:
		return false
	}
}

// commuteZControl slides a Z on the CNOT control through the CNOT.
// Z on the control commutes with CNOT. Measurement stays a barrier.
func commuteZControl(p *ir.Program) *ir.Program {
	ops := append([]ir.Op(nil), p.Ops...)
	for i := 0; i+1 < len(ops); i++ {
		if barrierKind(ops[i].Kind) || barrierKind(ops[i+1].Kind) {
			continue
		}
		if ops[i].Kind == ir.OpZ && ops[i+1].Kind == ir.OpCNOT && len(ops[i].Qubits) == 1 && len(ops[i+1].Qubits) >= 2 && ops[i].Qubits[0] == ops[i+1].Qubits[0] {
			ops[i], ops[i+1] = ops[i+1], ops[i]
		}
	}
	out, _ := cancelSelfInverse(withOps(p, ops))
	return out
}

func cancelTwoQubit(p *ir.Program) *ir.Program {
	var out []ir.Op
	for _, op := range p.Ops {
		if len(out) > 0 && twoQCancels(out[len(out)-1], op) {
			out = out[:len(out)-1]
			continue
		}
		out = append(out, op)
	}
	return withOps(p, out)
}

func twoQCancels(a, b ir.Op) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case ir.OpCNOT:
		return qubitsEqual(a.Qubits, b.Qubits)
	case ir.OpCZ:
		return qubitsEqual(a.Qubits, b.Qubits) || (len(a.Qubits) == 2 && len(b.Qubits) == 2 && a.Qubits[0] == b.Qubits[1] && a.Qubits[1] == b.Qubits[0])
	default:
		return false
	}
}

func cancelAdjacentSwap(p *ir.Program) *ir.Program {
	var out []ir.Op
	for _, op := range p.Ops {
		if op.Kind == ir.OpSWAP && len(out) > 0 && out[len(out)-1].Kind == ir.OpSWAP && swapSame(out[len(out)-1].Qubits, op.Qubits) {
			out = out[:len(out)-1]
			continue
		}
		out = append(out, op)
	}
	return withOps(p, out)
}

func swapSame(a, b []int) bool {
	if qubitsEqual(a, b) {
		return true
	}
	return len(a) == 2 && len(b) == 2 && a[0] == b[1] && a[1] == b[0]
}

// scheduleDepth pulls an operation earlier when every op it passes acts on
// a disjoint qubit set. Barriers are not crossed.
func scheduleDepth(p *ir.Program) *ir.Program {
	ops := append([]ir.Op(nil), p.Ops...)
	for i := 1; i < len(ops); i++ {
		j := i
		for j > 0 && !barrierKind(ops[j].Kind) && !barrierKind(ops[j-1].Kind) && disjoint(ops[j], ops[j-1]) {
			ops[j], ops[j-1] = ops[j-1], ops[j]
			j--
		}
	}
	return withOps(p, ops)
}

func disjoint(a, b ir.Op) bool {
	if len(a.Qubits) == 0 || len(b.Qubits) == 0 {
		return false
	}
	set := map[int]bool{}
	for _, q := range a.Qubits {
		set[q] = true
	}
	for _, q := range b.Qubits {
		if set[q] {
			return false
		}
	}
	return true
}

// DepthLayers counts sequential layers. Overlapping qubits start a new layer.
func DepthLayers(p *ir.Program) int {
	if p == nil || len(p.Ops) == 0 {
		return 0
	}
	layers := 0
	used := map[int]bool{}
	open := false
	flush := func() {
		if open {
			layers++
			used = map[int]bool{}
			open = false
		}
	}
	for _, op := range p.Ops {
		if barrierKind(op.Kind) || len(op.Qubits) == 0 {
			flush()
			layers++
			continue
		}
		conflict := false
		for _, q := range op.Qubits {
			if used[q] {
				conflict = true
			}
		}
		if conflict {
			flush()
		}
		for _, q := range op.Qubits {
			used[q] = true
		}
		open = true
	}
	flush()
	return layers
}

// FormatPass is a one-line record for tests and provenance notes.
func FormatPass(r PassRecord) string {
	return fmt.Sprintf("%s@%s in=%d out=%d latency=%s snapshot=%s", r.Name, r.Version, r.In, r.Out, r.Latency, r.Snapshot)
}
