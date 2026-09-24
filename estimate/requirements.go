// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"sort"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// ExecutionModel identifies the class of hardware/backend a workload needs.
// Quell source is gate-model circuit language, so DeriveRequirements always
// reports ExecGate today — QUBO problems (anneal.ParseQUBO) are a separate
// source format entirely and never flow through this function. The other
// values exist so the scheduler (qubitlabs-platform/internal/scheduler) has
// a shared vocabulary for backend capability, not because this package
// produces them yet.
type ExecutionModel string

const (
	ExecGate       ExecutionModel = "gate"
	ExecAnnealing  ExecutionModel = "annealing"
	ExecSimulation ExecutionModel = "simulation"
	// ExecAnalog / ExecHybrid are reserved for future target types
	// (neutral-atom analog, gate+anneal hybrid workflows) — not produced by
	// any current parser or adapter.
)

// WorkloadRequirements is what a compiled program actually needs from a
// backend, derived once from the IR rather than left for a caller to guess
// (e.g. via a manually supplied --min-qubits flag).
type WorkloadRequirements struct {
	ExecutionModel         ExecutionModel `json:"executionModel"`
	LogicalQubits          int            `json:"logicalQubits"`
	Depth                  int            `json:"depth"`
	GateCount              int            `json:"gateCount"`
	TwoQubitGates          int            `json:"twoQubitGates"`
	RequiredOps            []string       `json:"requiredOps"`
	MidCircuitMeasurement  bool           `json:"midCircuitMeasurement"`
	DynamicControl         bool           `json:"dynamicControl"`
	UsesParameterizedGates bool           `json:"usesParameterizedGates"`
	// RequiredConnectivity is the circuit's interaction graph: every
	// distinct pair of qubits (normalized low,high) touched together by a
	// multi-qubit op, deduplicated and sorted. A backend whose native
	// coupling map contains every one of these pairs can run the circuit
	// without SWAP-routing; missing a pair doesn't mean incompatible (SWAP
	// insertion — see quell/internal/optimizer/route.go — can always route
	// around a connected topology), it means "this will cost extra depth/
	// gates," which is why compatibility filtering treats topology as a
	// scoring/estimate signal rather than a hard reject — see
	// qubitlabs-platform's ScoreCandidateExplained.
	RequiredConnectivity [][2]int `json:"requiredConnectivity,omitempty"`
	Shots                int      `json:"shots,omitempty"`
}

// DeriveRequirements parses Quell source and reports what it needs from a
// backend. shots is caller-supplied (it's a job/run parameter, not
// something the circuit itself declares) and is passed through verbatim.
func DeriveRequirements(src string, shots int) (WorkloadRequirements, error) {
	circ, err := parser.Parse(src)
	if err != nil {
		return WorkloadRequirements{}, err
	}
	prog := ir.Lower(circ)
	stats := StatsFromProgram(prog)

	req := WorkloadRequirements{
		ExecutionModel:         ExecGate,
		LogicalQubits:          stats.NumQubits,
		Depth:                  stats.Depth,
		GateCount:              stats.GateCount,
		TwoQubitGates:          stats.TwoQubitGates,
		RequiredOps:            requiredOps(prog),
		MidCircuitMeasurement:  hasMidCircuitMeasurement(prog),
		DynamicControl:         hasDynamicControl(prog),
		UsesParameterizedGates: usesParameterizedGates(prog),
		RequiredConnectivity:   requiredConnectivity(prog),
		Shots:                  shots,
	}
	return req, nil
}

// usesParameterizedGates reports whether any op carries a symbolic
// (unbound) parameter. ir.Op.ArgNames is parallel to Args: ArgNames[i] is a
// non-empty name when Args[i] is a symbolic PARAM reference, and "" when
// it's a literal float — a fixed-angle gate still has a same-length
// ArgNames slice full of "" entries (see parser.go's argNames construction),
// so checking len(ArgNames) > 0 alone is wrong and would call every
// rotation gate "parameterized." Recurses into control-flow bodies like
// requiredOps does, for the same reason.
func usesParameterizedGates(p *ir.Program) bool {
	if p == nil {
		return false
	}
	var walk func(ops []ir.Op) bool
	walk = func(ops []ir.Op) bool {
		for _, op := range ops {
			for _, name := range op.ArgNames {
				if name != "" {
					return true
				}
			}
			if op.Body != nil && walk([]ir.Op{*op.Body}) {
				return true
			}
			if len(op.Then) > 0 && walk(op.Then) {
				return true
			}
			if len(op.Else) > 0 && walk(op.Else) {
				return true
			}
			for _, c := range op.Cases {
				if walk(c.Body) {
					return true
				}
			}
		}
		return false
	}
	return walk(p.Ops)
}

