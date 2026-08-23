// Copyright 2026 Magnobit, Inc. All rights reserved.

package migrate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/magnobit/quell/decompose"
)

var cirqRules = []gateRule{
	// Indexed: cirq.H(q[0]) — also used by LineQubit.range assigned to `q`
	{regexp.MustCompile(`cirq\.H\(q\[(\d+)\]\)`), "H $1"},
	{regexp.MustCompile(`cirq\.X\(q\[(\d+)\]\)`), "X $1"},
	{regexp.MustCompile(`cirq\.Y\(q\[(\d+)\]\)`), "Y $1"},
	{regexp.MustCompile(`cirq\.Z\(q\[(\d+)\]\)`), "Z $1"},
	{regexp.MustCompile(`cirq\.S\(q\[(\d+)\]\)`), "S $1"},
	{regexp.MustCompile(`cirq\.T\(q\[(\d+)\]\)`), "T $1"},
	{regexp.MustCompile(`cirq\.CNOT\(q\[(\d+)\],\s*q\[(\d+)\]\)`), "CNOT $1 $2"},
	{regexp.MustCompile(`cirq\.CZ\(q\[(\d+)\],\s*q\[(\d+)\]\)`), "CZ $1 $2"},
	{regexp.MustCompile(`cirq\.SWAP\(q\[(\d+)\],\s*q\[(\d+)\]\)`), "SWAP $1 $2"},
	{regexp.MustCompile(`cirq\.measure\(.*\)`), "MEASURE"},
}

// Cirq idioms use named qubits (q0, q1) and often pack gates into one Circuit(...).
// Token: q0 | q[0] | qubits[1]
const cirqQubitTok = `([a-zA-Z_]\w*(?:\s*\[\s*\d+\s*\])?)`

