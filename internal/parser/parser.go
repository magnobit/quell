// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"bufio"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Instruction represents a single parsed gate operation.
type Instruction struct {
	Gate       string
	Qubits     []int
	Args       []float64
	QubitNames []string
	Line       int // source line number (1-indexed) for diagnostics
}

// Circuit is the parsed representation of a .quell source file.
type Circuit struct {
	Instructions []Instruction
	NumQubits    int
	QubitNames   map[string]int
	Warnings     []string // non-fatal issues that compile fine but likely indicate mistakes
}

// gateSpec encodes the expected qubit and float-argument count per gate.
// qubits == -1 means variadic (MEASURE and BARRIER accept 0 or more qubits).
type gateSpec struct{ qubits, args int }

var gateArity = map[string]gateSpec{
	"H":       {1, 0},
	"X":       {1, 0},
	"Y":       {1, 0},
	"Z":       {1, 0},
	"S":       {1, 0},
	"T":       {1, 0},
	"SDG":     {1, 0},
	"TDG":     {1, 0},
	"SX":      {1, 0},
	"RX":      {1, 1},
	"RY":      {1, 1},
	"RZ":      {1, 1},
	"P":       {1, 1},
	"U":       {1, 3},
	"CNOT":    {2, 0},
	"CZ":      {2, 0},
	"SWAP":    {2, 0},
	"ISWAP":   {2, 0},
	"CRX":     {2, 1},
	"CRY":     {2, 1},
	"CRZ":     {2, 1},
	"CCX":     {3, 0},
	"CSWAP":   {3, 0},
	"MEASURE": {-1, 0},
	"BARRIER": {-1, 0},
	"RESET":   {1, 0},
}

const validGateNames = "H X Y Z S T SDG TDG SX RX RY RZ P U CNOT CZ SWAP ISWAP CRX CRY CRZ CCX CSWAP MEASURE BARRIER RESET"

// simulatorMaxQubits is the maximum supported by the QubitLabs browser simulator.
const simulatorMaxQubits = 12

// depthWarnThreshold is the circuit depth at which we warn about hardware decoherence.
const depthWarnThreshold = 100

// Parse parses Quell source code and returns a Circuit.
//
// Named qubit declarations:
//
//	qubit alice          → alice=0
//	qubit bob, charlie   → bob=1, charlie=2
//
// Rotation-gate angles may use PI notation: PI/2, 2*PI, or plain floats.
// Angle arguments come before qubit indices (e.g. RX PI/2 0).
//
// Non-fatal semantic issues are returned in Circuit.Warnings.
func Parse(src string) (*Circuit, error) {
	var instructions []Instruction
	maxQubit := -1
	namedQubits := map[string]int{}
	nextNamedIdx := 0

	scanner := bufio.NewScanner(strings.NewReader(src))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		tokens := strings.Fields(line)
		keyword := strings.ToUpper(tokens[0])

		// Named qubit declaration
		if keyword == "QUBIT" {
			for _, tok := range tokens[1:] {
				name := strings.Trim(tok, ", \t")
				if name == "" {
					continue
				}
				if _, exists := namedQubits[name]; !exists {
					namedQubits[name] = nextNamedIdx
					if nextNamedIdx > maxQubit {
						maxQubit = nextNamedIdx
					}
					nextNamedIdx++
				}
			}
			continue
		}

		if keyword == "IMPORT" {
			return nil, fmt.Errorf("line %d: import requires a file on disk to resolve paths against — use parser.ParseFile (or the CLI, which always does) instead of Parse on a bare source string", lineNum)
		}

		gate := keyword
		spec, known := gateArity[gate]
		if !known {
			return nil, fmt.Errorf("line %d: unknown gate %q (valid gates: %s)", lineNum, gate, validGateNames)
		}

		var qubits []int
		var qubitNames []string
		var args []float64

		for _, tok := range tokens[1:] {
			tok = strings.TrimRight(tok, ",")
			if tok == "" {
				continue
			}

			// Named qubit reference — always a qubit regardless of position
			if idx, ok := namedQubits[tok]; ok {
				if idx > maxQubit {
					maxQubit = idx
				}
				qubits = append(qubits, idx)
				qubitNames = append(qubitNames, tok)
				continue
			}

			// If gate expects more angle args, consume as angle.
			// Handles both "RX 1.5708 0" and "RX 1 0" (integer promoted to float).
			if spec.args > 0 && len(args) < spec.args {
				if f, ok := evalAngle(tok); ok {
					args = append(args, f)
					continue
				}
			}

			// Integer qubit index
			if n, err := strconv.Atoi(tok); err == nil {
				if n < 0 {
					return nil, fmt.Errorf("line %d: qubit index %d is negative", lineNum, n)
				}
				if n > maxQubit {
					maxQubit = n
				}
				qubits = append(qubits, n)
				qubitNames = append(qubitNames, tok)
				continue
			}

			// Float angle in non-standard position
			if f, ok := evalAngle(tok); ok {
				args = append(args, f)
				continue
			}

			return nil, fmt.Errorf("line %d: unexpected token %q", lineNum, tok)
		}

		// Validate qubit count
		if spec.qubits >= 0 && len(qubits) != spec.qubits {
			return nil, fmt.Errorf("line %d: %s requires %d qubit(s), got %d", lineNum, gate, spec.qubits, len(qubits))
		}
		// Validate angle arg count
		if len(args) != spec.args {
			return nil, fmt.Errorf("line %d: %s requires %d angle argument(s), got %d — use float or PI notation (e.g. PI/2)", lineNum, gate, spec.args, len(args))
		}
		// Validate no duplicate qubits on multi-qubit gates (CNOT 0 0 is invalid)
		if spec.qubits >= 2 {
			seen := map[int]bool{}
			for _, q := range qubits {
				if seen[q] {
					return nil, fmt.Errorf("line %d: %s has duplicate qubit %d — control and target must differ", lineNum, gate, q)
				}
				seen[q] = true
			}
		}

		instructions = append(instructions, Instruction{
			Gate:       gate,
			Qubits:     qubits,
			Args:       args,
			QubitNames: qubitNames,
			Line:       lineNum,
		})
	}

	if len(instructions) == 0 {
		return nil, fmt.Errorf("empty circuit: no gate instructions found")
	}

	nq := maxQubit + 1
	if nq < nextNamedIdx {
		nq = nextNamedIdx
	}

	warnings := validateCircuit(instructions, nq)

	return &Circuit{
		Instructions: instructions,
		NumQubits:    nq,
		QubitNames:   namedQubits,
		Warnings:     warnings,
	}, nil
}

