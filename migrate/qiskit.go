// Copyright 2026 Magnobit, Inc. All rights reserved.

package migrate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/magnobit/quell/decompose"
)

type gateRule struct {
	pat   *regexp.Regexp
	quell string
}

var qiskitRules = []gateRule{
	// Legacy c_if must precede plain gate rules (same line starts with qc.x(...).c_if)
	{regexp.MustCompile(`qc\.h\((\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$2]==$3 H $1"},
	{regexp.MustCompile(`qc\.x\((\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$2]==$3 X $1"},
	{regexp.MustCompile(`qc\.y\((\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$2]==$3 Y $1"},
	{regexp.MustCompile(`qc\.z\((\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$2]==$3 Z $1"},
	{regexp.MustCompile(`qc\.s\((\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$2]==$3 S $1"},
	{regexp.MustCompile(`qc\.t\((\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$2]==$3 T $1"},
	{regexp.MustCompile(`qc\.cx\((\d+),\s*(\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$3]==$4 CNOT $1 $2"},
	{regexp.MustCompile(`qc\.cnot\((\d+),\s*(\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$3]==$4 CNOT $1 $2"},
	{regexp.MustCompile(`qc\.cz\((\d+),\s*(\d+)\)\.c_if\(\s*(?:\w+\[)?(\d+)\]?\s*,\s*(\d+)\s*\)`), "IF c[$3]==$4 CZ $1 $2"},
	// Single-qubit gates
	{regexp.MustCompile(`qc\.h\((\d+)\)`), "H $1"},
	{regexp.MustCompile(`qc\.x\((\d+)\)`), "X $1"},
	{regexp.MustCompile(`qc\.y\((\d+)\)`), "Y $1"},
	{regexp.MustCompile(`qc\.z\((\d+)\)`), "Z $1"},
	{regexp.MustCompile(`qc\.s\((\d+)\)`), "S $1"},
	{regexp.MustCompile(`qc\.t\((\d+)\)`), "T $1"},
	{regexp.MustCompile(`qc\.sdg\((\d+)\)`), "SDG $1"},
	{regexp.MustCompile(`qc\.tdg\((\d+)\)`), "TDG $1"},
	{regexp.MustCompile(`qc\.sx\((\d+)\)`), "SX $1"},
	{regexp.MustCompile(`qc\.rx\(([^,)]+),\s*(\d+)\)`), "RX $1 $2"},
	{regexp.MustCompile(`qc\.ry\(([^,)]+),\s*(\d+)\)`), "RY $1 $2"},
	{regexp.MustCompile(`qc\.rz\(([^,)]+),\s*(\d+)\)`), "RZ $1 $2"},
	{regexp.MustCompile(`qc\.p\(([^,)]+),\s*(\d+)\)`), "P $1 $2"},
	{regexp.MustCompile(`qc\.cx\((\d+),\s*(\d+)\)`), "CNOT $1 $2"},
	{regexp.MustCompile(`qc\.cnot\((\d+),\s*(\d+)\)`), "CNOT $1 $2"},
	{regexp.MustCompile(`qc\.cz\((\d+),\s*(\d+)\)`), "CZ $1 $2"},
	{regexp.MustCompile(`qc\.swap\((\d+),\s*(\d+)\)`), "SWAP $1 $2"},
	{regexp.MustCompile(`qc\.iswap\((\d+),\s*(\d+)\)`), "ISWAP $1 $2"},
	{regexp.MustCompile(`qc\.ccx\((\d+),\s*(\d+),\s*(\d+)\)`), "CCX $1 $2 $3"},
	{regexp.MustCompile(`qc\.cswap\((\d+),\s*(\d+),\s*(\d+)\)`), "CSWAP $1 $2 $3"},
	{regexp.MustCompile(`qc\.crz\(([^,)]+),\s*(\d+),\s*(\d+)\)`), "CRZ $1 $2 $3"},
	{regexp.MustCompile(`qc\.cp\(([^,)]+),\s*(\d+),\s*(\d+)\)`), "CRZ $1 $2 $3"},
	{regexp.MustCompile(`qc\.barrier\((.*)\)`), "BARRIER"},
	{regexp.MustCompile(`qc\.barrier\(\)`), "BARRIER"},
	{regexp.MustCompile(`qc\.reset\((\d+)\)`), "RESET $1"},
	{regexp.MustCompile(`qc\.id\((\d+)\)`), "BARRIER $1"},
	{regexp.MustCompile(`qc\.measure_all\(\)`), "MEASURE"},
	{regexp.MustCompile(`qc\.measure\((\d+),\s*\d+\)`), "MEASURE $1"},
	{regexp.MustCompile(`qc\.measure\(\[.*?\].*?\)`), "MEASURE"},
	// Amazon Braket Circuit() style (also used when language=braket)
	{regexp.MustCompile(`(?i)circuit\.h\((\d+)\)`), "H $1"},
	{regexp.MustCompile(`(?i)circuit\.x\((\d+)\)`), "X $1"},
	{regexp.MustCompile(`(?i)circuit\.y\((\d+)\)`), "Y $1"},
	{regexp.MustCompile(`(?i)circuit\.z\((\d+)\)`), "Z $1"},
	{regexp.MustCompile(`(?i)circuit\.s\((\d+)\)`), "S $1"},
	{regexp.MustCompile(`(?i)circuit\.t\((\d+)\)`), "T $1"},
	{regexp.MustCompile(`(?i)circuit\.rx\((\d+),\s*([^)]+)\)`), "RX $2 $1"},
	{regexp.MustCompile(`(?i)circuit\.ry\((\d+),\s*([^)]+)\)`), "RY $2 $1"},
	{regexp.MustCompile(`(?i)circuit\.rz\((\d+),\s*([^)]+)\)`), "RZ $2 $1"},
	{regexp.MustCompile(`(?i)circuit\.cnot\((\d+),\s*(\d+)\)`), "CNOT $1 $2"},
	{regexp.MustCompile(`(?i)circuit\.cz\((\d+),\s*(\d+)\)`), "CZ $1 $2"},
	{regexp.MustCompile(`(?i)circuit\.swap\((\d+),\s*(\d+)\)`), "SWAP $1 $2"},
	{regexp.MustCompile(`(?i)circuit\.ccnot\((\d+),\s*(\d+),\s*(\d+)\)`), "CCX $1 $2 $3"},
	{regexp.MustCompile(`(?i)circuit\.measure\((\d+)\)`), "MEASURE $1"},
}

var (
	qcRXX  = regexp.MustCompile(`(?i)qc\.rxx\(\s*([^,]+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcRYY  = regexp.MustCompile(`(?i)qc\.ryy\(\s*([^,]+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcRZZ  = regexp.MustCompile(`(?i)qc\.rzz\(\s*([^,]+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcRZX  = regexp.MustCompile(`(?i)qc\.rzx\(\s*([^,]+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcECR  = regexp.MustCompile(`(?i)qc\.ecr\(\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcCY   = regexp.MustCompile(`(?i)qc\.cy\(\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcCH   = regexp.MustCompile(`(?i)qc\.ch\(\s*(\d+)\s*,\s*(\d+)\s*\)`)
	qcSXDG = regexp.MustCompile(`(?i)qc\.sxdg\(\s*(\d+)\s*\)`)
	qcU3   = regexp.MustCompile(`(?i)qc\.u(?:3)?\(\s*([^,]+)\s*,\s*([^,]+)\s*,\s*([^,]+)\s*,\s*(\d+)\s*\)`)
	qcU2   = regexp.MustCompile(`(?i)qc\.u2\(\s*([^,]+)\s*,\s*([^,]+)\s*,\s*(\d+)\s*\)`)
)

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

// tryQiskitDecompose maps one source line to Quell decomposition lines.
func tryQiskitDecompose(line string) (lines []string, ok bool) {
	appendR := func(r decompose.Result) ([]string, bool) {
		return decompose.FormatAnnotate(r), true
	}
	if m := qcRXX.FindStringSubmatch(line); m != nil {
		return appendR(decompose.RXX(m[1], atoiOr(m[2], 0), atoiOr(m[3], 1)))
	}
	if m := qcRYY.FindStringSubmatch(line); m != nil {
		return appendR(decompose.RYY(m[1], atoiOr(m[2], 0), atoiOr(m[3], 1)))
	}
	if m := qcRZZ.FindStringSubmatch(line); m != nil {
		return appendR(decompose.RZZ(m[1], atoiOr(m[2], 0), atoiOr(m[3], 1)))
	}
	if m := qcRZX.FindStringSubmatch(line); m != nil {
		return appendR(decompose.RZX(m[1], atoiOr(m[2], 0), atoiOr(m[3], 1)))
	}
	if m := qcECR.FindStringSubmatch(line); m != nil {
		return appendR(decompose.ECR(atoiOr(m[1], 0), atoiOr(m[2], 1)))
	}
	if m := qcCY.FindStringSubmatch(line); m != nil {
		return appendR(decompose.CY(atoiOr(m[1], 0), atoiOr(m[2], 1)))
	}
	if m := qcCH.FindStringSubmatch(line); m != nil {
		return appendR(decompose.CH(atoiOr(m[1], 0), atoiOr(m[2], 1)))
	}
	if m := qcSXDG.FindStringSubmatch(line); m != nil {
		return appendR(decompose.SXDG(atoiOr(m[1], 0)))
	}
	if m := qcU3.FindStringSubmatch(line); m != nil {
		return []string{
			"// from qc.u/u3",
			fmt.Sprintf("U %s %s %s %s", decompose.NormAngle(m[1]), decompose.NormAngle(m[2]), decompose.NormAngle(m[3]), m[4]),
		}, true
	}
	if m := qcU2.FindStringSubmatch(line); m != nil {
		return []string{
			"// from qc.u2",
			fmt.Sprintf("U PI/2 %s %s %s", decompose.NormAngle(m[1]), decompose.NormAngle(m[2]), m[3]),
		}, true
	}
	return nil, false
}

// convertPythonToQuell handles Qiskit (and falls back to Cirq scan). Prefer
// convertCirqToQuell when language is known Cirq.
func convertPythonToQuell(code string) (string, []string, error) {
	if strings.Contains(code, "cirq.") || strings.Contains(code, "import cirq") {
		return convertCirqToQuell(code)
	}
	out, warnings := convertToQuell(code)
	if !quellHasGates(out) {
		return "", warnings, fmt.Errorf("no supported Qiskit/Cirq gates found — try OpenQASM or a simpler circuit snippet")
	}
	return out, warnings, nil
}

func quellHasGates(src string) bool {
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		return true
	}
	return false
}

// convertToQuell is the line-oriented Qiskit/Cirq regex converter.
func convertToQuell(code string) (string, []string) {
	skipPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^from\s+`),
		regexp.MustCompile(`^import\s+`),
		regexp.MustCompile(`QuantumCircuit\(`),
		regexp.MustCompile(`^\s*#`),
		regexp.MustCompile(`circuit\s*=\s*Circuit\(\)`),
		regexp.MustCompile(`q\s*=\s*cirq\.`),
		regexp.MustCompile(`=\s*cirq\.LineQubit`),
		regexp.MustCompile(`^\s*(qc|circuit)\s*=`),
		regexp.MustCompile(`\.draw\(`),
		regexp.MustCompile(`print\(`),
		regexp.MustCompile(`Aer\.|execute\(|transpile\(|assemble\(`),
		regexp.MustCompile(`Backend|Job|Result|Sampler|Estimator`),
	}
	qcCall := regexp.MustCompile(`(?i)\bqc\.([a-z_][a-z0-9_]*)\s*\(`)
	ifTestRe := regexp.MustCompile(`(?i)^with\s+qc\.if_test\(\s*\(\s*(?:qc\.clbits\[(\d+)\]|(\w+)\[(\d+)\]|(\d+))\s*,\s*(\d+)\s*\)\s*\)(?:\s+as\s+(\w+))?\s*:?\s*$`)
	elseWithRe := regexp.MustCompile(`(?i)^with\s+(\w+)\s*:?\s*$`)
	indentOf := func(s string) int {
		n := 0
		for _, r := range s {
			if r == ' ' {
				n++
			} else if r == '\t' {
				n += 4
			} else {
				break
			}
		}
		return n
	}
	mapGateLine := func(line string) string {
		for _, rule := range qiskitRules {
			if rule.pat.MatchString(line) {
				return strings.TrimSpace(rule.pat.ReplaceAllString(line, rule.quell))
			}
		}
		return ""
	}

	lines := strings.Split(code, "\n")
	var out []string
	var warnings []string
	out = append(out, "// Converted to Quell")
	seenWarn := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		// for i in range(n): / for i in range(a, b): — unroll like Quell FOR
		if m := regexp.MustCompile(`(?i)^for\s+(\w+)\s+in\s+range\s*\(\s*(\d+)\s*(?:,\s*(\d+)\s*)?\)\s*:?\s*$`).FindStringSubmatch(line); m != nil {
			varName := m[1]
			lo, hi := 0, 0
			if m[3] != "" {
				lo, _ = strconv.Atoi(m[2])
				hi, _ = strconv.Atoi(m[3])
				hi-- // Python range end exclusive
			} else {
				hi, _ = strconv.Atoi(m[2])
				hi--
			}
			baseIndent := indentOf(raw)
			var body []string
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "" {
					continue
				}
				if indentOf(lines[j]) <= baseIndent {
					break
				}
				body = append(body, lines[j])
			}
			if hi >= lo && hi-lo <= 64 {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(varName) + `\b`)
				for v := lo; v <= hi; v++ {
					vs := strconv.Itoa(v)
					for _, bl := range body {
						exp := re.ReplaceAllString(strings.TrimSpace(bl), vs)
						if decomp, ok := tryQiskitDecompose(exp); ok {
							out = append(out, decomp...)
							continue
						}
						if g := mapGateLine(exp); g != "" {
							out = append(out, g)
						} else {
							out = append(out, "// "+exp)
						}
					}
				}
			} else {
				warnings = append(warnings, fmt.Sprintf("line %d: for-range skipped (empty or too large)", i+1))
			}
			i = j - 1
			continue
		}

		// Modern Qiskit: with qc.if_test((c[0], 1)): ... / as else_:
		if m := ifTestRe.FindStringSubmatch(line); m != nil {
			cbit := m[1]
			if cbit == "" {
				cbit = m[3]
			}
			if cbit == "" {
				cbit = m[4]
			}
			eq := m[5]
			elseName := m[6]
			baseIndent := indentOf(raw)
			var bodies []string
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "" {
					continue
				}
				if indentOf(lines[j]) <= baseIndent {
					break
				}
				bodies = append(bodies, strings.TrimSpace(lines[j]))
			}
			for _, b := range bodies {
				g := mapGateLine(b)
				if g == "" || strings.HasPrefix(g, "IF ") {
					warnings = append(warnings, fmt.Sprintf("line %d: unsupported gate inside if_test: %s", i+1, b))
					out = append(out, "// "+b)
					continue
				}
				out = append(out, fmt.Sprintf("IF c[%s]==%s %s", cbit, eq, g))
			}
			i = j - 1
			// Optional else block: with else_:
			if elseName != "" && i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if em := elseWithRe.FindStringSubmatch(next); em != nil && em[1] == elseName {
					elseIndent := indentOf(lines[i+1])
					elseEq := "0"
					if eq == "0" {
						elseEq = "1"
					}
					k := i + 2
					for ; k < len(lines); k++ {
						if strings.TrimSpace(lines[k]) == "" {
							continue
						}
						if indentOf(lines[k]) <= elseIndent {
							break
						}
						b := strings.TrimSpace(lines[k])
						g := mapGateLine(b)
						if g == "" || strings.HasPrefix(g, "IF ") {
							warnings = append(warnings, fmt.Sprintf("line %d: unsupported gate inside else: %s", k+1, b))
							out = append(out, "// "+b)
							continue
						}
						if eq != "0" && eq != "1" {
							warnings = append(warnings, "else after non-binary if_test skipped")
							continue
						}
						out = append(out, fmt.Sprintf("IF c[%s]==%s %s", cbit, elseEq, g))
					}
					i = k - 1
				}
			}
			continue
		}

		skip := false
		for _, p := range skipPatterns {
			if p.MatchString(line) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if strings.HasPrefix(line, "#") {
			out = append(out, "//"+strings.TrimPrefix(line, "#"))
			continue
		}

		if decomp, ok := tryQiskitDecompose(line); ok {
			out = append(out, decomp...)
			continue
		}

		converted := mapGateLine(line)
		if converted == "" {
			for _, rule := range cirqRules {
				if rule.pat.MatchString(line) {
					converted = strings.TrimSpace(rule.pat.ReplaceAllString(line, rule.quell))
					break
				}
			}
		}

		if converted != "" {
			out = append(out, converted)
			continue
		}

		out = append(out, "// "+line)
		if m := qcCall.FindStringSubmatch(line); m != nil {
			op := m[1]
			if !seenWarn[op] {
				seenWarn[op] = true
				warnings = append(warnings, fmt.Sprintf("line %d: unsupported Qiskit call qc.%s — not in Quell subset yet", i+1, op))
			}
		}
	}

	return strings.Join(out, "\n"), warnings
}
