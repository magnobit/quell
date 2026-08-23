// Copyright 2026 Magnobit, Inc. All rights reserved.

package migrate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/magnobit/quell/decompose"
)

// Q# qubit args are often qs[0] / qubits[1], not bare identifiers — \w+ alone failed
// on the Migration sample (H(qs[0]); CNOT(qs[0], qs[1]); M(qs[0])).
const qsQubitTok = `([\w.]+(?:\s*\[\s*\d+\s*\])?)`

var (
	qsH    = regexp.MustCompile(`(?i)\bH\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsX    = regexp.MustCompile(`(?i)\bX\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsY    = regexp.MustCompile(`(?i)\bY\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsZ    = regexp.MustCompile(`(?i)\bZ\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsS    = regexp.MustCompile(`(?i)\bS\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsT    = regexp.MustCompile(`(?i)\bT\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsCNOT = regexp.MustCompile(`(?i)\b(?:CNOT|CX)\s*\(\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
	qsCZ   = regexp.MustCompile(`(?i)\bCZ\s*\(\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
	qsSWAP = regexp.MustCompile(`(?i)\bSWAP\s*\(\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
	qsRX   = regexp.MustCompile(`(?i)\bRx\s*\(\s*([^,]+)\s*,\s*` + qsQubitTok + `\s*\)`)
	qsRY   = regexp.MustCompile(`(?i)\bRy\s*\(\s*([^,]+)\s*,\s*` + qsQubitTok + `\s*\)`)
	qsRZ   = regexp.MustCompile(`(?i)\bRz\s*\(\s*([^,]+)\s*,\s*` + qsQubitTok + `\s*\)`)
	qsMeas = regexp.MustCompile(`(?i)\b(?:M|Measure)\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsQRef = regexp.MustCompile(`(?i)(?:qubits?|qs?)\s*\[\s*(\d+)\s*\]|q(\d+)`)
	// Heuristic: Q# gate-like calls we don't map yet (for warnings).
	qsUnknownGate = regexp.MustCompile(`(?i)\b([A-Z][A-Za-z0-9]*)\s*\(`)
	qsOpDecl      = regexp.MustCompile(`(?i)\b(?:operation|function)\s+([A-Za-z_][\w]*)`)
	qsAdjointS    = regexp.MustCompile(`(?i)\bAdjoint\s+S\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsAdjointT    = regexp.MustCompile(`(?i)\bAdjoint\s+T\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsCCNOT       = regexp.MustCompile(`(?i)\b(?:CCNOT|CCX)\s*\(\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
	qsReset       = regexp.MustCompile(`(?i)\bReset\s*\(\s*` + qsQubitTok + `\s*\)`)
	qsI           = regexp.MustCompile(`(?i)\bI\s*\(\s*` + qsQubitTok + `\s*\)`)
	// let r = M(q); … if (r == One)
	qsLetMeas = regexp.MustCompile(`(?i)\blet\s+(\w+)\s*=\s*M\s*\(\s*` + qsQubitTok + `\s*\)\s*;`)
	qsIfOnLet = regexp.MustCompile(`(?is)if\s*\(\s*(\w+)\s*==\s*(One|Zero|Result\.One|Result\.Zero|1|0)\s*\)\s*\{`)

	qsRxx = regexp.MustCompile(`(?i)\bRxx\s*\(\s*([^,]+)\s*,\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
	qsRzz = regexp.MustCompile(`(?i)\bRzz\s*\(\s*([^,]+)\s*,\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
	qsRyy = regexp.MustCompile(`(?i)\bRyy\s*\(\s*([^,]+)\s*,\s*` + qsQubitTok + `\s*,\s*` + qsQubitTok + `\s*\)`)
)

type qsHit struct {
	pos  int
	line string
}

// convertQSharpToQuell is a thin subset converter for common Q# gate calls.
// Returns Quell, soft warnings for unrecognized gate-like calls, and error if nothing mapped.
func convertQSharpToQuell(code string) (string, []string, error) {
	qubitNames := map[string]int{}
	next := 0
	resolve := func(tok string) int {
		tok = strings.TrimSpace(tok)
		if m := qsQRef.FindStringSubmatch(tok); m != nil {
			idx := m[1]
			if idx == "" {
				idx = m[2]
			}
			var n int
			fmt.Sscanf(idx, "%d", &n)
			return n
		}
		if id, ok := qubitNames[tok]; ok {
			return id
		}
		id := next
		qubitNames[tok] = id
		next++
		return id
	}

	matchedSpans := map[string]bool{}
	var hits []qsHit
	var warnings []string
	addAll := func(re *regexp.Regexp, emit func([]string) string) {
		for _, m := range re.FindAllStringSubmatchIndex(code, -1) {
			full := code[m[0]:m[1]]
			sub := re.FindStringSubmatch(full)
			if sub == nil {
				continue
			}
			matchedSpans[fmt.Sprintf("%d:%d", m[0], m[1])] = true
			hits = append(hits, qsHit{pos: m[0], line: emit(sub)})
		}
	}
	addAll(qsCNOT, func(m []string) string {
		return fmt.Sprintf("CNOT %d %d", resolve(m[1]), resolve(m[2]))
	})
	addAll(qsCZ, func(m []string) string {
		return fmt.Sprintf("CZ %d %d", resolve(m[1]), resolve(m[2]))
	})
	addAll(qsSWAP, func(m []string) string {
		return fmt.Sprintf("SWAP %d %d", resolve(m[1]), resolve(m[2]))
	})
	addAll(qsH, func(m []string) string { return fmt.Sprintf("H %d", resolve(m[1])) })
	addAll(qsX, func(m []string) string { return fmt.Sprintf("X %d", resolve(m[1])) })
	addAll(qsY, func(m []string) string { return fmt.Sprintf("Y %d", resolve(m[1])) })
	addAll(qsZ, func(m []string) string { return fmt.Sprintf("Z %d", resolve(m[1])) })
	addAll(qsS, func(m []string) string { return fmt.Sprintf("S %d", resolve(m[1])) })
	addAll(qsT, func(m []string) string { return fmt.Sprintf("T %d", resolve(m[1])) })
	addAll(qsRX, func(m []string) string {
		return fmt.Sprintf("RX %s %d", strings.TrimSpace(m[1]), resolve(m[2]))
	})
	addAll(qsRY, func(m []string) string {
		return fmt.Sprintf("RY %s %d", strings.TrimSpace(m[1]), resolve(m[2]))
	})
	addAll(qsRZ, func(m []string) string {
		return fmt.Sprintf("RZ %s %d", strings.TrimSpace(m[1]), resolve(m[2]))
	})
	addAll(qsMeas, func(m []string) string { return fmt.Sprintf("MEASURE %d", resolve(m[1])) })
	addAll(qsAdjointS, func(m []string) string { return fmt.Sprintf("SDG %d", resolve(m[1])) })
	addAll(qsAdjointT, func(m []string) string { return fmt.Sprintf("TDG %d", resolve(m[1])) })
	addAll(qsCCNOT, func(m []string) string {
		return fmt.Sprintf("CCX %d %d %d", resolve(m[1]), resolve(m[2]), resolve(m[3]))
	})
	addAll(qsReset, func(m []string) string { return fmt.Sprintf("RESET %d", resolve(m[1])) })
	addAll(qsI, func(m []string) string { return fmt.Sprintf("// I %d (identity)", resolve(m[1])) })

	// Rxx / Ryy / Rzz → decomposed Quell
	for _, m := range qsRxx.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := qsRxx.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		matchedSpans[fmt.Sprintf("%d:%d", m[0], m[1])] = true
		r := decompose.RXX(sub[1], resolve(sub[2]), resolve(sub[3]))
		for i, line := range decompose.FormatAnnotate(r) {
			hits = append(hits, qsHit{pos: m[0] + i, line: line})
		}
	}
	for _, m := range qsRyy.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := qsRyy.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		matchedSpans[fmt.Sprintf("%d:%d", m[0], m[1])] = true
		r := decompose.RYY(sub[1], resolve(sub[2]), resolve(sub[3]))
		for i, line := range decompose.FormatAnnotate(r) {
			hits = append(hits, qsHit{pos: m[0] + i, line: line})
		}
	}
	for _, m := range qsRzz.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := qsRzz.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		matchedSpans[fmt.Sprintf("%d:%d", m[0], m[1])] = true
		r := decompose.RZZ(sub[1], resolve(sub[2]), resolve(sub[3]))
		for i, line := range decompose.FormatAnnotate(r) {
			hits = append(hits, qsHit{pos: m[0] + i, line: line})
		}
	}

	// Mid-circuit feedforward: if (M(q) == One) { … } else { … }
	type span struct{ a, b int }
	var ifSpans []span

	parseQSGateStmt := func(stmt string) (string, bool) {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			return "", false
		}
		switch {
		case qsH.MatchString(stmt):
			sub := qsH.FindStringSubmatch(stmt)
			return fmt.Sprintf("H %d", resolve(sub[1])), true
		case qsX.MatchString(stmt):
			sub := qsX.FindStringSubmatch(stmt)
			return fmt.Sprintf("X %d", resolve(sub[1])), true
		case qsY.MatchString(stmt):
			sub := qsY.FindStringSubmatch(stmt)
			return fmt.Sprintf("Y %d", resolve(sub[1])), true
		case qsZ.MatchString(stmt):
			sub := qsZ.FindStringSubmatch(stmt)
			return fmt.Sprintf("Z %d", resolve(sub[1])), true
		case qsS.MatchString(stmt):
			sub := qsS.FindStringSubmatch(stmt)
			return fmt.Sprintf("S %d", resolve(sub[1])), true
		case qsT.MatchString(stmt):
			sub := qsT.FindStringSubmatch(stmt)
			return fmt.Sprintf("T %d", resolve(sub[1])), true
		case qsAdjointS.MatchString(stmt):
			sub := qsAdjointS.FindStringSubmatch(stmt)
			return fmt.Sprintf("SDG %d", resolve(sub[1])), true
		case qsAdjointT.MatchString(stmt):
			sub := qsAdjointT.FindStringSubmatch(stmt)
			return fmt.Sprintf("TDG %d", resolve(sub[1])), true
		case qsCNOT.MatchString(stmt):
			sub := qsCNOT.FindStringSubmatch(stmt)
			return fmt.Sprintf("CNOT %d %d", resolve(sub[1]), resolve(sub[2])), true
		case qsCZ.MatchString(stmt):
			sub := qsCZ.FindStringSubmatch(stmt)
			return fmt.Sprintf("CZ %d %d", resolve(sub[1]), resolve(sub[2])), true
		case qsSWAP.MatchString(stmt):
			sub := qsSWAP.FindStringSubmatch(stmt)
			return fmt.Sprintf("SWAP %d %d", resolve(sub[1]), resolve(sub[2])), true
		case qsCCNOT.MatchString(stmt):
			sub := qsCCNOT.FindStringSubmatch(stmt)
			return fmt.Sprintf("CCX %d %d %d", resolve(sub[1]), resolve(sub[2]), resolve(sub[3])), true
		case qsRX.MatchString(stmt):
			sub := qsRX.FindStringSubmatch(stmt)
			return fmt.Sprintf("RX %s %d", strings.TrimSpace(sub[1]), resolve(sub[2])), true
		case qsRY.MatchString(stmt):
			sub := qsRY.FindStringSubmatch(stmt)
			return fmt.Sprintf("RY %s %d", strings.TrimSpace(sub[1]), resolve(sub[2])), true
		case qsRZ.MatchString(stmt):
			sub := qsRZ.FindStringSubmatch(stmt)
			return fmt.Sprintf("RZ %s %d", strings.TrimSpace(sub[1]), resolve(sub[2])), true
		default:
			return "", false
		}
	}
	bodyToQuell := func(body string) []string {
		var out []string
		for _, stmt := range strings.Split(body, ";") {
			line, ok := parseQSGateStmt(stmt)
			if !ok {
				if strings.TrimSpace(stmt) != "" {
					warnings = append(warnings, fmt.Sprintf("unsupported Q# gate inside if/else: %s", strings.TrimSpace(stmt)))
				}
				continue
			}
			out = append(out, line)
		}
		return out
	}
	emitBlockIF := func(pos, cbit, eq int, thenBody, elseBody string) {
		hits = append(hits, qsHit{pos: pos - 1, line: fmt.Sprintf("MEASURE %d", cbit)})
		thenLines := bodyToQuell(thenBody)
		elseLines := bodyToQuell(elseBody)
		if len(thenLines) == 0 && len(elseLines) == 0 {
			return
		}
		// Prefer flat IF lines when bodies are single gates — portable to OpenQASM 2 / Qiskit / Cirq.
		if len(thenLines) <= 1 && len(elseLines) <= 1 {
			if len(thenLines) == 1 {
				hits = append(hits, qsHit{pos: pos, line: fmt.Sprintf("IF c[%d]==%d %s", cbit, eq, thenLines[0])})
			}
			if len(elseLines) == 1 {
				elseEq := 0
				if eq == 0 {
					elseEq = 1
				}
				hits = append(hits, qsHit{pos: pos + 1, line: fmt.Sprintf("IF c[%d]==%d %s", cbit, elseEq, elseLines[0])})
			}
			return
		}
		hits = append(hits, qsHit{pos: pos, line: fmt.Sprintf("IF c[%d]==%d {", cbit, eq)})
		for _, l := range thenLines {
			hits = append(hits, qsHit{pos: pos, line: "  " + l})
		}
		hits = append(hits, qsHit{pos: pos, line: "}"})
		if len(elseLines) > 0 {
			hits = append(hits, qsHit{pos: pos + 1, line: "ELSE {"})
			for _, l := range elseLines {
				hits = append(hits, qsHit{pos: pos + 1, line: "  " + l})
			}
			hits = append(hits, qsHit{pos: pos + 1, line: "}"})
		}
	}

	// Brace-aware scan for if (M(q) == One/Zero) { … } else { … }
	qsIfHead := regexp.MustCompile(`(?is)if\s*\(\s*M\s*\(\s*` + qsQubitTok + `\s*\)\s*==\s*(One|Zero|Result\.One|Result\.Zero|1|0)\s*\)\s*\{`)
	for _, loc := range qsIfHead.FindAllStringSubmatchIndex(code, -1) {
		sub := qsIfHead.FindStringSubmatch(code[loc[0]:loc[1]])
		if sub == nil {
			continue
		}
		thenBody, endThen, ok := readBalancedBrace(code, loc[1]-1)
		if !ok {
			continue
		}
		elseBody := ""
		endAll := endThen
		rest := strings.TrimSpace(code[endThen:])
		if strings.HasPrefix(strings.ToLower(rest), "else") {
			ei := strings.Index(strings.ToLower(code[endThen:]), "else")
			afterElse := endThen + ei + 4
			for afterElse < len(code) && (code[afterElse] == ' ' || code[afterElse] == '\t' || code[afterElse] == '\n' || code[afterElse] == '\r') {
				afterElse++
			}
			if afterElse < len(code) && code[afterElse] == '{' {
				eb, endElse, ok2 := readBalancedBrace(code, afterElse)
				if ok2 {
					elseBody = eb
					endAll = endElse
				}
			}
		}
		ifSpans = append(ifSpans, span{loc[0], endAll})
		matchedSpans[fmt.Sprintf("%d:%d", loc[0], endAll)] = true
		cbit := resolve(sub[1])
		eq := 1
		val := strings.ToLower(sub[2])
		if strings.Contains(val, "zero") || val == "0" {
			eq = 0
		}
		emitBlockIF(loc[0], cbit, eq, thenBody, elseBody)
	}

	// let r = M(q); if (r == One) { … }
	letMeas := map[string]int{} // result name → qubit
	for _, m := range qsLetMeas.FindAllStringSubmatchIndex(code, -1) {
		full := code[m[0]:m[1]]
		sub := qsLetMeas.FindStringSubmatch(full)
		if sub == nil {
			continue
		}
		matchedSpans[fmt.Sprintf("%d:%d", m[0], m[1])] = true
		cbit := resolve(sub[2])
		letMeas[sub[1]] = cbit
		hits = append(hits, qsHit{pos: m[0], line: fmt.Sprintf("MEASURE %d", cbit)})
	}
	for _, loc := range qsIfOnLet.FindAllStringSubmatchIndex(code, -1) {
		sub := qsIfOnLet.FindStringSubmatch(code[loc[0]:loc[1]])
		if sub == nil {
			continue
		}
		cbit, ok := letMeas[sub[1]]
		if !ok {
			continue
		}
		thenBody, endThen, ok := readBalancedBrace(code, loc[1]-1)
		if !ok {
			continue
		}
		elseBody := ""
		endAll := endThen
		rest := strings.TrimSpace(code[endThen:])
		if strings.HasPrefix(strings.ToLower(rest), "else") {
			ei := strings.Index(strings.ToLower(code[endThen:]), "else")
			afterElse := endThen + ei + 4
			for afterElse < len(code) && (code[afterElse] == ' ' || code[afterElse] == '\t' || code[afterElse] == '\n' || code[afterElse] == '\r') {
				afterElse++
			}
			if afterElse < len(code) && code[afterElse] == '{' {
				eb, endElse, ok2 := readBalancedBrace(code, afterElse)
				if ok2 {
					elseBody = eb
					endAll = endElse
				}
			}
		}
		ifSpans = append(ifSpans, span{loc[0], endAll})
		matchedSpans[fmt.Sprintf("%d:%d", loc[0], endAll)] = true
		eq := 1
		val := strings.ToLower(sub[2])
		if strings.Contains(val, "zero") || val == "0" {
			eq = 0
		}
		// MEASURE already emitted from let; only emit IF (same flat/block rules as emitBlockIF)
		thenLines := bodyToQuell(thenBody)
		elseLines := bodyToQuell(elseBody)
		pos := loc[0]
		if len(thenLines) == 0 && len(elseLines) == 0 {
			continue
		}
		if len(thenLines) <= 1 && len(elseLines) <= 1 {
			if len(thenLines) == 1 {
				hits = append(hits, qsHit{pos: pos, line: fmt.Sprintf("IF c[%d]==%d %s", cbit, eq, thenLines[0])})
			}
			if len(elseLines) == 1 {
				elseEq := 0
				if eq == 0 {
					elseEq = 1
				}
				hits = append(hits, qsHit{pos: pos + 1, line: fmt.Sprintf("IF c[%d]==%d %s", cbit, elseEq, elseLines[0])})
			}
			continue
		}
		hits = append(hits, qsHit{pos: pos, line: fmt.Sprintf("IF c[%d]==%d {", cbit, eq)})
		for _, l := range thenLines {
			hits = append(hits, qsHit{pos: pos, line: "  " + l})
		}
		hits = append(hits, qsHit{pos: pos, line: "}"})
		if len(elseLines) > 0 {
			hits = append(hits, qsHit{pos: pos + 1, line: "ELSE {"})
			for _, l := range elseLines {
				hits = append(hits, qsHit{pos: pos + 1, line: "  " + l})
			}
			hits = append(hits, qsHit{pos: pos + 1, line: "}"})
		}
	}

	// Drop unconditional gates that were matched inside if/else bodies.
	if len(ifSpans) > 0 {
		filtered := hits[:0]
		for _, h := range hits {
			inside := false
			isCtrl := strings.HasPrefix(h.line, "IF ") || strings.HasPrefix(h.line, "ELSE") ||
				h.line == "}" || strings.HasPrefix(h.line, "  ")
			if !isCtrl {
				for _, sp := range ifSpans {
					if h.pos >= sp.a && h.pos < sp.b {
						inside = true
						break
					}
				}
			}
			// MEASURE from M() inside if condition (not the pre-emitted one at pos-1)
			if strings.HasPrefix(h.line, "MEASURE") && !isCtrl {
				for _, sp := range ifSpans {
					if h.pos >= sp.a && h.pos < sp.b {
						inside = true
						break
					}
				}
			}
			if !inside {
				filtered = append(filtered, h)
			}
		}
		hits = filtered
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].pos != hits[j].pos {
			return hits[i].pos < hits[j].pos
		}
		// Keep MEASURE before IF when same-ish position
		if strings.HasPrefix(hits[i].line, "MEASURE") {
			return true
		}
		if strings.HasPrefix(hits[j].line, "MEASURE") {
			return false
		}
		return false
	})

	known := map[string]bool{
		"h": true, "x": true, "y": true, "z": true, "s": true, "t": true, "i": true,
		"cnot": true, "cx": true, "cz": true, "swap": true, "ccnot": true, "ccx": true,
		"rx": true, "ry": true, "rz": true, "m": true, "measure": true, "reset": true,
		"result": true, "qubit": true, "bool": true, "unit": true, "int": true, "double": true,
		"message": true, "dumpmachine": true, "one": true, "zero": true, "pauli": true,
		"rxx": true, "ryy": true, "rzz": true, "adjoint": true, "controlled": true,
		"operation": true, "function": true, "namespace": true, "open": true, "export": true,
		"internal": true, "use": true, "borrow": true, "let": true, "mutable": true,
		"set": true, "return": true, "within": true, "apply": true, "body": true,
		"factory": true, "auto": true, "is": true, "newtype": true, "struct": true,
		"if": true, "elif": true, "else": true, "for": true, "while": true, "repeat": true,
		"until": true, "fix": true, "and": true, "or": true, "not": true,
	}
	// User-defined operation/function names are not missing gates.
	for _, m := range qsOpDecl.FindAllStringSubmatch(code, -1) {
		known[strings.ToLower(m[1])] = true
	}
	seenWarn := map[string]bool{}
	for _, m := range qsUnknownGate.FindAllStringSubmatchIndex(code, -1) {
		name := code[m[2]:m[3]]
		if known[strings.ToLower(name)] {
			continue
		}
		overlapped := false
		for span := range matchedSpans {
			var a, b int
			fmt.Sscanf(span, "%d:%d", &a, &b)
			if m[0] >= a && m[0] < b {
				overlapped = true
				break
			}
		}
		if overlapped {
			continue
		}
		key := strings.ToLower(name)
		if seenWarn[key] {
			continue
		}
		seenWarn[key] = true
		warnings = append(warnings, fmt.Sprintf("unsupported Q# call %q — not in Quell subset yet", name))
		if len(warnings) >= 12 {
			warnings = append(warnings, "…additional unsupported Q# calls omitted")
			break
		}
	}

	var lines []string
	lines = append(lines, "// Converted from Q# (H/X/Y/Z/S/T/SDG/TDG/CNOT/CZ/SWAP/CCX/Rx/Ry/Rz/Measure/Reset + if/else feedforward)")
	for _, h := range hits {
		lines = append(lines, h.line)
	}
	hasM := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(l)), "MEASURE") {
			hasM = true
			break
		}
	}
	if !hasM && len(lines) > 1 {
		lines = append(lines, "MEASURE")
	}
	if len(lines) <= 1 {
		return "", warnings, fmt.Errorf("no supported Q# gates found (H, X, Y, Z, S, T, CNOT, Measure, if/else feedforward, …) — try OpenQASM or Qiskit")
	}
	return strings.Join(lines, "\n") + "\n", warnings, nil
}

// readBalancedBrace reads the body inside { … } starting at openIdx pointing at '{'.
// Returns inner body (without braces), index just after the closing '}', and ok.
func readBalancedBrace(src string, openIdx int) (body string, end int, ok bool) {
	if openIdx < 0 || openIdx >= len(src) || src[openIdx] != '{' {
		return "", openIdx, false
	}
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[openIdx+1 : i], i + 1, true
			}
		}
	}
	return "", openIdx, false
}
