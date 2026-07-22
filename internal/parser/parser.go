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
	// Classical condition for IF / WHILE / ASSERT.
	// CondRightBit < 0: c[CondCbit]==CondEq (or register c==CondEq when CondCbit < 0).
	// CondRightBit >= 0: c[CondCbit]==c[CondRightBit].
	CondCbit     int
	CondEq       int
	CondRightBit int // -1 = compare to CondEq
	Body         *Instruction
	ThenBody     []Instruction
	ElseBody     []Instruction
	// WHILE: Cond* + ThenBody; MaxIter caps iterations (required).
	MaxIter int
	// SWITCH: Cases arms; CondCbit selects the discriminant bit (or -1 for int(c)).
	Cases []CaseArm
	// MEASURE: optional classical targets parallel to Qubits (empty → c[q]=measure q).
	MeasTargets []int
	// PARAM type annotation (e.g. "angle"); empty = untyped.
	ParamType string
}

// CaseArm is one SWITCH branch. Value is the match integer; Default marks DEFAULT.
type CaseArm struct {
	Value   int
	Default bool
	Body    []Instruction
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

const validGateNames = "H X Y Z S T SDG TDG SX RX RY RZ P U CNOT CZ SWAP ISWAP CRX CRY CRZ CCX CSWAP MEASURE BARRIER RESET IF FOR WHILE SWITCH PAR ASSERT"

// simulatorMaxQubits is the maximum supported by the QubitLabs browser simulator.
const simulatorMaxQubits = 12

// depthWarnThreshold is the circuit depth at which we warn about hardware decoherence.
const depthWarnThreshold = 100

// Conditions: c[i]==v | c[i]==c[j] | c==v
var (
	condBitEqInt = regexp.MustCompile(`(?i)^c\[(\d+)\]\s*==\s*(\d+)$`)
	condBitEqBit = regexp.MustCompile(`(?i)^c\[(\d+)\]\s*==\s*c\[(\d+)\]$`)
	condRegEqInt = regexp.MustCompile(`(?i)^c\s*==\s*(\d+)$`)

	condExpr = `(?:c\[\d+\]\s*==\s*(?:c\[\d+\]|\d+)|c\s*==\s*\d+)`

	elseHeadRe    = regexp.MustCompile(`(?i)^ELSE\s*\{\s*(.*)$`)
	forHeadRe     = regexp.MustCompile(`(?i)^FOR\s+(\w+)\s+IN\s+(\d+)\s*\.\.\s*(\d+)\s*\{\s*(.*)$`)
	caseHeadRe    = regexp.MustCompile(`(?i)^CASE\s+(\d+)\s*:\s*(.*)$`)
	defaultHeadRe = regexp.MustCompile(`(?i)^DEFAULT\s*:\s*(.*)$`)
	parHeadRe     = regexp.MustCompile(`(?i)^PAR\s*\{\s*(.*)$`)
	assertRe      = regexp.MustCompile(`(?i)^ASSERT\s+(.+)$`)
	measArrowRe   = regexp.MustCompile(`(?i)^MEASURE\s+(.+?)\s*->\s*(.+)$`)
	paramTypedRe  = regexp.MustCompile(`(?i)^PARAM\s+(\w+)\s*:\s*(\w+)\s*$`)
)

func init() {
	ifLineRe = regexp.MustCompile(`(?i)^IF\s+(` + condExpr + `)\s+(.+)$`)
	ifBlockHeadRe = regexp.MustCompile(`(?i)^IF\s+(` + condExpr + `)\s*\{\s*(.*)$`)
	whileHeadRe = regexp.MustCompile(`(?i)^WHILE\s+(` + condExpr + `)\s+MAX\s+(\d+)\s*\{\s*(.*)$`)
	switchHeadRe = regexp.MustCompile(`(?i)^SWITCH\s+(c(?:\[\d+\])?)\s*\{\s*(.*)$`)
}

var (
	ifLineRe      *regexp.Regexp
	ifBlockHeadRe *regexp.Regexp
	whileHeadRe   *regexp.Regexp
	switchHeadRe  *regexp.Regexp
)

// parseCond parses a classical condition expression.
// Returns cbit (-1 = whole register c), eq, rightBit (-1 = int compare).
func parseCond(expr string) (cbit, eq, rightBit int, err error) {
	expr = strings.TrimSpace(expr)
	rightBit = -1
	if m := condBitEqBit.FindStringSubmatch(expr); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		return a, 0, b, nil
	}
	if m := condBitEqInt.FindStringSubmatch(expr); m != nil {
		a, _ := strconv.Atoi(m[1])
		v, _ := strconv.Atoi(m[2])
		return a, v, -1, nil
	}
	if m := condRegEqInt.FindStringSubmatch(expr); m != nil {
		v, _ := strconv.Atoi(m[1])
		return -1, v, -1, nil
	}
	return 0, 0, -1, fmt.Errorf("invalid condition %q — use c[i]==v, c[i]==c[j], or c==v", expr)
}

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

	allLines := strings.Split(src, "\n")
	i := 0

	for i < len(allLines) {
		lineNum := i + 1
		raw := allLines[i]
		i++
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
			name, def, err := parseGateDefLines(tokens, line, allLines, &i, lineNum)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			if _, builtin := gateArity[name]; builtin {
				return nil, fmt.Errorf("line %d: cannot redefine built-in gate %s", lineNum, name)
			}
			gateDefs[name] = def
			continue
		}

		// PARAM theta [, phi]  or  PARAM theta : angle
		if keyword == "PARAM" {
			if m := paramTypedRe.FindStringSubmatch(line); m != nil {
				name, typ := m[1], strings.ToLower(m[2])
				if !isIdent(name) {
					return nil, fmt.Errorf("line %d: invalid param name %q", lineNum, name)
				}
				switch typ {
				case "angle", "float", "real":
					// accepted type annotations (documentation / future binding)
				default:
					return nil, fmt.Errorf("line %d: unknown PARAM type %q (angle|float|real)", lineNum, m[2])
				}
				paramSet[name] = true
				continue
			}
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

		// FOR i IN 0..3 { … } — inclusive range, unrolled at parse time
		if keyword == "FOR" {
			m := forHeadRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid FOR — use FOR i IN 0..3 { ... }", lineNum)
			}
			varName := m[1]
			lo, _ := strconv.Atoi(m[2])
			hi, _ := strconv.Atoi(m[3])
			if hi < lo {
				return nil, fmt.Errorf("line %d: FOR range %d..%d is empty/invalid", lineNum, lo, hi)
			}
			if hi-lo > 64 {
				return nil, fmt.Errorf("line %d: FOR range too large (max 65 iterations)", lineNum)
			}
			bodyLines, err := readBraceBlockLines(allLines, &i, m[4])
			if err != nil {
				return nil, fmt.Errorf("line %d: FOR: %w", lineNum, err)
			}
			for v := lo; v <= hi; v++ {
				expanded := expandLoopVar(bodyLines, varName, v)
				sub, err := Parse(expanded)
				if err != nil {
					return nil, fmt.Errorf("line %d: FOR body (i=%d): %w", lineNum, v, err)
				}
				for _, inst := range sub.Instructions {
					for _, q := range inst.Qubits {
						if q > maxQubit {
							maxQubit = q
						}
					}
					trackIFQubits(&inst, &maxQubit, paramSet)
					instructions = append(instructions, inst)
				}
			}
			continue
		}

		// WHILE <cond> MAX n { … }
		if keyword == "WHILE" {
			m := whileHeadRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid WHILE — use WHILE c[i]==v MAX n { ... }", lineNum)
			}
			cbit, eq, rightBit, err := parseCond(m[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: WHILE: %w", lineNum, err)
			}
			maxIt, _ := strconv.Atoi(m[2])
			if maxIt < 1 || maxIt > 256 {
				return nil, fmt.Errorf("line %d: WHILE MAX must be 1..256", lineNum)
			}
			bodyLines, err := readBraceBlockLines(allLines, &i, m[3])
			if err != nil {
				return nil, fmt.Errorf("line %d: WHILE: %w", lineNum, err)
			}
			bodyCirc, err := Parse(strings.Join(bodyLines, "\n") + "\n")
			if err != nil {
				return nil, fmt.Errorf("line %d: WHILE body: %w", lineNum, err)
			}
			for _, inst := range bodyCirc.Instructions {
				if inst.Gate == "WHILE" {
					return nil, fmt.Errorf("line %d: nested WHILE not supported", lineNum)
				}
				trackIFQubits(&inst, &maxQubit, paramSet)
			}
			trackCondQubits(cbit, rightBit, &maxQubit)
			instructions = append(instructions, Instruction{
				Gate: "WHILE", CondCbit: cbit, CondEq: eq, CondRightBit: rightBit, MaxIter: maxIt,
				ThenBody: bodyCirc.Instructions, Line: lineNum,
			})
			continue
		}

		// SWITCH c[i] | c { CASE n: … DEFAULT: … }
		if keyword == "SWITCH" {
			m := switchHeadRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid SWITCH — use SWITCH c[i] { CASE 0: … DEFAULT: … }", lineNum)
			}
			disc := strings.TrimSpace(m[1])
			cbit := -1
			if strings.EqualFold(disc, "c") {
				cbit = -1
			} else if cm := regexp.MustCompile(`(?i)^c\[(\d+)\]$`).FindStringSubmatch(disc); cm != nil {
				cbit, _ = strconv.Atoi(cm[1])
			} else {
				return nil, fmt.Errorf("line %d: SWITCH discriminant must be c or c[i]", lineNum)
			}
			bodyLines, err := readBraceBlockLines(allLines, &i, m[2])
			if err != nil {
				return nil, fmt.Errorf("line %d: SWITCH: %w", lineNum, err)
			}
			cases, err := parseSwitchCases(bodyLines, lineNum, &maxQubit, paramSet)
			if err != nil {
				return nil, err
			}
			trackCondQubits(cbit, -1, &maxQubit)
			instructions = append(instructions, Instruction{
				Gate: "SWITCH", CondCbit: cbit, CondRightBit: -1, Cases: cases, Line: lineNum,
			})
			continue
		}

		// PAR { … } — commuting parallel layer (sim runs sequentially; depth = 1 layer)
		if keyword == "PAR" {
			m := parHeadRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid PAR — use PAR { GATE …; … }", lineNum)
			}
			bodyLines, err := readBraceBlockLines(allLines, &i, m[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: PAR: %w", lineNum, err)
			}
			bodyCirc, err := Parse(strings.Join(bodyLines, "\n") + "\n")
			if err != nil {
				return nil, fmt.Errorf("line %d: PAR body: %w", lineNum, err)
			}
			seenQ := map[int]bool{}
			for j := range bodyCirc.Instructions {
				inst := &bodyCirc.Instructions[j]
				switch inst.Gate {
				case "IF", "WHILE", "SWITCH", "PAR", "MEASURE", "ASSERT":
					return nil, fmt.Errorf("line %d: PAR body cannot contain %s", lineNum, inst.Gate)
				}
				for _, q := range inst.Qubits {
					if seenQ[q] {
						return nil, fmt.Errorf("line %d: PAR body has overlapping qubit %d — gates must be disjoint", lineNum, q)
					}
					seenQ[q] = true
					if q > maxQubit {
						maxQubit = q
					}
				}
				trackIFQubits(inst, &maxQubit, paramSet)
			}
			instructions = append(instructions, Instruction{
				Gate: "PAR", ThenBody: bodyCirc.Instructions, CondRightBit: -1, Line: lineNum,
			})
			continue
		}

		// ASSERT <cond> — local sim only
		if keyword == "ASSERT" {
			m := assertRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid ASSERT — use ASSERT c[i]==v", lineNum)
			}
			cbit, eq, rightBit, err := parseCond(m[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: ASSERT: %w", lineNum, err)
			}
			trackCondQubits(cbit, rightBit, &maxQubit)
			instructions = append(instructions, Instruction{
				Gate: "ASSERT", CondCbit: cbit, CondEq: eq, CondRightBit: rightBit, Line: lineNum,
			})
			continue
		}

		// MEASURE q -> c[i]  (or MEASURE q0 q1 -> c[0] c[1])
		if keyword == "MEASURE" {
			if m := measArrowRe.FindStringSubmatch(line); m != nil {
				leftCirc, err := Parse("MEASURE " + strings.TrimSpace(m[1]) + "\n")
				if err != nil {
					return nil, fmt.Errorf("line %d: MEASURE: %w", lineNum, err)
				}
				if len(leftCirc.Instructions) != 1 || leftCirc.Instructions[0].Gate != "MEASURE" {
					return nil, fmt.Errorf("line %d: MEASURE left side must be qubit list", lineNum)
				}
				left := leftCirc.Instructions[0]
				targets, err := parseMeasTargets(m[2], len(left.Qubits))
				if err != nil {
					return nil, fmt.Errorf("line %d: MEASURE: %w", lineNum, err)
				}
				for _, q := range left.Qubits {
					if q > maxQubit {
						maxQubit = q
					}
				}
				for _, t := range targets {
					if t > maxQubit {
						maxQubit = t
					}
				}
				instructions = append(instructions, Instruction{
					Gate: "MEASURE", Qubits: left.Qubits, QubitNames: left.QubitNames,
					MeasTargets: targets, CondRightBit: -1, Line: lineNum,
				})
				continue
			}
		}

		// IF — block form or line form
		if keyword == "IF" {
			if m := ifBlockHeadRe.FindStringSubmatch(line); m != nil {
				cbit, eq, rightBit, err := parseCond(m[1])
				if err != nil {
					return nil, fmt.Errorf("line %d: IF: %w", lineNum, err)
				}
				thenLines, err := readBraceBlockLines(allLines, &i, m[2])
				if err != nil {
					return nil, fmt.Errorf("line %d: IF: %w", lineNum, err)
				}
				thenCirc, err := Parse(strings.Join(thenLines, "\n") + "\n")
				if err != nil {
					return nil, fmt.Errorf("line %d: IF then-body: %w", lineNum, err)
				}
				for j := range thenCirc.Instructions {
					if thenCirc.Instructions[j].Gate == "MEASURE" {
						return nil, fmt.Errorf("line %d: IF body cannot contain MEASURE", lineNum)
					}
					trackIFQubits(&thenCirc.Instructions[j], &maxQubit, paramSet)
				}
				var elseBody []Instruction
				for i < len(allLines) {
					peek := strings.TrimSpace(allLines[i])
					if ci := strings.Index(peek, "//"); ci >= 0 {
						peek = strings.TrimSpace(peek[:ci])
					}
					if peek == "" {
						i++
						continue
					}
					if em := elseHeadRe.FindStringSubmatch(peek); em != nil {
						i++
						elseLines, err := readBraceBlockLines(allLines, &i, em[1])
						if err != nil {
							return nil, fmt.Errorf("line %d: ELSE: %w", lineNum, err)
						}
						elseCirc, err := Parse(strings.Join(elseLines, "\n") + "\n")
						if err != nil {
							return nil, fmt.Errorf("line %d: ELSE body: %w", lineNum, err)
						}
						for j := range elseCirc.Instructions {
							if elseCirc.Instructions[j].Gate == "MEASURE" {
								return nil, fmt.Errorf("line %d: ELSE body cannot contain MEASURE", lineNum)
							}
							trackIFQubits(&elseCirc.Instructions[j], &maxQubit, paramSet)
						}
						elseBody = elseCirc.Instructions
					}
					break
				}
				trackCondQubits(cbit, rightBit, &maxQubit)
				instructions = append(instructions, Instruction{
					Gate: "IF", CondCbit: cbit, CondEq: eq, CondRightBit: rightBit,
					ThenBody: thenCirc.Instructions, ElseBody: elseBody, Line: lineNum,
				})
				continue
			}

			m := ifLineRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid IF — use IF c[i]==v GATE or IF c[i]==v { ... }", lineNum)
			}
			cbit, eq, rightBit, err := parseCond(m[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: IF: %w", lineNum, err)
			}
			bodyCirc, err := Parse(m[2])
			if err != nil {
				return nil, fmt.Errorf("line %d: IF body: %w", lineNum, err)
			}
			if len(bodyCirc.Instructions) != 1 {
				return nil, fmt.Errorf("line %d: IF line-form body must be a single gate", lineNum)
			}
			body := bodyCirc.Instructions[0]
			if body.Gate == "IF" || body.Gate == "MEASURE" || body.Gate == "WHILE" || body.Gate == "SWITCH" || body.Gate == "PAR" {
				return nil, fmt.Errorf("line %d: IF body cannot be IF, WHILE, SWITCH, PAR, or MEASURE", lineNum)
			}
			trackIFQubits(&body, &maxQubit, paramSet)
			trackCondQubits(cbit, rightBit, &maxQubit)
			bodyCopy := body
			instructions = append(instructions, Instruction{
				Gate: "IF", CondCbit: cbit, CondEq: eq, CondRightBit: rightBit, Body: &bodyCopy, Line: lineNum,
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

func trackIFQubits(inst *Instruction, maxQubit *int, paramSet map[string]bool) {
	for _, q := range inst.Qubits {
		if q > *maxQubit {
			*maxQubit = q
		}
	}
	for _, t := range inst.MeasTargets {
		if t > *maxQubit {
			*maxQubit = t
		}
	}
	for _, name := range inst.ArgNames {
		if name != "" {
			paramSet[name] = true
		}
	}
	if inst.Body != nil {
		trackIFQubits(inst.Body, maxQubit, paramSet)
	}
	for i := range inst.ThenBody {
		trackIFQubits(&inst.ThenBody[i], maxQubit, paramSet)
	}
	for i := range inst.ElseBody {
		trackIFQubits(&inst.ElseBody[i], maxQubit, paramSet)
	}
	for _, arm := range inst.Cases {
		for i := range arm.Body {
			trackIFQubits(&arm.Body[i], maxQubit, paramSet)
		}
	}
	trackCondQubits(inst.CondCbit, inst.CondRightBit, maxQubit)
}

func trackCondQubits(cbit, rightBit int, maxQubit *int) {
	if cbit > *maxQubit {
		*maxQubit = cbit
	}
	if rightBit > *maxQubit {
		*maxQubit = rightBit
	}
}

var cbitTargetRe = regexp.MustCompile(`(?i)^c\[(\d+)\]$`)

func parseMeasTargets(s string, nQ int) ([]int, error) {
	toks := strings.Fields(strings.ReplaceAll(s, ",", " "))
	if nQ == 0 {
		return nil, fmt.Errorf("MEASURE … -> … requires explicit qubit list on the left")
	}
	if len(toks) != nQ {
		return nil, fmt.Errorf("target count %d != qubit count %d", len(toks), nQ)
	}
	out := make([]int, nQ)
	for i, tok := range toks {
		if m := cbitTargetRe.FindStringSubmatch(tok); m != nil {
			out[i], _ = strconv.Atoi(m[1])
			continue
		}
		if n, err := strconv.Atoi(tok); err == nil && n >= 0 {
			out[i] = n
			continue
		}
		return nil, fmt.Errorf("invalid measure target %q (use c[i] or integer bit index)", tok)
	}
	return out, nil
}

func parseSwitchCases(bodyLines []string, lineNum int, maxQubit *int, paramSet map[string]bool) ([]CaseArm, error) {
	var cases []CaseArm
	var cur *CaseArm
	var bodyAcc []string
	flush := func() error {
		if cur == nil {
			return nil
		}
		src := strings.Join(bodyAcc, "\n")
		if strings.TrimSpace(src) != "" {
			circ, err := Parse(src + "\n")
			if err != nil {
				return fmt.Errorf("line %d: SWITCH case body: %w", lineNum, err)
			}
			for j := range circ.Instructions {
				g := circ.Instructions[j].Gate
				if g == "MEASURE" || g == "SWITCH" {
					return fmt.Errorf("line %d: SWITCH case cannot contain %s", lineNum, g)
				}
				trackIFQubits(&circ.Instructions[j], maxQubit, paramSet)
			}
			cur.Body = circ.Instructions
		}
		cases = append(cases, *cur)
		cur = nil
		bodyAcc = nil
		return nil
	}
	for _, raw := range bodyLines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if m := caseHeadRe.FindStringSubmatch(line); m != nil {
			if err := flush(); err != nil {
				return nil, err
			}
			v, _ := strconv.Atoi(m[1])
			cur = &CaseArm{Value: v}
			if rest := strings.TrimSpace(m[2]); rest != "" {
				bodyAcc = append(bodyAcc, rest)
			}
			continue
		}
		if m := defaultHeadRe.FindStringSubmatch(line); m != nil {
			if err := flush(); err != nil {
				return nil, err
			}
			cur = &CaseArm{Default: true}
			if rest := strings.TrimSpace(m[1]); rest != "" {
				bodyAcc = append(bodyAcc, rest)
			}
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("line %d: SWITCH body must start with CASE or DEFAULT", lineNum)
		}
		bodyAcc = append(bodyAcc, line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("line %d: SWITCH has no CASE arms", lineNum)
	}
	return cases, nil
}

// readBraceBlockLines collects lines until the matching closing }.
// firstInner is text after "{" on the opening line. *idx is the next line to read.
func readBraceBlockLines(lines []string, idx *int, firstInner string) ([]string, error) {
	var buf []string
	depth := 1
	firstInner = strings.TrimSpace(firstInner)
	if firstInner != "" {
		if strings.HasSuffix(firstInner, "}") && !strings.Contains(strings.TrimSuffix(firstInner, "}"), "{") {
			inner := strings.TrimSpace(strings.TrimSuffix(firstInner, "}"))
			if inner != "" {
				buf = append(buf, strings.TrimSuffix(inner, ";"))
			}
			return buf, nil
		}
		// count braces on first remainder
		for _, r := range firstInner {
			if r == '{' {
				depth++
			} else if r == '}' {
				depth--
			}
		}
		if depth == 0 {
			inner := firstInner
			if j := strings.LastIndex(inner, "}"); j >= 0 {
				inner = inner[:j]
			}
			inner = strings.TrimSpace(inner)
			if inner != "" {
				buf = append(buf, strings.TrimSuffix(inner, ";"))
			}
			return buf, nil
		}
		buf = append(buf, strings.TrimSuffix(firstInner, ";"))
	}
	for *idx < len(lines) {
		raw := lines[*idx]
		*idx++
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		for _, r := range s {
			if r == '{' {
				depth++
			} else if r == '}' {
				depth--
			}
		}
		if depth == 0 {
			if strings.HasPrefix(s, "}") {
				rem := strings.TrimSpace(strings.TrimPrefix(s, "}"))
				if rem != "" {
					// put remainder back
					*idx = *idx - 1
					lines[*idx] = rem
				}
				return buf, nil
			}
			// } at end of line with content before
			if j := strings.LastIndex(s, "}"); j >= 0 {
				before := strings.TrimSpace(s[:j])
				if before != "" {
					buf = append(buf, strings.TrimSuffix(before, ";"))
				}
			}
			return buf, nil
		}
		buf = append(buf, strings.TrimSuffix(s, ";"))
	}
	return nil, fmt.Errorf("unclosed {")
}

func expandLoopVar(bodyLines []string, varName string, val int) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(varName) + `\b`)
	repl := strconv.Itoa(val)
	var b strings.Builder
	for _, line := range bodyLines {
		b.WriteString(re.ReplaceAllString(line, repl))
		b.WriteByte('\n')
	}
	return b.String()
}

// parseGateDefLines is parseGateDef for the line-index based Parse loop.
func parseGateDefLines(tokens []string, line string, lines []string, idx *int, lineNum int) (string, gateDef, error) {
	// Reuse scanner-based logic by synthesizing a reader from remaining lines.
	rest := strings.Join(lines[*idx:], "\n")
	full := line + "\n" + rest
	sc := bufio.NewScanner(strings.NewReader(full))
	ln := lineNum
	if !sc.Scan() {
		return "", gateDef{}, fmt.Errorf("empty gate")
	}
	name, def, err := parseGateDef(tokens, sc.Text(), sc, &ln)
	if err != nil {
		return "", gateDef{}, err
	}
	// Advance idx by how many extra lines were consumed (ln - lineNum)
	consumed := ln - lineNum
	*idx += consumed
	return name, def, nil
}