var (
	cirqRangeAssign = regexp.MustCompile(`(?m)([a-zA-Z_]\w*(?:\s*,\s*[a-zA-Z_]\w*)*)\s*=\s*cirq\.LineQubit\.range\s*\(\s*(\d+)\s*\)`)
	cirqIndexed     = regexp.MustCompile(`(?i)^([a-zA-Z_]\w*)\s*\[\s*(\d+)\s*\]$`)
	cirqNamedDigit  = regexp.MustCompile(`(?i)^q(\d+)$`)

	cirqOpH    = regexp.MustCompile(`cirq\.H\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpX    = regexp.MustCompile(`cirq\.X\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpY    = regexp.MustCompile(`cirq\.Y\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpZ    = regexp.MustCompile(`cirq\.Z\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpS    = regexp.MustCompile(`cirq\.S\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpT    = regexp.MustCompile(`cirq\.T\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpCNOT = regexp.MustCompile(`cirq\.CNOT\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)`)
	cirqOpCX   = regexp.MustCompile(`cirq\.CX\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)`)
	cirqOpCZ   = regexp.MustCompile(`cirq\.CZ\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)`)
	cirqOpSWAP = regexp.MustCompile(`cirq\.SWAP\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)`)
	cirqOpRX   = regexp.MustCompile(`cirq\.rx\(\s*([^)]+)\s*\)\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpRY   = regexp.MustCompile(`cirq\.ry\(\s*([^)]+)\s*\)\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpRZ   = regexp.MustCompile(`cirq\.rz\(\s*([^)]+)\s*\)\(\s*` + cirqQubitTok + `\s*\)`)
	cirqOpMeas = regexp.MustCompile(`cirq\.measure\(([^)]*)\)`)

	cirqPhasedX = regexp.MustCompile(
		`(?i)cirq\.PhasedXPowGate\(\s*phase_exponent\s*=\s*([^,\)]+)\s*(?:,\s*exponent\s*=\s*([^,\)]+))?\s*\)\s*\(\s*` + cirqQubitTok + `\s*\)`)
	cirqXPow     = regexp.MustCompile(`(?i)cirq\.XPowGate\(\s*exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*\)`)
	cirqYPow     = regexp.MustCompile(`(?i)cirq\.YPowGate\(\s*exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*\)`)
	cirqZPow     = regexp.MustCompile(`(?i)cirq\.ZPowGate\(\s*exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*\)`)
	cirqHPow     = regexp.MustCompile(`(?i)cirq\.HPowGate\(\s*exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*\)`)
	cirqXXPow    = regexp.MustCompile(`(?i)cirq\.XXPowGate\(\s*exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)`)
	cirqZZPow    = regexp.MustCompile(`(?i)cirq\.ZZPowGate\(\s*exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)`)
	cirqPhasedXZ = regexp.MustCompile(
		`(?i)cirq\.PhasedXZGate\(\s*x_exponent\s*=\s*([^,]+)\s*,\s*z_exponent\s*=\s*([^,]+)\s*,\s*axis_phase_exponent\s*=\s*([^)]+)\s*\)\s*\(\s*` + cirqQubitTok + `\s*\)`)
)

type cirqHit struct {
	pos  int
	line string
}

func parseCirqQubitNames(code string) map[string]int {
	names := map[string]int{}
	for _, m := range cirqRangeAssign.FindAllStringSubmatch(code, -1) {
		parts := strings.Split(m[1], ",")
		for i, p := range parts {
			name := strings.TrimSpace(p)
			if name == "" {
				continue
			}
			names[name] = i
		}
	}
	return names
}

func resolveCirqQubit(tok string, names map[string]int) (int, bool) {
	tok = strings.TrimSpace(tok)
	if tok == "" || strings.HasPrefix(tok, "*") {
		return 0, false
	}
	if m := cirqIndexed.FindStringSubmatch(tok); m != nil {
		var n int
		fmt.Sscanf(m[2], "%d", &n)
		return n, true
	}
	if id, ok := names[tok]; ok {
		return id, true
	}
	if m := cirqNamedDigit.FindStringSubmatch(tok); m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		return n, true
	}
	return 0, false
}

// convertCirqToQuell extracts cirq.GATE(...) calls anywhere in the source
// (including multi-gate Circuit(...) lines) and maps named qubits to indices.
func convertCirqToQuell(code string) (string, []string, error) {
	names := parseCirqQubitNames(code)
	resolve := func(tok string) (int, error) {
		n, ok := resolveCirqQubit(tok, names)
		if !ok {
			return 0, fmt.Errorf("unresolved Cirq qubit %q", tok)
		}
		return n, nil
	}

	var warnings []string
	var hits []cirqHit
	addUnary := func(re *regexp.Regexp, gate string) {
		for _, m := range re.FindAllStringSubmatchIndex(code, -1) {
			full := code[m[0]:m[1]]
			tail := strings.TrimSpace(code[m[1]:])
			if strings.HasPrefix(tail, ".with_classical_controls") {
				continue // handled below as IF
			}
			sub := re.FindStringSubmatch(full)
			if sub == nil {
				continue
			}
			n, err := resolve(sub[1])
			if err != nil {
				warnings = append(warnings, err.Error()+" in "+full)
				continue
			}
			hits = append(hits, cirqHit{pos: m[0], line: fmt.Sprintf("%s %d", gate, n)})
		}
	}
	addBinary := func(re *regexp.Regexp, gate string) {
		for _, m := range re.FindAllStringSubmatchIndex(code, -1) {
			full := code[m[0]:m[1]]
			tail := strings.TrimSpace(code[m[1]:])
			if strings.HasPrefix(tail, ".with_classical_controls") {
				continue
			}
			sub := re.FindStringSubmatch(full)
			if sub == nil {
				continue
			}
			a, errA := resolve(sub[1])
			b, errB := resolve(sub[2])
			if errA != nil || errB != nil {
				warnings = append(warnings, "unresolved Cirq qubits in "+full)
				continue
			}
			hits = append(hits, cirqHit{pos: m[0], line: fmt.Sprintf("%s %d %d", gate, a, b)})
		}
	}

	addUnary(cirqOpH, "H")
	addUnary(cirqOpX, "X")
	addUnary(cirqOpY, "Y")
	addUnary(cirqOpZ, "Z")
	addUnary(cirqOpS, "S")
	addUnary(cirqOpT, "T")
	addBinary(cirqOpCNOT, "CNOT")
	addBinary(cirqOpCX, "CNOT")
	addBinary(cirqOpCZ, "CZ")
	addBinary(cirqOpSWAP, "SWAP")

	addParamUnary := func(re *regexp.Regexp, gate string) {
		for _, m := range re.FindAllStringSubmatchIndex(code, -1) {
			full := code[m[0]:m[1]]
			sub := re.FindStringSubmatch(full)
			if sub == nil {
				continue
			}
			n, err := resolve(sub[2])
			if err != nil {
				warnings = append(warnings, err.Error()+" in "+full)
				continue
			}
			angle := strings.TrimSpace(sub[1])
			hits = append(hits, cirqHit{pos: m[0], line: fmt.Sprintf("%s %s %d", gate, angle, n)})
		}
	}
	addParamUnary(cirqOpRX, "RX")
	addParamUnary(cirqOpRY, "RY")
	addParamUnary(cirqOpRZ, "RZ")

	// Classically controlled ops: cirq.X(q1).with_classical_controls('c0')
	cirqCtrl := regexp.MustCompile(`cirq\.(H|X|Y|Z|S|T)\(\s*` + cirqQubitTok + `\s*\)\.with_classical_controls\(\s*['\"](?:c|m)?(\d+)['\"]\s*\)`)
	cirqCtrl2 := regexp.MustCompile(`cirq\.(CNOT|CX|CZ|SWAP)\(\s*` + cirqQubitTok + `\s*,\s*` + cirqQubitTok + `\s*\)\.with_classical_controls\(\s*['\"](?:c|m)?(\d+)['\"]\s*\)`)
	for _, m := range cirqCtrl.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqCtrl.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[2])
		if err != nil {
			warnings = append(warnings, err.Error()+" in "+full)
			continue
		}
		cbit := sub[3]
		hits = append(hits, cirqHit{pos: m[0], line: fmt.Sprintf("IF c[%s]==1 %s %d", cbit, sub[1], n)})
	}
	for _, m := range cirqCtrl2.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqCtrl2.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		a, errA := resolve(sub[2])
		b, errB := resolve(sub[3])
		if errA != nil || errB != nil {
			warnings = append(warnings, "unresolved Cirq qubits in "+full)
			continue
		}
		gate := sub[1]
		if gate == "CX" {
			gate = "CNOT"
		}
		cbit := sub[4]
		hits = append(hits, cirqHit{pos: m[0], line: fmt.Sprintf("IF c[%s]==1 %s %d %d", cbit, gate, a, b)})
	}

	addDecompHits := func(pos int, r decompose.Result) {
		for i, line := range decompose.FormatAnnotate(r) {
			hits = append(hits, cirqHit{pos: pos + i, line: line})
		}
	}
	for _, m := range cirqPhasedX.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqPhasedX.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[3])
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		exp := "1"
		if strings.TrimSpace(sub[2]) != "" {
			exp = sub[2]
		}
		addDecompHits(m[0], decompose.PhasedXPow(sub[1], exp, n))
	}
	for _, m := range cirqXPow.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqXPow.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[2])
		if err != nil {
			continue
		}
		addDecompHits(m[0], decompose.XPow(sub[1], n))
	}
	for _, m := range cirqYPow.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqYPow.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[2])
		if err != nil {
			continue
		}
		addDecompHits(m[0], decompose.YPow(sub[1], n))
	}
	for _, m := range cirqZPow.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqZPow.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[2])
		if err != nil {
			continue
		}
		addDecompHits(m[0], decompose.ZPow(sub[1], n))
	}
	for _, m := range cirqHPow.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqHPow.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[2])
		if err != nil {
			continue
		}
		addDecompHits(m[0], decompose.HPow(sub[1], n))
	}
	for _, m := range cirqXXPow.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqXXPow.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		a, errA := resolve(sub[2])
		b, errB := resolve(sub[3])
		if errA != nil || errB != nil {
			continue
		}
		addDecompHits(m[0], decompose.XXPow(sub[1], a, b))
	}
	for _, m := range cirqZZPow.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqZZPow.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		a, errA := resolve(sub[2])
		b, errB := resolve(sub[3])
		if errA != nil || errB != nil {
			continue
		}
		addDecompHits(m[0], decompose.ZZPow(sub[1], a, b))
	}
	for _, m := range cirqPhasedXZ.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := cirqPhasedXZ.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		n, err := resolve(sub[4])
		if err != nil {
			continue
		}
		addDecompHits(m[0], decompose.PhasedXZ(sub[1], sub[2], sub[3], n))
	}

	for _, m := range cirqOpMeas.FindAllStringSubmatchIndex(code, -1) {
		hits = append(hits, cirqHit{pos: m[0], line: "MEASURE"})
	}

	// Soft-warn on other cirq.* ops we don't map.
	cirqAny := regexp.MustCompile(`cirq\.([A-Za-z_][A-Za-z0-9_]*)`)
	supported := map[string]bool{
		"H": true, "X": true, "Y": true, "Z": true, "S": true, "T": true,
		"CNOT": true, "CX": true, "CZ": true, "SWAP": true,
		"rx": true, "ry": true, "rz": true, "measure": true,
		"Circuit": true, "LineQubit": true, "GridQubit": true, "NamedQubit": true,
		"Simulator": true, "DensityMatrixSimulator": true, "ops": true,
		"KeyCondition": true, "ClassicallyControlledOperation": true,
		"PhasedXPowGate": true, "PhasedXZGate": true,
		"XPowGate": true, "YPowGate": true, "ZPowGate": true, "HPowGate": true,
		"XXPowGate": true, "ZZPowGate": true, "YYPowGate": true,
	}
	seen := map[string]bool{}
	for _, m := range cirqAny.FindAllStringSubmatch(code, -1) {
		name := m[1]
		if supported[name] {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		warnings = append(warnings, fmt.Sprintf("unsupported Cirq op cirq.%s — not in Quell subset yet", name))
		if len(warnings) >= 16 {
			warnings = append(warnings, "…additional unsupported Cirq ops omitted")
			break
		}
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })

	var out []string
	out = append(out, "// Converted from Cirq")
	for _, h := range hits {
		out = append(out, h.line)
	}
	if len(out) <= 1 {
		return "", warnings, fmt.Errorf("no supported Cirq gates found — use cirq.H(q0) / cirq.CNOT(q0, q1) style, or OpenQASM")
	}
	return strings.Join(out, "\n") + "\n", warnings, nil
}
