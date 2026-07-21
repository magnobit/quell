// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qasmimport provides a thin OpenQASM 3 → Quell translator for the
// common gate subset (not full QASM 3 parity).
package qasmimport

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	qubitArrayRe = regexp.MustCompile(`(?i)^qubit\s*\[\s*(\d+)\s*\]\s+(\w+)\s*;?\s*$`)
	qubitOneRe   = regexp.MustCompile(`(?i)^qubit\s+(\w+)\s*;?\s*$`)
	gateCallRe   = regexp.MustCompile(`(?i)^([a-z][a-z0-9_]*)\s*(?:\(([^)]*)\))?\s+(.+?)\s*;?\s*$`)
	qRefRe       = regexp.MustCompile(`(?i)(\w+)\s*\[\s*(\d+)\s*\]|(\w+)`)
	measureRe    = regexp.MustCompile(`(?i)^measure\s+(.+?)\s*(?:->\s*.+)?\s*;?\s*$`)
	ifRe         = regexp.MustCompile(`(?i)^if\s*\(\s*(\w+)\s*\[\s*(\d+)\s*\]\s*==\s*(\d+)\s*\)\s*(.+?)\s*;?\s*$`)
)

// ToQuell converts a subset of OpenQASM 3 into Quell source.
// Supported: qubit decls, h/x/y/z/s/t/sx/rx/ry/rz/p, cx/cnot/cz/swap,
// ccx/toffoli, barrier, reset, measure. Unknown statements become comments.
func ToQuell(src string) (string, error) {
	var out strings.Builder
	out.WriteString("// Converted from OpenQASM (thin import — common gates only)\n")

	regs := map[string]int{} // register name → size (1 for scalar qubit)
	nextIdx := 0
	qubitIndex := map[string]int{} // "q[0]" or "q0" → quell index
	sawGate := false

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
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "openqasm") || strings.HasPrefix(lower, "include") ||
			strings.HasPrefix(lower, "bit") || strings.HasPrefix(lower, "const ") ||
			strings.HasPrefix(lower, "input ") || strings.HasPrefix(lower, "output ") {
			continue
		}

		if m := qubitArrayRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			name := m[2]
			regs[name] = n
			for i := 0; i < n; i++ {
				key := fmt.Sprintf("%s[%d]", name, i)
				qubitIndex[key] = nextIdx
				nextIdx++
			}
			continue
		}
		if m := qubitOneRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			regs[name] = 1
			qubitIndex[name] = nextIdx
			nextIdx++
			continue
		}

		if m := measureRe.FindStringSubmatch(line); m != nil {
			qs, err := resolveQubits(m[1], qubitIndex)
			if err != nil {
				return "", fmt.Errorf("line %d: %w", lineNum, err)
			}
			if len(qs) == 0 {
				out.WriteString("MEASURE\n")
			} else {
				out.WriteString("MEASURE")
				for _, q := range qs {
					out.WriteString(" ")
					out.WriteString(strconv.Itoa(q))
				}
				out.WriteByte('\n')
			}
			sawGate = true
			continue
		}

		if m := ifRe.FindStringSubmatch(line); m != nil {
			cbit, _ := strconv.Atoi(m[2])
			eq, _ := strconv.Atoi(m[3])
			bodyLine := strings.TrimSpace(m[4])
			bodyLine = strings.TrimSuffix(bodyLine, ";")
			gm := gateCallRe.FindStringSubmatch(bodyLine)
			if gm == nil {
				out.WriteString("// unsupported: " + line + "\n")
				continue
			}
			g := strings.ToLower(gm[1])
			args := strings.TrimSpace(gm[2])
			targets := strings.TrimSpace(gm[3])
			qs, err := resolveQubits(targets, qubitIndex)
			if err != nil {
				out.WriteString("// unsupported: " + line + "\n")
				continue
			}
			quell, ok := mapGate(g, args, qs)
			if !ok {
				out.WriteString("// unsupported: " + line + "\n")
				continue
			}
			fmt.Fprintf(&out, "IF c[%d]==%d %s\n", cbit, eq, quell)
			sawGate = true
			continue
		}

		if m := gateCallRe.FindStringSubmatch(line); m != nil {
			g := strings.ToLower(m[1])
			args := strings.TrimSpace(m[2])
			targets := strings.TrimSpace(m[3])
			qs, err := resolveQubits(targets, qubitIndex)
			if err != nil {
				out.WriteString("// unsupported: " + line + "\n")
				continue
			}
			quell, ok := mapGate(g, args, qs)
			if !ok {
				out.WriteString("// unsupported: " + line + "\n")
				continue
			}
			out.WriteString(quell)
			out.WriteByte('\n')
			sawGate = true
			continue
		}

		out.WriteString("// skipped: " + line + "\n")
	}

	if !sawGate {
		return "", fmt.Errorf("qasmimport: no supported gate statements found")
	}
	if !strings.Contains(out.String(), "MEASURE") {
		out.WriteString("MEASURE\n")
	}
	return out.String(), nil
}

