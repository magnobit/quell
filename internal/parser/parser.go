// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"bufio"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Instruction represents a single parsed gate operation.
type Instruction struct {
	Gate       string
	Qubits     []int
	Args       []float64
	ArgNames   []string // parallel to Args; non-empty = unbound symbolic param
	QubitNames []string
	Line       int // source line number (1-indexed) for diagnostics
	// Conditional (IF): when Gate=="IF", CondCbit/CondEq describe c[i]==v and
	// Body holds the single gated instruction to apply when true.
	CondCbit int
	CondEq   int
	Body     *Instruction
}

// Circuit is the parsed representation of a .quell source file.
type Circuit struct {
	Instructions []Instruction
	NumQubits    int
	QubitNames   map[string]int
	Params       []string // unbound symbolic angle names declared/used
	Warnings     []string // non-fatal issues that compile fine but likely indicate mistakes
	// Local-sim noise (from NOISE directives). Compile targets ignore these.
	NoiseDepolarizing     float64
	NoiseAmplitudeDamping float64
	NoisePhaseDamping     float64
	NoiseReadout          float64
}

// gateDef is a user-defined macro expanded at parse time (not a runtime call).
type gateDef struct {
	params []string
	body   []string // one Quell statement per entry
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

const validGateNames = "H X Y Z S T SDG TDG SX RX RY RZ P U CNOT CZ SWAP ISWAP CRX CRY CRZ CCX CSWAP MEASURE BARRIER RESET IF"

// simulatorMaxQubits is the maximum supported by the QubitLabs browser simulator.
const simulatorMaxQubits = 12

// depthWarnThreshold is the circuit depth at which we warn about hardware decoherence.
const depthWarnThreshold = 100

var ifLineRe = regexp.MustCompile(`(?i)^IF\s+c\[(\d+)\]\s*==\s*(\d+)\s+(.+)$`)

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
	paramSet := map[string]bool{}
	gateDefs := map[string]gateDef{}
	noiseDep := 0.0
	noiseAmp := 0.0
	noisePhase := 0.0
	noiseReadout := 0.0

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

		// NOISE depolarizing 0.01 | amplitude_damping | phase_damping | readout
		if keyword == "NOISE" {
			if len(tokens) < 3 {
				return nil, fmt.Errorf("line %d: NOISE expects <model> <rate> (e.g. NOISE depolarizing 0.01)", lineNum)
			}
			model := strings.ToLower(strings.ReplaceAll(tokens[1], "-", "_"))
			rate, err := strconv.ParseFloat(tokens[2], 64)
			if err != nil || rate < 0 || rate > 1 {
				return nil, fmt.Errorf("line %d: NOISE rate must be a float in [0,1], got %q", lineNum, tokens[2])
			}
			switch model {
			case "depolarizing", "depolarising", "dep":
				noiseDep = rate
			case "amplitude_damping", "amp", "t1":
				noiseAmp = rate
			case "phase_damping", "dephasing", "t2":
				noisePhase = rate
			case "readout", "readout_error", "spam":
				noiseReadout = rate
			default:
				return nil, fmt.Errorf("line %d: unknown noise model %q (depolarizing|amplitude_damping|phase_damping|readout)", lineNum, tokens[1])
			}
			continue
		}

		// gate name p0 p1 { … } — macro expanded at parse time
		if keyword == "GATE" {
			name, def, err := parseGateDef(tokens, line, scanner, &lineNum)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			if _, builtin := gateArity[name]; builtin {
				return nil, fmt.Errorf("line %d: cannot redefine built-in gate %s", lineNum, name)
			}
			gateDefs[name] = def
			continue
		}

		// PARAM theta [, phi] — declare symbolic angles (optional; also inferred from use)
		if keyword == "PARAM" {
			for _, tok := range tokens[1:] {
				name := strings.Trim(tok, ", \t")
				if name == "" {
					continue
				}
				if !isIdent(name) {
					return nil, fmt.Errorf("line %d: invalid param name %q", lineNum, name)
				}
				paramSet[name] = true
			}
			continue
		}

		// IF c[i]==v GATE … — classical conditional (single body gate)
		if keyword == "IF" {
			m := ifLineRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid IF — use IF c[i]==v GATE … (e.g. IF c[0]==1 X 1)", lineNum)
			}
			cbit, _ := strconv.Atoi(m[1])
			eq, _ := strconv.Atoi(m[2])
			bodyCirc, err := Parse(m[3])
			if err != nil {
				return nil, fmt.Errorf("line %d: IF body: %w", lineNum, err)
			}
			if len(bodyCirc.Instructions) != 1 {
				return nil, fmt.Errorf("line %d: IF body must be a single gate", lineNum)
			}
			body := bodyCirc.Instructions[0]
			if body.Gate == "IF" || body.Gate == "MEASURE" {
				return nil, fmt.Errorf("line %d: IF body cannot be IF or MEASURE", lineNum)
			}
			for _, q := range body.Qubits {
				if q > maxQubit {
					maxQubit = q
				}
			}
			for _, name := range body.ArgNames {
				if name != "" {
					paramSet[name] = true
				}
			}
			if cbit > maxQubit {
				maxQubit = cbit
			}
			bodyCopy := body
			instructions = append(instructions, Instruction{
				Gate: "IF", CondCbit: cbit, CondEq: eq, Body: &bodyCopy, Line: lineNum,
			})
			continue
		}

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
		if def, ok := gateDefs[gate]; ok {
			callArgs := tokens[1:]
			if len(callArgs) != len(def.params) {
				return nil, fmt.Errorf("line %d: gate %s requires %d qubit arg(s), got %d", lineNum, gate, len(def.params), len(callArgs))
			}
			subst := map[string]string{}
			for i, p := range def.params {
				arg := strings.TrimRight(callArgs[i], ",")
				if idx, ok := namedQubits[arg]; ok {
					arg = strconv.Itoa(idx)
				} else if _, err := strconv.Atoi(arg); err != nil {
					return nil, fmt.Errorf("line %d: gate %s arg %q is not a qubit index or name", lineNum, gate, arg)
				}
				subst[p] = arg
			}
			expanded := expandGateBody(def.body, subst)
			sub, err := Parse(expanded)
			if err != nil {
				return nil, fmt.Errorf("line %d: expanding gate %s: %w", lineNum, gate, err)
			}
			for _, inst := range sub.Instructions {
				for _, q := range inst.Qubits {
					if q > maxQubit {
						maxQubit = q
					}
				}
				for _, an := range inst.ArgNames {
					if an != "" {
						paramSet[an] = true
					}
				}
				inst.Line = lineNum
				instructions = append(instructions, inst)
			}
			continue
		}

		spec, known := gateArity[gate]
		if !known {
			return nil, fmt.Errorf("line %d: unknown gate %q (valid gates: %s)", lineNum, gate, validGateNames)
		}

		var qubits []int
		var qubitNames []string
		var args []float64
		var argNames []string

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

			// If gate expects more angle args, consume as angle or symbolic param.
			if spec.args > 0 && len(args) < spec.args {
				if f, ok := evalAngle(tok); ok {
					args = append(args, f)
					argNames = append(argNames, "")
					continue
				}
				if isIdent(tok) && !isGateName(tok) {
					args = append(args, 0)
					argNames = append(argNames, tok)
					paramSet[tok] = true
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
				argNames = append(argNames, "")
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
			return nil, fmt.Errorf("line %d: %s requires %d angle argument(s), got %d — use float, PI notation, or a param name (e.g. theta)", lineNum, gate, spec.args, len(args))
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
			ArgNames:   argNames,
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

	params := make([]string, 0, len(paramSet))
	for name := range paramSet {
		params = append(params, name)
	}

	return &Circuit{
		Instructions:          instructions,
		NumQubits:             nq,
		QubitNames:            namedQubits,
		Params:                params,
		Warnings:              warnings,
		NoiseDepolarizing:     noiseDep,
		NoiseAmplitudeDamping: noiseAmp,
		NoisePhaseDamping:     noisePhase,
		NoiseReadout:          noiseReadout,
	}, nil
}

// parseGateDef parses `gate name p0 p1 { body }` spanning one or more lines.
// lineNum points at the GATE line on entry and is advanced for body lines.
func parseGateDef(tokens []string, line string, scanner *bufio.Scanner, lineNum *int) (string, gateDef, error) {
	if len(tokens) < 2 {
		return "", gateDef{}, fmt.Errorf("gate requires a name")
	}
	name := strings.ToUpper(strings.TrimRight(tokens[1], "{,"))
	if !isIdent(name) {
		return "", gateDef{}, fmt.Errorf("invalid gate name %q", tokens[1])
	}

	idx := strings.Index(strings.ToUpper(line), "GATE")
	if idx < 0 {
		return "", gateDef{}, fmt.Errorf("malformed gate header")
	}
	afterGate := strings.TrimSpace(line[idx+4:])
	// drop name
	if !strings.HasPrefix(strings.ToUpper(afterGate), name) {
		// name may be mixed case in source
		parts := strings.Fields(afterGate)
		if len(parts) == 0 {
			return "", gateDef{}, fmt.Errorf("malformed gate header")
		}
		afterGate = strings.TrimSpace(afterGate[len(parts[0]):])
	} else {
		// strip case-insensitively by original token length
		origName := tokens[1]
		afterGate = strings.TrimSpace(afterGate[len(origName):])
	}

	brace := strings.Index(afterGate, "{")
	if brace < 0 {
		return "", gateDef{}, fmt.Errorf("gate %s: expected { … } body", name)
	}
	paramPart := strings.TrimSpace(afterGate[:brace])
	bodyText := afterGate[brace+1:]

	var params []string
	for _, tok := range strings.Fields(strings.ReplaceAll(paramPart, ",", " ")) {
		tok = strings.Trim(tok, ", \t")
		if tok == "" {
			continue
		}
		if !isIdent(tok) {
			return "", gateDef{}, fmt.Errorf("invalid gate param %q", tok)
		}
		params = append(params, tok)
	}

	if j := strings.Index(bodyText, "}"); j >= 0 {
		bodyText = bodyText[:j]
	} else {
		var b strings.Builder
		b.WriteString(bodyText)
		b.WriteByte('\n')
		for scanner.Scan() {
			*lineNum++
			raw := scanner.Text()
			if ci := strings.Index(raw, "//"); ci >= 0 {
				raw = raw[:ci]
			}
			if j := strings.Index(raw, "}"); j >= 0 {
				b.WriteString(raw[:j])
				bodyText = b.String()
				goto parsedBody
			}
			b.WriteString(raw)
			b.WriteByte('\n')
		}
		return "", gateDef{}, fmt.Errorf("gate %s: missing closing }", name)
	}

parsedBody:
	var body []string
	for _, part := range strings.FieldsFunc(bodyText, func(r rune) bool {
		return r == ';' || r == '\n'
	}) {
		s := strings.TrimSpace(part)
		if s != "" {
			body = append(body, s)
		}
	}
	if len(body) == 0 {
		return "", gateDef{}, fmt.Errorf("gate %s: empty body", name)
	}
	return name, gateDef{params: params, body: body}, nil
}

func expandGateBody(body []string, subst map[string]string) string {
	var b strings.Builder
	for _, line := range body {
		toks := strings.Fields(line)
		for i, tok := range toks {
			clean := strings.TrimRight(tok, ",")
			if repl, ok := subst[clean]; ok {
				toks[i] = repl
			}
		}
		b.WriteString(strings.Join(toks, " "))
		b.WriteByte('\n')
	}
	return b.String()
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

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func isGateName(s string) bool {
	_, ok := gateArity[strings.ToUpper(s)]
	return ok
}
