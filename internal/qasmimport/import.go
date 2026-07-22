// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qasmimport provides a thin OpenQASM 3 → Quell translator for the
// common gate subset (not full QASM 3 parity).
package qasmimport

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/magnobit/quell/decompose"
)

var (
	qubitArrayRe = regexp.MustCompile(`(?i)^qubit\s*\[\s*(\d+)\s*\]\s+(\w+)\s*;?\s*$`)
	qubitOneRe   = regexp.MustCompile(`(?i)^qubit\s+(\w+)\s*;?\s*$`)
	// OpenQASM 2.0: qreg q[2];
	qregArrayRe = regexp.MustCompile(`(?i)^qreg\s+(\w+)\s*\[\s*(\d+)\s*\]\s*;?\s*$`)
	qregOneRe   = regexp.MustCompile(`(?i)^qreg\s+(\w+)\s*;?\s*$`)
	// Classical registers: creg c[2]; / bit[2] c; / bit c;
	cregArrayRe = regexp.MustCompile(`(?i)^creg\s+(\w+)\s*\[\s*(\d+)\s*\]\s*;?\s*$`)
	cregOneRe   = regexp.MustCompile(`(?i)^creg\s+(\w+)\s*;?\s*$`)
	bitArrayRe  = regexp.MustCompile(`(?i)^bit\s*\[\s*(\d+)\s*\]\s+(\w+)\s*;?\s*$`)
	bitOneRe    = regexp.MustCompile(`(?i)^bit\s+(\w+)\s*;?\s*$`)
	gateCallRe  = regexp.MustCompile(`(?i)^([a-z][a-z0-9_]*)\s*(?:\(([^)]*)\))?\s+(.+?)\s*;?\s*$`)
	qRefRe      = regexp.MustCompile(`(?i)(\w+)\s*\[\s*(\d+)\s*\]|(\w+)`)
	measureRe       = regexp.MustCompile(`(?i)^measure\s+(.+?)\s*(?:->\s*.+)?\s*;?\s*$`)
	measureAssignRe = regexp.MustCompile(`(?i)^(\w+(?:\s*\[\s*\d+\s*\])?)\s*=\s*measure\s+(.+?)\s*;?\s*$`)
	// if (c[i] == v) body   |  if(c == v) body   |  optional { } / else
	ifHeadRe = regexp.MustCompile(`(?i)^if\s*\(\s*(.+?)\s*\)\s*(.*)$`)
	elseHeadRe = regexp.MustCompile(`(?i)^else\s*(.*)$`)
	gateDefRe  = regexp.MustCompile(`(?i)^gate\s+`)
	whileHeadRe = regexp.MustCompile(`(?i)^while\s*\(\s*(.+?)\s*\)\s*(.*)$`)
	// OpenQASM 3: for i in [0:4] { … }  (end exclusive in some dialects — we treat as inclusive lo..hi-1 if hi>lo, else lo..hi)
	forHeadRe = regexp.MustCompile(`(?i)^for\s+(?:uint\s+)?(\w+)\s+in\s*\[\s*(\d+)\s*:\s*(\d+)\s*\]\s*(.*)$`)
	switchHeadRe = regexp.MustCompile(`(?i)^switch\s*\(\s*(.+?)\s*\)\s*(.*)$`)
)

// Result holds Quell output plus soft warnings for unsupported / skipped lines.
type Result struct {
	Quell    string   `json:"quell"`
	Warnings []string `json:"warnings,omitempty"`
}

// ToQuell converts a subset of OpenQASM 2/3 into Quell source.
func ToQuell(src string) (string, error) {
	r, err := Convert(src)
	return r.Quell, err
}