// requiredConnectivity collects every distinct pair of qubits touched
// together by the same op — the circuit's interaction graph, normalized
// (low index first) and deduplicated. A 3+-qubit op (CCX, CSWAP) contributes
// every pairwise combination among its qubits, since all of them need to
// interact for the gate to execute, not just adjacent ones in Qubits' order.
func requiredConnectivity(p *ir.Program) [][2]int {
	if p == nil {
		return nil
	}
	seen := map[[2]int]bool{}
	var walk func(ops []ir.Op)
	walk = func(ops []ir.Op) {
		for _, op := range ops {
			for i := 0; i < len(op.Qubits); i++ {
				for j := i + 1; j < len(op.Qubits); j++ {
					a, b := op.Qubits[i], op.Qubits[j]
					if a > b {
						a, b = b, a
					}
					seen[[2]int{a, b}] = true
				}
			}
			if op.Body != nil {
				walk([]ir.Op{*op.Body})
			}
			if len(op.Then) > 0 {
				walk(op.Then)
			}
			if len(op.Else) > 0 {
				walk(op.Else)
			}
			for _, c := range op.Cases {
				walk(c.Body)
			}
		}
	}
	walk(p.Ops)

	if len(seen) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(seen))
	for pair := range seen {
		out = append(out, pair)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// requiredOps returns the sorted, deduplicated set of op kinds the program
// actually uses — including those nested inside IF/WHILE/SWITCH bodies, so
// a backend's native-gate check sees everything that would need to run.
func requiredOps(p *ir.Program) []string {
	if p == nil {
		return nil
	}
	seen := map[string]bool{}
	var walk func(ops []ir.Op)
	walk = func(ops []ir.Op) {
		for _, op := range ops {
			seen[string(op.Kind)] = true
			if op.Body != nil {
				walk([]ir.Op{*op.Body})
			}
			if len(op.Then) > 0 {
				walk(op.Then)
			}
			if len(op.Else) > 0 {
				walk(op.Else)
			}
			for _, c := range op.Cases {
				walk(c.Body)
			}
		}
	}
	walk(p.Ops)

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// hasMidCircuitMeasurement reports whether any qubit is measured and then
// touched again afterward — a plain "MEASURE everything at the end" circuit
// (the overwhelmingly common case) does not count, since every gate-model
// backend supports a single terminal measurement.
func hasMidCircuitMeasurement(p *ir.Program) bool {
	if p == nil || p.NumQubits == 0 {
		return false
	}
	measuredAt := make([]int, p.NumQubits) // last index a MEASURE touched this qubit, -1 if never
	for i := range measuredAt {
		measuredAt[i] = -1
	}
	for i, op := range p.Ops {
		if op.Kind == ir.OpMEASURE {
			for _, q := range measureTargets(op, p.NumQubits) {
				if q >= 0 && q < p.NumQubits {
					measuredAt[q] = i
				}
			}
			continue
		}
		for _, q := range op.Qubits {
			if q >= 0 && q < p.NumQubits && measuredAt[q] >= 0 {
				return true
			}
		}
	}
	return false
}

// measureTargets mirrors the "empty Qubits means measure everything" rule
// used elsewhere (see estimate/stats.go's touched()).
func measureTargets(op ir.Op, numQubits int) []int {
	if len(op.Qubits) > 0 {
		return op.Qubits
	}
	out := make([]int, numQubits)
	for i := range out {
		out[i] = i
	}
	return out
}

// hasDynamicControl reports whether the program contains any classically
// conditioned control flow (IF/WHILE/SWITCH) — the hallmark of a "dynamic
// circuit" in the OpenQASM 3 / hardware-vendor sense, as opposed to a
// straight-line list of gates.
func hasDynamicControl(p *ir.Program) bool {
	if p == nil {
		return false
	}
	for _, op := range p.Ops {
		switch op.Kind {
		case ir.OpIF, ir.OpWHILE, ir.OpSWITCH:
			return true
		}
	}
	return false
}
