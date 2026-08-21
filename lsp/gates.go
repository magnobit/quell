// Copyright 2026 Magnobit, Inc. All rights reserved.

package lsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/magnobit/quell/internal/parser"
)

// gateDoc is human-readable hover/completion text for a built-in gate or
// control-flow keyword — parser.GateArity gives the canonical name/arity
// list this keys off of, but arity alone isn't useful in an editor tooltip.
var gateDoc = map[string]string{
	"H":       "Hadamard — puts the qubit into an equal superposition of |0⟩ and |1⟩.",
	"X":       "Pauli-X — bit flip (quantum NOT).",
	"Y":       "Pauli-Y — combined bit and phase flip.",
	"Z":       "Pauli-Z — phase flip.",
	"S":       "S gate — quarter-turn phase gate (√Z).",
	"T":       "T gate — eighth-turn phase gate (√S).",
	"SDG":     "S† — inverse of S.",
	"TDG":     "T† — inverse of T.",
	"SX":      "√X — square root of the X gate.",
	"RX":      "Rotation around the X-axis by an angle in radians.",
	"RY":      "Rotation around the Y-axis by an angle in radians.",
	"RZ":      "Rotation around the Z-axis by an angle in radians.",
	"P":       "Phase gate — shifts the phase of |1⟩ by the given angle.",
	"U":       "Universal single-qubit gate — three Euler angles (θ, φ, λ).",
	"CNOT":    "Controlled-NOT — flips the target qubit when the control is |1⟩.",
	"CZ":      "Controlled-Z — applies Z to the target when the control is |1⟩.",
	"SWAP":    "Swaps the state of two qubits.",
	"ISWAP":   "Swap with an added phase — the native two-qubit gate on some hardware.",
	"CRX":     "Controlled rotation around the X-axis.",
	"CRY":     "Controlled rotation around the Y-axis.",
	"CRZ":     "Controlled rotation around the Z-axis.",
	"CCX":     "Toffoli — flips the target when both controls are |1⟩.",
	"CSWAP":   "Fredkin — swaps two qubits when the control is |1⟩.",
	"MEASURE": "Measures one or more qubits into classical bits.",
	"BARRIER": "Optimization barrier — blocks gate reordering/merging across this point.",
	"RESET":   "Resets a qubit to |0⟩.",
	"IF":      "Classical control flow — runs the body only if a classical-bit condition holds.",
	"WHILE":   "Classical control flow — repeats the body while a condition holds, up to MAX iterations.",
	"FOR":     "Unrolls the body once per value in a range at parse time.",
	"SWITCH":  "Classical control flow — branches on a classical bit or register value.",
	"PAR":     "Marks a block whose gates may be scheduled in parallel.",
	"ASSERT":  "Fails compilation if a classical condition doesn't hold — a compile-time sanity check.",
	"NOISE":   "Declares a local-simulator noise model (depolarizing, amplitude_damping, phase_damping, readout). Ignored by hardware compile targets.",
	"NOT":     "Alias — see IF/WHILE for classical control flow.",
}

// gateHover renders the hover markdown for a built-in gate/keyword name
// (already uppercased), or "" if name isn't one.
func gateHover(name string) string {
	doc, ok := gateDoc[name]
	if !ok {
		return ""
	}
	if spec, isGate := parser.GateArity[name]; isGate {
		arity := arityText(spec)
		return fmt.Sprintf("**%s** — %s\n\n%s", name, arity, doc)
	}
	return fmt.Sprintf("**%s**\n\n%s", name, doc)
}

func arityText(spec parser.GateSpec) string {
	var parts []string
	switch {
	case spec.Qubits < 0:
		parts = append(parts, "variadic qubits")
	case spec.Qubits == 1:
		parts = append(parts, "1 qubit")
	default:
		parts = append(parts, fmt.Sprintf("%d qubits", spec.Qubits))
	}
	if spec.Args == 1 {
		parts = append(parts, "1 angle argument")
	} else if spec.Args > 1 {
		parts = append(parts, fmt.Sprintf("%d angle arguments", spec.Args))
	}
	return strings.Join(parts, ", ")
}

// sortedGateNames returns every built-in gate name plus control-flow
// keywords that have documentation, for completion — sorted so results are
// stable across requests instead of following Go's randomized map order.
func sortedGateNames() []string {
	seen := map[string]bool{}
	var names []string
	for name := range parser.GateArity {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, kw := range []string{"IF", "WHILE", "FOR", "SWITCH", "PAR", "ASSERT", "NOISE"} {
		if !seen[kw] {
			seen[kw] = true
			names = append(names, kw)
		}
	}
	sort.Strings(names)
	return names
}