// validateCircuit runs semantic checks on a complete parsed circuit and returns
// non-fatal warnings. These are issues that compile successfully but likely
// indicate a mistake or will produce unexpected results on real hardware.
func validateCircuit(instructions []Instruction, nq int) []string {
	var w []string

	// No MEASURE — simulation results will all be |0⟩
	hasMeasure := false
	for _, inst := range instructions {
		if inst.Gate == "MEASURE" {
			hasMeasure = true
			break
		}
	}
	if !hasMeasure {
		w = append(w, "no MEASURE instruction — simulation results will all be |0⟩ (add MEASURE at the end)")
	}

	// Qubit count exceeds simulator limit
	if nq > simulatorMaxQubits {
		w = append(w, fmt.Sprintf(
			"circuit uses %d qubits — the QubitLabs simulator supports ≤%d; "+
				"real hardware limits vary (IBM Quantum free tier: 5–127 qubits depending on device)",
			nq, simulatorMaxQubits))
	}

	// Circuit depth exceeds decoherence warning threshold
	depth := calcDepth(instructions, nq)
	if depth > depthWarnThreshold {
		w = append(w, fmt.Sprintf(
			"circuit depth is %d — real quantum hardware decoherence typically limits useful depth to ~%d layers; "+
				"use the simulator for accurate results with deep circuits",
			depth, depthWarnThreshold))
	}

	// No-op rotation gates (angle == 0)
	for _, inst := range instructions {
		switch inst.Gate {
		case "RX", "RY", "RZ", "P":
			if len(inst.Args) > 0 && inst.Args[0] == 0 {
				w = append(w, fmt.Sprintf(
					"line %d: %s(0) is a no-op — it has no effect on the qubit state",
					inst.Line, inst.Gate))
			}
		}
	}

	// Angle > 4π — almost certainly a mistake (common error: using degrees instead of radians)
	for _, inst := range instructions {
		switch inst.Gate {
		case "RX", "RY", "RZ", "P", "CRX", "CRY", "CRZ":
			if len(inst.Args) > 0 {
				angle := inst.Args[0]
				if math.Abs(angle) > 4*math.Pi {
					normalized := math.Mod(angle, 2*math.Pi)
					w = append(w, fmt.Sprintf(
						"line %d: %s angle %.4g rad is unusually large — if you meant degrees, divide by 57.296; "+
							"modulo 2π this is equivalent to %.4g rad",
						inst.Line, inst.Gate, angle, normalized))
				}
			}
		}
	}

	// RESET with no subsequent gates on that qubit (reset at end of circuit = no effect)
	resetIdx := map[int]int{} // qubit → instruction index of last RESET
	for i, inst := range instructions {
		if inst.Gate == "RESET" && len(inst.Qubits) == 1 {
			resetIdx[inst.Qubits[0]] = i
		} else {
			for _, q := range inst.Qubits {
				delete(resetIdx, q) // qubit was used after reset — fine
			}
		}
	}
	for q := range resetIdx {
		w = append(w, fmt.Sprintf(
			"RESET on qubit %d is the last operation on that qubit — resetting without subsequent gates has no useful effect",
			q))
	}

	return w
}

// calcDepth computes the critical-path depth of a circuit using per-qubit layer tracking.
// Depth = the length of the longest sequential chain of gates on any qubit path.
func calcDepth(instructions []Instruction, nq int) int {
	if nq <= 0 {
		return len(instructions)
	}
	qubitDepth := make([]int, nq)

	for _, inst := range instructions {
		if len(inst.Qubits) == 0 {
			// Global gate (MEASURE all, BARRIER all) — synchronises all qubits
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
		for _, q := range inst.Qubits {
			if q < nq && qubitDepth[q] > maxD {
				maxD = qubitDepth[q]
			}
		}
		for _, q := range inst.Qubits {
			if q < nq {
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

// evalAngle parses an angle token that may contain PI.
// Supports: plain float, integer, PI, PI/2, 2*PI, PI/4, etc.
func evalAngle(tok string) (float64, bool) {
	upper := strings.ToUpper(tok)
	s := strings.ReplaceAll(upper, "PI", strconv.FormatFloat(math.Pi, 'f', -1, 64))

	// a*b
	if i := strings.Index(s, "*"); i > 0 {
		a, err1 := strconv.ParseFloat(s[:i], 64)
		b, err2 := strconv.ParseFloat(s[i+1:], 64)
		if err1 == nil && err2 == nil {
			return a * b, true
		}
	}
	// a/b
	if i := strings.LastIndex(s, "/"); i > 0 {
		a, err1 := strconv.ParseFloat(s[:i], 64)
		b, err2 := strconv.ParseFloat(s[i+1:], 64)
		if err1 == nil && err2 == nil && b != 0 {
			return a / b, true
		}
	}
	// plain number (includes the PI replacement itself)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return 0, false
}

func isInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