// Convert is ToQuell with structured warnings (skipped/unsupported lines logged).
func Convert(src string) (Result, error) {
	src = stripQASMSourceComments(src)
	var out strings.Builder
	out.WriteString("// Converted from OpenQASM (thin import — common gates only)\n")
	var warnings []string
	warn := func(lineNum int, msg string) {
		warnings = append(warnings, fmt.Sprintf("line %d: %s", lineNum, msg))
	}

	regs := map[string]int{} // register name → size (1 for scalar qubit)
	cregs := map[string]int{} // classical register name → size
	nextIdx := 0
	qubitIndex := map[string]int{} // "q[0]" or "q0" → quell index
	sawGate := false

	registerQubits := func(name string, n int) {
		if n < 1 {
			n = 1
		}
		regs[name] = n
		for i := 0; i < n; i++ {
			key := fmt.Sprintf("%s[%d]", name, i)
			qubitIndex[key] = nextIdx
			nextIdx++
		}
		if n == 1 {
			qubitIndex[name] = qubitIndex[fmt.Sprintf("%s[0]", name)]
		}
	}
	registerCreg := func(name string, n int) {
		if n < 1 {
			n = 1
		}
		cregs[name] = n
	}

	// Pending else after a binary if (c[i]==0|1): invert condition for else body.
	type elseCtx struct {
		cbit, eq int
		armed    bool
	}
	var pendingElse elseCtx

	emitIFBodies := func(lineNum, cbit, eq int, bodies []string, forElse bool) {
		label := "if"
		if forElse {
			label = "else"
		}
		var quellGates []string
		for _, bodyLine := range bodies {
			bodyLine = strings.TrimSpace(strings.TrimSuffix(bodyLine, ";"))
			if bodyLine == "" {
				continue
			}
			gm := gateCallRe.FindStringSubmatch(bodyLine)
			if gm == nil {
				warn(lineNum, "unsupported "+label+" body (need a single gate per statement): "+bodyLine)
				out.WriteString("// unsupported "+label+": "+bodyLine+"\n")
				continue
			}
			g := strings.ToLower(gm[1])
			args := strings.TrimSpace(gm[2])
			targets := strings.TrimSpace(gm[3])
			qs, err := resolveQubits(targets, qubitIndex, regs)
			if err != nil {
				warn(lineNum, label+"-body: "+err.Error())
				out.WriteString("// unsupported "+label+": "+bodyLine+"\n")
				continue
			}
			quell, ok := mapGate(g, args, qs)
			if !ok {
				warn(lineNum, "unsupported gate in "+label+"-body: "+g)
				out.WriteString("// unsupported "+label+": "+bodyLine+"\n")
				continue
			}
			for _, part := range strings.Split(quell, "\n") {
				part = strings.TrimSpace(part)
				if part == "" || strings.HasPrefix(part, "//") {
					continue
				}
				quellGates = append(quellGates, part)
			}
		}
		if len(quellGates) == 0 {
			return
		}
		if len(quellGates) == 1 {
			fmt.Fprintf(&out, "IF c[%d]==%d %s\n", cbit, eq, quellGates[0])
		} else {
			fmt.Fprintf(&out, "IF c[%d]==%d {\n", cbit, eq)
			for _, g := range quellGates {
				fmt.Fprintf(&out, "  %s\n", g)
			}
			out.WriteString("}\n")
		}
		sawGate = true
	}

	parseCond := func(cond string) (cbit, eq int, ok bool, msg string) {
		cond = strings.TrimSpace(cond)
		// c[i] == v
		if m := regexp.MustCompile(`(?i)^(\w+)\s*\[\s*(\d+)\s*\]\s*==\s*(\d+)$`).FindStringSubmatch(cond); m != nil {
			cbit, _ = strconv.Atoi(m[2])
			eq, _ = strconv.Atoi(m[3])
			return cbit, eq, true, ""
		}
		// c == v  (OpenQASM 2 register equality)
		if m := regexp.MustCompile(`(?i)^(\w+)\s*==\s*(\d+)$`).FindStringSubmatch(cond); m != nil {
			name := m[1]
			eq, _ = strconv.Atoi(m[2])
			size := cregs[name]
			if size == 0 {
				size = 1 // assume single-bit if undeclared
			}
			if size == 1 {
				if eq != 0 && eq != 1 {
					return 0, 0, false, fmt.Sprintf("creg %s has 1 bit — condition value must be 0 or 1", name)
				}
				return 0, eq, true, ""
			}
			// Multi-bit: only map power-of-two / small ints to a single bit when clear.
			if eq == 0 {
				return 0, 0, true, ""
			}
			// Find single set bit
			bit := -1
			v := eq
			for i := 0; i < size && v > 0; i++ {
				if v&1 == 1 {
					if bit >= 0 {
						return 0, 0, false, fmt.Sprintf("if(%s==%d) needs multiple bits — Quell IF is single-bit only; use OpenQASM 3 bit tests or decompose", name, eq)
					}
					bit = i
				}
				v >>= 1
			}
			if bit < 0 {
				return 0, 0, false, fmt.Sprintf("unsupported condition %s==%d", name, eq)
			}
			return bit, 1, true, ""
		}
		return 0, 0, false, "unsupported if condition (use c[i]==v or creg==int)"
	}

	collectBraceBody := func(rest string, lines []string, startIdx int) (bodies []string, nextIdx int, errMsg string) {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return nil, startIdx, "empty if/else body"
		}
		if !strings.HasPrefix(rest, "{") {
			rest = strings.TrimSuffix(rest, ";")
			return []string{rest}, startIdx, ""
		}
		// Find matching closing brace (not the last "}" — supports "} else {")
		depth := 0
		end := -1
		for i, r := range rest {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			// Brace opens on this line; continue across following lines
			inner := strings.TrimSpace(strings.TrimPrefix(rest, "{"))
			var buf []string
			if inner != "" {
				buf = append(buf, strings.TrimSuffix(inner, ";"))
			}
			for j := startIdx + 1; j < len(lines); j++ {
				raw := lines[j]
				if ci := strings.Index(raw, "//"); ci >= 0 {
					raw = raw[:ci]
				}
				s := strings.TrimSpace(raw)
				if s == "" {
					continue
				}
				if strings.HasPrefix(s, "}") {
					rem := strings.TrimSpace(strings.TrimPrefix(s, "}"))
					if rem != "" {
						lines[j] = rem
						return buf, j - 1, ""
					}
					return buf, j, ""
				}
				buf = append(buf, strings.TrimSuffix(s, ";"))
			}
			return nil, startIdx, "unclosed { in if/else"
		}
		inner := strings.TrimSpace(rest[1:end])
		rem := strings.TrimSpace(rest[end+1:])
		var buf []string
		if inner != "" {
			for _, p := range splitStatements(inner) {
				buf = append(buf, p)
			}
		}
		if rem != "" {
			// Same-line "} else …" — stash remainder for outer loop
			lines[startIdx] = rem
			return buf, startIdx - 1, ""
		}
		return buf, startIdx, ""
	}

	allLines := strings.Split(src, "\n")
	for i := 0; i < len(allLines); i++ {
		lineNum := i + 1
		raw := allLines[i]
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "openqasm") || strings.HasPrefix(lower, "include") ||
			strings.HasPrefix(lower, "const ") || strings.HasPrefix(lower, "input ") ||
			strings.HasPrefix(lower, "output ") {
			continue
		}

		if m := qubitArrayRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			registerQubits(m[2], n)
			continue
		}
		if m := qubitOneRe.FindStringSubmatch(line); m != nil {
			registerQubits(m[1], 1)
			continue
		}
		if m := qregArrayRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			registerQubits(m[1], n)
			continue
		}
		if m := qregOneRe.FindStringSubmatch(line); m != nil {
			registerQubits(m[1], 1)
			continue
		}
		if m := cregArrayRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			registerCreg(m[1], n)
			continue
		}
		if m := cregOneRe.FindStringSubmatch(line); m != nil {
			registerCreg(m[1], 1)
			continue
		}
		if m := bitArrayRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			registerCreg(m[2], n)
			continue
		}
		if m := bitOneRe.FindStringSubmatch(line); m != nil {
			registerCreg(m[1], 1)
			continue
		}

		if gateDefRe.MatchString(line) {
			warn(lineNum, "custom gate definitions are not imported yet — expand gates or use standard library ops")
			out.WriteString("// skipped gate def: " + line + "\n")
			pendingElse.armed = false
			continue
		}

		emitMeasure := func(qubitExpr string) {
			qs, err := resolveQubits(qubitExpr, qubitIndex, regs)
			if err != nil {
				warn(lineNum, err.Error()+" — measure skipped")
				out.WriteString("// skipped measure: " + line + "\n")
				return
			}
			if len(qs) == 0 {
				out.WriteString("MEASURE\n")
			} else if regSize, ok := regs[strings.TrimSpace(qubitExpr)]; ok && regSize > 0 && len(qs) == regSize {
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
		}

		if m := measureAssignRe.FindStringSubmatch(line); m != nil {
			emitMeasure(m[2])
			pendingElse.armed = false
			continue
		}
		if m := measureRe.FindStringSubmatch(line); m != nil {
			emitMeasure(m[1])
			pendingElse.armed = false
			continue
		}

		// else …  (follows a binary if)
		if m := elseHeadRe.FindStringSubmatch(line); m != nil {
			if !pendingElse.armed {
				warn(lineNum, "else without a preceding binary if (c[i]==0|1) — skipped")
				out.WriteString("// skipped else: " + line + "\n")
				continue
			}
			elseEq := 0
			if pendingElse.eq == 0 {
				elseEq = 1
			}
			bodies, next, errMsg := collectBraceBody(m[1], allLines, i)
			if errMsg != "" {
				warn(lineNum, errMsg)
				out.WriteString("// unsupported else: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			i = next
			emitIFBodies(lineNum, pendingElse.cbit, elseEq, bodies, true)
			pendingElse.armed = false
			continue
		}

		// while (…) { … } → Quell WHILE … MAX n (default 32)
		if m := whileHeadRe.FindStringSubmatch(line); m != nil {
			cbit, eq, ok, msg := parseCond(m[1])
			if !ok {
				warn(lineNum, "while: "+msg)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			bodies, next, errMsg := collectBraceBody(strings.TrimSpace(m[2]), allLines, i)
			if errMsg != "" {
				warn(lineNum, "while: "+errMsg)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			i = next
			quellBody := bodyLinesToQuell(lineNum, bodies, qubitIndex, regs, warn)
			if len(quellBody) == 0 {
				warn(lineNum, "while body empty or unsupported")
				pendingElse.armed = false
				continue
			}
			const maxDefault = 32
			warn(lineNum, fmt.Sprintf("OpenQASM while has no MAX — importing as WHILE … MAX %d (Quell requires a bound)", maxDefault))
			fmt.Fprintf(&out, "WHILE c[%d]==%d MAX %d {\n", cbit, eq, maxDefault)
			for _, g := range quellBody {
				fmt.Fprintf(&out, "  %s\n", g)
			}
			out.WriteString("}\n")
			sawGate = true
			pendingElse.armed = false
			continue
		}

		// for i in [lo:hi] { … } — unroll like Quell FOR
		if m := forHeadRe.FindStringSubmatch(line); m != nil {
			varName := m[1]
			lo, _ := strconv.Atoi(m[2])
			hi, _ := strconv.Atoi(m[3])
			// OpenQASM 3 ranges are typically end-exclusive [lo:hi) → Quell inclusive lo..(hi-1)
			endInclusive := hi - 1
			if hi <= lo {
				endInclusive = hi
			}
			if endInclusive < lo {
				warn(lineNum, fmt.Sprintf("for range [%d:%d] empty", lo, hi))
				pendingElse.armed = false
				continue
			}
			if endInclusive-lo > 64 {
				warn(lineNum, "for range too large (max 65) — skipped")
				pendingElse.armed = false
				continue
			}
			bodies, next, errMsg := collectBraceBody(strings.TrimSpace(m[4]), allLines, i)
			if errMsg != "" {
				warn(lineNum, "for: "+errMsg)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			i = next
			for v := lo; v <= endInclusive; v++ {
				expanded := expandLoopVarLines(bodies, varName, v)
				quellBody := bodyLinesToQuell(lineNum, expanded, qubitIndex, regs, warn)
				for _, g := range quellBody {
					out.WriteString(g + "\n")
					sawGate = true
				}
			}
			pendingElse.armed = false
			continue
		}

		// switch (c[i]) { 0: … default: … } — Quell SWITCH
		if m := switchHeadRe.FindStringSubmatch(line); m != nil {
			disc := strings.TrimSpace(m[1])
			cbit := 0
			if dm := regexp.MustCompile(`(?i)^(\w+)\s*\[\s*(\d+)\s*\]$`).FindStringSubmatch(disc); dm != nil {
				cbit, _ = strconv.Atoi(dm[2])
			} else if regexp.MustCompile(`(?i)^\w+$`).MatchString(disc) {
				cbit = 0 // whole register → c[0] approximation with warning
				warn(lineNum, "switch on whole register approximated as c[0]")
			} else {
				warn(lineNum, "unsupported switch discriminant: "+disc)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			bodies, next, errMsg := collectBraceBody(strings.TrimSpace(m[2]), allLines, i)
			if errMsg != "" {
				warn(lineNum, "switch: "+errMsg)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			i = next
			arms := parseSwitchArms(bodies)
			if len(arms) == 0 {
				warn(lineNum, "switch has no CASE/default arms")
				pendingElse.armed = false
				continue
			}
			fmt.Fprintf(&out, "SWITCH c[%d] {\n", cbit)
			for _, arm := range arms {
				quellBody := bodyLinesToQuell(lineNum, arm.body, qubitIndex, regs, warn)
				if arm.def {
					out.WriteString("  DEFAULT:\n")
				} else {
					fmt.Fprintf(&out, "  CASE %d:\n", arm.val)
				}
				for _, g := range quellBody {
					fmt.Fprintf(&out, "    %s\n", g)
				}
			}
			out.WriteString("}\n")
			sawGate = true
			pendingElse.armed = false
			continue
		}

		// if (…) …
		if m := ifHeadRe.FindStringSubmatch(line); m != nil {
			cbit, eq, ok, msg := parseCond(m[1])
			if !ok {
				warn(lineNum, msg)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			rest := strings.TrimSpace(m[2])
			// Inline else: if (...) x q[0]; else y q[0];
			inlineElse := ""
			if ei := regexp.MustCompile(`(?i)\belse\b`).FindStringIndex(rest); ei != nil && !strings.HasPrefix(strings.TrimSpace(rest), "{") {
				// Only split else outside braces — simple same-line form
				parts := regexp.MustCompile(`(?i)\s+else\s+`).Split(rest, 2)
				if len(parts) == 2 {
					rest = strings.TrimSpace(parts[0])
					inlineElse = strings.TrimSpace(parts[1])
				}
			}
			bodies, next, errMsg := collectBraceBody(rest, allLines, i)
			if errMsg != "" {
				warn(lineNum, errMsg)
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			i = next
			emitIFBodies(lineNum, cbit, eq, bodies, false)
			if inlineElse != "" {
				elseEq := 0
				if eq == 0 {
					elseEq = 1
				}
				if eq != 0 && eq != 1 {
					warn(lineNum, "else after if with non-binary condition skipped")
				} else {
					eb, _, eMsg := collectBraceBody(inlineElse, allLines, i)
					if eMsg != "" {
						warn(lineNum, "else: "+eMsg)
					} else {
						emitIFBodies(lineNum, cbit, elseEq, eb, true)
					}
				}
				pendingElse.armed = false
			} else if eq == 0 || eq == 1 {
				pendingElse = elseCtx{cbit: cbit, eq: eq, armed: true}
			} else {
				pendingElse.armed = false
				warn(lineNum, "else not supported for non-binary if conditions (need ==0 or ==1)")
			}
			continue
		}

		if m := gateCallRe.FindStringSubmatch(line); m != nil {
			g := strings.ToLower(m[1])
			args := strings.TrimSpace(m[2])
			targets := strings.TrimSpace(m[3])
			qs, err := resolveQubits(targets, qubitIndex, regs)
			if err != nil {
				warn(lineNum, fmt.Sprintf("could not resolve qubits for %s: %v", g, err))
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			quell, ok := mapGate(g, args, qs)
			if !ok {
				warn(lineNum, "unsupported OpenQASM op \""+g+"\" — not in Quell subset yet")
				out.WriteString("// unsupported: " + line + "\n")
				pendingElse.armed = false
				continue
			}
			out.WriteString(quell)
			out.WriteByte('\n')
			sawGate = true
			pendingElse.armed = false
			continue
		}

		warn(lineNum, "skipped unrecognized statement")
		out.WriteString("// skipped: " + line + "\n")
		pendingElse.armed = false
	}

	if !sawGate {
		return Result{Warnings: warnings}, fmt.Errorf("qasmimport: no supported gate statements found")
	}
	if !strings.Contains(out.String(), "MEASURE") {
		out.WriteString("MEASURE\n")
	}
	return Result{Quell: out.String(), Warnings: warnings}, nil
}

// splitStatements splits "a; b; c" into statements (brace-aware not needed for gate lists).
func splitStatements(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ";") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func resolveQubits(s string, index map[string]int, regs map[string]int) ([]int, error) {
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
		// Bare register name → all qubits in that register (measure q -> c)
		if n, ok := regs[p]; ok && n > 0 {
			var regQs []int
			for i := 0; i < n; i++ {
				key := fmt.Sprintf("%s[%d]", p, i)
				if qi, ok := index[key]; ok {
					regQs = append(regQs, qi)
				}
			}
			if len(regQs) > 0 {
				out = append(out, regQs...)
				continue
			}
		}
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
	case "id", "i":
		return "BARRIER " + join(qs), true // no-op placeholder so converters don't fail
	case "u1":
		if len(qs) != 1 || args == "" {
			return "", false
		}
		return "P " + normalizeAngle(args) + " " + join(qs), true
	case "u2":
		if len(qs) != 1 || args == "" {
			return "", false
		}
		parts := splitArgs(args)
		if len(parts) != 2 {
			return "", false
		}
		return "U PI/2 " + normalizeAngle(parts[0]) + " " + normalizeAngle(parts[1]) + " " + join(qs), true
	case "cp", "cu1":
		if len(qs) != 2 || args == "" {
			return "", false
		}
		return "CRZ " + normalizeAngle(args) + " " + join(qs), true
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
	case "sxdg":
		if len(qs) != 1 {
			return "", false
		}
		return decompose.JoinQuell(decompose.SXDG(qs[0])), true
	case "rxx":
		if len(qs) != 2 || args == "" {
			return "", false
		}
		return decompose.JoinQuell(decompose.RXX(args, qs[0], qs[1])), true
	case "ryy":
		if len(qs) != 2 || args == "" {
			return "", false
		}
		return decompose.JoinQuell(decompose.RYY(args, qs[0], qs[1])), true
	case "rzz":
		if len(qs) != 2 || args == "" {
			return "", false
		}
		return decompose.JoinQuell(decompose.RZZ(args, qs[0], qs[1])), true
	case "rzx":
		if len(qs) != 2 || args == "" {
			return "", false
		}
		return decompose.JoinQuell(decompose.RZX(args, qs[0], qs[1])), true
	case "ecr":
		if len(qs) != 2 {
			return "", false
		}
		return decompose.JoinQuell(decompose.ECR(qs[0], qs[1])), true
	case "cy":
		if len(qs) != 2 {
			return "", false
		}
		return decompose.JoinQuell(decompose.CY(qs[0], qs[1])), true
	case "ch":
		if len(qs) != 2 {
			return "", false
		}
		return decompose.JoinQuell(decompose.CH(qs[0], qs[1])), true
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

// stripQASMSourceComments removes // line comments and /* */ blocks so gate
// examples inside comments are not imported as live instructions.
func stripQASMSourceComments(src string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			if i+1 < len(src) {
				i += 2
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func bodyLinesToQuell(lineNum int, bodies []string, qubitIndex map[string]int, regs map[string]int, warn func(int, string)) []string {
	var out []string
	for _, bodyLine := range bodies {
		bodyLine = strings.TrimSpace(strings.TrimSuffix(bodyLine, ";"))
		if bodyLine == "" {
			continue
		}
		// Nested if/while not expanded here — warn.
		lower := strings.ToLower(bodyLine)
		if strings.HasPrefix(lower, "if ") || strings.HasPrefix(lower, "if(") ||
			strings.HasPrefix(lower, "while ") || strings.HasPrefix(lower, "while(") ||
			strings.HasPrefix(lower, "for ") || strings.HasPrefix(lower, "switch ") || strings.HasPrefix(lower, "switch(") {
			warn(lineNum, "nested control flow in body not imported: "+bodyLine)
			continue
		}
		gm := gateCallRe.FindStringSubmatch(bodyLine)
		if gm == nil {
			if mm := measureRe.FindStringSubmatch(bodyLine); mm != nil {
				qs, err := resolveQubits(mm[1], qubitIndex, regs)
				if err != nil {
					warn(lineNum, "measure in body: "+err.Error())
					continue
				}
				if len(qs) == 0 {
					out = append(out, "MEASURE")
				} else {
					for _, q := range qs {
						out = append(out, fmt.Sprintf("MEASURE %d", q))
					}
				}
				continue
			}
			warn(lineNum, "unsupported body stmt: "+bodyLine)
			continue
		}
		g := strings.ToLower(gm[1])
		args := strings.TrimSpace(gm[2])
		targets := strings.TrimSpace(gm[3])
		qs, err := resolveQubits(targets, qubitIndex, regs)
		if err != nil {
			warn(lineNum, err.Error())
			continue
		}
		quell, ok := mapGate(g, args, qs)
		if !ok {
			warn(lineNum, "unsupported gate in body: "+g)
			continue
		}
		for _, part := range strings.Split(quell, "\n") {
			part = strings.TrimSpace(part)
			if part == "" || strings.HasPrefix(part, "//") {
				continue
			}
			out = append(out, part)
		}
	}
	return out
}

func expandLoopVarLines(lines []string, varName string, val int) []string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(varName) + `\b`)
	vs := strconv.Itoa(val)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, re.ReplaceAllString(l, vs))
	}
	return out
}

type switchArm struct {
	val  int
	def  bool
	body []string
}

func parseSwitchArms(lines []string) []switchArm {
	var arms []switchArm
	var cur *switchArm
	caseRe := regexp.MustCompile(`(?i)^(?:case\s+)?(\d+)\s*:\s*(.*)$`)
	defRe := regexp.MustCompile(`(?i)^default\s*:\s*(.*)$`)
	flush := func() {
		if cur != nil {
			arms = append(arms, *cur)
			cur = nil
		}
	}
	for _, raw := range lines {
		s := strings.TrimSpace(strings.TrimSuffix(raw, ";"))
		if s == "" {
			continue
		}
		if m := defRe.FindStringSubmatch(s); m != nil {
			flush()
			cur = &switchArm{def: true}
			if rest := strings.TrimSpace(m[1]); rest != "" {
				cur.body = append(cur.body, rest)
			}
			continue
		}
		if m := caseRe.FindStringSubmatch(s); m != nil {
			flush()
			v, _ := strconv.Atoi(m[1])
			cur = &switchArm{val: v}
			if rest := strings.TrimSpace(m[2]); rest != "" {
				cur.body = append(cur.body, rest)
			}
			continue
		}
		if cur == nil {
			cur = &switchArm{def: true}
		}
		cur.body = append(cur.body, s)
	}
	flush()
	return arms
}