func resolveQubits(s string, index map[string]int) ([]int, error) {
	parts := splitArgs(s)
	var out []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i, ok := index[p]; ok {
			out = append(out, i)
			continue
		}
		// Bare register name → all qubits in that register (measure q → c)
		var regQs []int
		prefix := p + "["
		for key, i := range index {
			if strings.HasPrefix(key, prefix) {
				regQs = append(regQs, i)
			}
		}
		if len(regQs) > 0 {
			sort.Ints(regQs)
			out = append(out, regQs...)
			continue
		}
		m := qRefRe.FindStringSubmatch(p)
		if m == nil {
			return nil, fmt.Errorf("cannot resolve qubit %q", p)
		}
		var key string
		if m[1] != "" {
			key = fmt.Sprintf("%s[%s]", m[1], m[2])
		} else {
			key = m[3]
		}
		i, ok := index[key]
		if !ok {
			return nil, fmt.Errorf("unknown qubit %q", p)
		}
		out = append(out, i)
	}
	return out, nil
}

func splitArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func mapGate(g, args string, qs []int) (string, bool) {
	join := func(idxs []int) string {
		var b strings.Builder
		for i, q := range idxs {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(q))
		}
		return b.String()
	}
	switch g {
	case "h":
		if len(qs) != 1 {
			return "", false
		}
		return "H " + join(qs), true
	case "x":
		if len(qs) != 1 {
			return "", false
		}
		return "X " + join(qs), true
	case "y":
		if len(qs) != 1 {
			return "", false
		}
		return "Y " + join(qs), true
	case "z":
		if len(qs) != 1 {
			return "", false
		}
		return "Z " + join(qs), true
	case "s":
		if len(qs) != 1 {
			return "", false
		}
		return "S " + join(qs), true
	case "t":
		if len(qs) != 1 {
			return "", false
		}
		return "T " + join(qs), true
	case "sdg":
		if len(qs) != 1 {
			return "", false
		}
		return "SDG " + join(qs), true
	case "tdg":
		if len(qs) != 1 {
			return "", false
		}
		return "TDG " + join(qs), true
	case "sx":
		if len(qs) != 1 {
			return "", false
		}
		return "SX " + join(qs), true
	case "cx", "cnot":
		if len(qs) != 2 {
			return "", false
		}
		return "CNOT " + join(qs), true
	case "cz":
		if len(qs) != 2 {
			return "", false
		}
		return "CZ " + join(qs), true
	case "swap":
		if len(qs) != 2 {
			return "", false
		}
		return "SWAP " + join(qs), true
	case "ccx", "toffoli":
		if len(qs) != 3 {
			return "", false
		}
		return "CCX " + join(qs), true
	case "iswap":
		if len(qs) != 2 {
			return "", false
		}
		return "ISWAP " + join(qs), true
	case "cswap", "fredkin":
		if len(qs) != 3 {
			return "", false
		}
		return "CSWAP " + join(qs), true
	case "barrier":
		if len(qs) == 0 {
			return "BARRIER", true
		}
		return "BARRIER " + join(qs), true
	case "reset":
		if len(qs) != 1 {
			return "", false
		}
		return "RESET " + join(qs), true
	case "rx", "ry", "rz", "p":
		if len(qs) != 1 || args == "" {
			return "", false
		}
		angle := normalizeAngle(args)
		return strings.ToUpper(g) + " " + angle + " " + join(qs), true
	case "u", "u3":
		if len(qs) != 1 || args == "" {
			return "", false
		}
		parts := splitArgs(args)
		if len(parts) != 3 {
			return "", false
		}
		return "U " + normalizeAngle(parts[0]) + " " + normalizeAngle(parts[1]) + " " + normalizeAngle(parts[2]) + " " + join(qs), true
	case "crx", "cry", "crz":
		if len(qs) != 2 || args == "" {
			return "", false
		}
		return strings.ToUpper(g) + " " + normalizeAngle(args) + " " + join(qs), true
	default:
		return "", false
	}
}

func normalizeAngle(a string) string {
	a = strings.TrimSpace(a)
	a = strings.ReplaceAll(a, "pi", "PI")
	a = strings.ReplaceAll(a, "Pi", "PI")
	return a
}
