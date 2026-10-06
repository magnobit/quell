// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/magnobit/quell/qerr"
)

// QuantumFn is a reusable quantum kernel. Calls are inlined into the
// existing instruction list before IR lowering. It is not a gate macro
// and not a host function.
type QuantumFn struct {
	Name   string
	Params []QParam
	Body   []string
	Line   int
}

// QParam is one quantum-fn parameter.
// Type is qubit, qubit[], float, or int.
type QParam struct {
	Name string
	Type string
}

// QubitRegister is a statically sized qubit collection. Base is the first
// allocated index. There is no manual free: the register lives until the
// circuit ends. RESET uses the existing reset gate.
type QubitRegister struct {
	Name string
	Size int
	Base int
	Line int
}

const maxQubitRegister = 24

func isQuantumModifier(tok string) bool {
	switch strings.ToLower(tok) {
	case "adjoint", "inverse", "controlled":
		return true
	default:
		return false
	}
}

func registerSlice(regs map[string]QubitRegister) []QubitRegister {
	out := make([]QubitRegister, 0, len(regs))
	for _, r := range regs {
		out = append(out, r)
	}
	return out
}

func splitRegister(tok string) (string, int, bool) {
	i := strings.IndexByte(tok, '[')
	if i <= 0 || !strings.HasSuffix(tok, "]") {
		return "", 0, false
	}
	name := tok[:i]
	if !isIdent(name) {
		return "", 0, false
	}
	n, err := strconv.Atoi(tok[i+1 : len(tok)-1])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return name, n, true
}

func parseQuantumAt(lines []string, idx *int, header string, headerLine int) (QuantumFn, error) {
	s := strings.TrimSpace(header)
	rest := strings.TrimSpace(s[len("quantum"):])
	if len(rest) < 2 || !strings.EqualFold(rest[:2], "fn") || (len(rest) > 2 && isIdentCont(rest[2])) {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn is not supported", "Write quantum fn name(q: qubit) { H q }.")
	}
	rest = strings.TrimSpace(rest[2:])
	if strings.Contains(rest, "->") {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn has no host return type", "Drop -> type. A quantum fn is a kernel, not a host function.")
	}
	name, nlen := scanIdent(rest)
	if name == "" || !isIdent(name) {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn requires a name", "Write quantum fn name(q: qubit) { ... }.")
	}
	if _, builtin := GateArity[strings.ToUpper(name)]; builtin {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn cannot reuse a gate name", "Choose a name that is not a built-in gate. gate remains a compile-time macro.")
	}
	rest = strings.TrimSpace(rest[nlen:])
	if rest == "" || rest[0] != '(' {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn requires a parameter list", "Write quantum fn name(q: qubit) { ... }.")
	}
	endParen := strings.IndexByte(rest, ')')
	if endParen < 0 {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn parameter list is missing )", "Close the parameter list.")
	}
	params, err := parseQParams(rest[1:endParen], headerLine)
	if err != nil {
		return QuantumFn{}, err
	}
	after := strings.TrimSpace(rest[endParen+1:])
	if after == "" || after[0] != '{' {
		return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn requires a body", "Open the body with {.")
	}
	body, err := takeQuantumBody(lines, idx, after[1:], headerLine)
	if err != nil {
		return QuantumFn{}, err
	}
	for _, line := range body {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if word, ok := hostOnlyWord(strings.Fields(trim)[0]); ok {
			return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn cannot call host "+word, "Keep print, loops, and mutation in a host fn. A quantum fn only contains gates and quantum calls.")
		}
		low := strings.ToLower(trim)
		if strings.HasPrefix(low, "let ") || strings.HasPrefix(low, "var ") || strings.HasPrefix(low, "fn ") || strings.HasPrefix(low, "return") || strings.HasPrefix(low, "print") {
			return QuantumFn{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn cannot contain host statements", "Move host code out of the kernel.")
		}
	}
	return QuantumFn{Name: name, Params: params, Body: body, Line: headerLine}, nil
}

func parseQParams(s string, line int) ([]QParam, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []QParam
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, typ, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fnSyntax(line, 1, len(part)+1, "quantum parameter requires a type", "Write name: qubit, name: qubit[], or name: float.")
		}
		name = strings.TrimSpace(name)
		typ = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(typ), " ", ""))
		if !isIdent(name) {
			return nil, fnSyntax(line, 1, len(name)+1, "invalid quantum parameter name", "Use a plain identifier.")
		}
		switch typ {
		case "qubit", "qubit[]", "float", "int":
		default:
			return nil, fnSyntax(line, 1, len(part)+1, "quantum parameter type must be qubit, qubit[], float, or int", "Host-only types stay on fn.")
		}
		out = append(out, QParam{Name: name, Type: typ})
	}
	return out, nil
}

func takeQuantumBody(lines []string, idx *int, first string, headerLine int) ([]string, error) {
	var chunks []string
	depth := 1
	consume := func(s string) (done bool, err error) {
		for _, r := range s {
			if r == '{' {
				depth++
			}
			if r == '}' {
				depth--
				if depth == 0 {
					return true, nil
				}
				if depth < 0 {
					return false, fnSyntax(headerLine, 1, 2, "unbalanced quantum fn body", "Close the body with }.")
				}
			}
		}
		return false, nil
	}
	before, _, found := strings.Cut(first, "}")
	if found && !strings.Contains(before, "{") {
		if strings.TrimSpace(before) != "" {
			chunks = append(chunks, before)
		}
		return splitQuantumBody(chunks), nil
	}
	if strings.TrimSpace(first) != "" {
		done, err := consume(first)
		if err != nil {
			return nil, err
		}
		if done {
			cut := strings.LastIndex(first, "}")
			if cut > 0 {
				chunks = append(chunks, first[:cut])
			}
			return splitQuantumBody(chunks), nil
		}
		chunks = append(chunks, first)
	}
	for *idx < len(lines) {
		raw := lines[*idx]
		*idx++
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		done, err := consume(raw)
		if err != nil {
			return nil, err
		}
		if done {
			cut := strings.LastIndex(raw, "}")
			if cut > 0 {
				chunks = append(chunks, raw[:cut])
			}
			return splitQuantumBody(chunks), nil
		}
		chunks = append(chunks, raw)
	}
	return nil, fnSyntax(headerLine, 1, 2, "quantum fn is missing }", "Close the kernel body.")
}

func splitQuantumBody(chunks []string) []string {
	text := strings.Join(chunks, "\n")
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		for _, part := range strings.Split(line, ";") {
			part = strings.TrimSpace(part)
			if part != "" {
				lines = append(lines, part)
			}
		}
	}
	return lines
}

type qbind struct {
	kind   string
	index  int
	base   int
	size   int
	scalar string
}

func expandQuantumCall(fn *QuantumFn, args []string, qfns map[string]*QuantumFn, stack []string, named map[string]int, regs map[string]QubitRegister, line int) ([]Instruction, error) {
	for _, s := range stack {
		if s == fn.Name {
			return nil, qerr.New(qerr.KindParse, qerr.Diagnostic{
				Code: qerr.CodeRecursion, Severity: qerr.SeverityError,
				Message: fmt.Sprintf("quantum fn %s calls itself", fn.Name),
				Line:    line, Column: 1, EndLine: line, EndColumn: 2,
				SuggestedFix: "Remove the recursive call. Bounded quantum recursion is not defined.",
			})
		}
	}
	if len(stack) > 32 {
		return nil, fmt.Errorf("line %d: quantum fn call depth exceeds 32", line)
	}
	clean := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.Trim(a, ", \t")
		if a != "" {
			clean = append(clean, a)
		}
	}
	if len(clean) != len(fn.Params) {
		return nil, fmt.Errorf("line %d: quantum fn %s expects %d argument(s), got %d", line, fn.Name, len(fn.Params), len(clean))
	}
	binds := map[string]qbind{}
	for i, p := range fn.Params {
		arg := clean[i]
		switch p.Type {
		case "qubit":
			idx, ok := resolveQubitArg(arg, named, regs)
			if !ok {
				return nil, fmt.Errorf("line %d: %s argument %q is not a qubit", line, fn.Name, arg)
			}
			binds[p.Name] = qbind{kind: "qubit", index: idx}
		case "qubit[]":
			reg, ok := regs[arg]
			if !ok {
				return nil, fmt.Errorf("line %d: %s argument %q is not a qubit register", line, fn.Name, arg)
			}
			binds[p.Name] = qbind{kind: "reg", base: reg.Base, size: reg.Size}
		default:
			binds[p.Name] = qbind{kind: "scalar", scalar: arg}
		}
	}
	nextStack := append(append([]string{}, stack...), fn.Name)
	var out []Instruction
	for _, raw := range fn.Body {
		insts, err := expandQuantumLine(raw, qfns, nextStack, binds, named, regs, line)
		if err != nil {
			return nil, err
		}
		out = append(out, insts...)
	}
	return out, nil
}

func expandQuantumLine(raw string, qfns map[string]*QuantumFn, stack []string, binds map[string]qbind, named map[string]int, regs map[string]QubitRegister, line int) ([]Instruction, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil, nil
	}
	return lowerQuantumFields(fields, qfns, stack, binds, named, regs, line)
}

func resolveCallTok(tok string, binds map[string]qbind, named map[string]int, regs map[string]QubitRegister) (string, error) {
	if got, err := substQuantumTok(tok, binds, false); err == nil {
		return got, nil
	}
	if idx, ok := resolveQubitArg(tok, named, regs); ok {
		return strconv.Itoa(idx), nil
	}
	if _, ok := evalAngle(tok); ok {
		return tok, nil
	}
	return "", fmt.Errorf("unknown quantum token %q", tok)
}

func substQuantumTok(tok string, binds map[string]qbind, gate bool) (string, error) {
	if gate {
		return tok, nil
	}
	if name, idx, ok := splitIndex(tok); ok {
		b, known := binds[name]
		if !known || b.kind != "reg" {
			return "", fmt.Errorf("unknown register %q", name)
		}
		if idx < 0 || idx >= b.size {
			return "", fmt.Errorf("QL2601: index out of bounds on %s", name)
		}
		return strconv.Itoa(b.base + idx), nil
	}
	if b, ok := binds[tok]; ok {
		switch b.kind {
		case "qubit":
			return strconv.Itoa(b.index), nil
		case "scalar":
			return b.scalar, nil
		default:
			return "", fmt.Errorf("%s is a register; index it", tok)
		}
	}
	if _, err := strconv.Atoi(tok); err == nil {
		return tok, nil
	}
	if _, ok := evalAngle(tok); ok {
		return tok, nil
	}
	return "", fmt.Errorf("unknown quantum token %q", tok)
}

func splitIndex(tok string) (string, int, bool) {
	i := strings.IndexByte(tok, '[')
	if i <= 0 || !strings.HasSuffix(tok, "]") {
		return "", 0, false
	}
	n, err := strconv.Atoi(tok[i+1 : len(tok)-1])
	if err != nil {
		return "", 0, false
	}
	return tok[:i], n, true
}

func resolveQubitArg(arg string, named map[string]int, regs map[string]QubitRegister) (int, bool) {
	if idx, ok := named[arg]; ok {
		return idx, true
	}
	if name, k, ok := splitIndex(arg); ok {
		if reg, ok := regs[name]; ok && k >= 0 && k < reg.Size {
			return reg.Base + k, true
		}
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func adjointInstructions(in []Instruction, line int) ([]Instruction, error) {
	out := make([]Instruction, 0, len(in))
	for i := len(in) - 1; i >= 0; i-- {
		inst, ok := adjointOne(in[i])
		if !ok {
			return nil, fmt.Errorf("line %d: adjoint is not defined for %s", line, in[i].Gate)
		}
		inst.Line = line
		out = append(out, inst)
	}
	return out, nil
}

func adjointOne(in Instruction) (Instruction, bool) {
	switch in.Gate {
	case "H", "X", "Y", "Z", "CNOT", "CZ", "SWAP", "BARRIER":
		return in, true
	case "S":
		in.Gate = "SDG"
		return in, true
	case "SDG":
		in.Gate = "S"
		return in, true
	case "T":
		in.Gate = "TDG"
		return in, true
	case "TDG":
		in.Gate = "T"
		return in, true
	case "RX", "RY", "RZ", "P":
		if len(in.Args) != 1 {
			return in, false
		}
		if len(in.ArgNames) > 0 && in.ArgNames[0] != "" {
			return in, false
		}
		args := append([]float64(nil), in.Args...)
		args[0] = -args[0]
		in.Args = args
		return in, true
	default:
		return in, false
	}
}

func lowerQuantumStatement(tokens []string, qfns map[string]*QuantumFn, stack []string, named map[string]int, regs map[string]QubitRegister, line int) ([]Instruction, error) {
	return lowerQuantumFields(tokens, qfns, stack, nil, named, regs, line)
}

func lowerQuantumFields(fields []string, qfns map[string]*QuantumFn, stack []string, binds map[string]qbind, named map[string]int, regs map[string]QubitRegister, line int) ([]Instruction, error) {
	mods, rest := splitQuantumMods(fields)
	if len(rest) == 0 {
		return nil, fmt.Errorf("line %d: quantum call is missing a name", line)
	}
	nCtrl := 0
	adjoint := false
	for _, m := range mods {
		switch m {
		case "controlled":
			nCtrl++
		case "adjoint", "inverse":
			adjoint = true
		default:
			return nil, fmt.Errorf("line %d: unknown quantum modifier %s", line, m)
		}
	}
	if nCtrl > len(rest)-1 {
		return nil, fmt.Errorf("line %d: controlled call needs %d control qubit(s)", line, nCtrl)
	}
	name := strings.TrimRight(rest[0], ",")
	ctrlToks := rest[1 : 1+nCtrl]
	body := append([]string{name}, rest[1+nCtrl:]...)
	insts, err := expandPlainQuantum(body, qfns, stack, binds, named, regs, line)
	if err != nil {
		return nil, err
	}
	if adjoint {
		insts, err = adjointInstructions(insts, line)
		if err != nil {
			return nil, err
		}
	}
	if nCtrl == 0 {
		return insts, nil
	}
	ctrls := make([]int, nCtrl)
	for i, tok := range ctrlToks {
		got, err := resolveCallTok(strings.TrimRight(tok, ","), binds, named, regs)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		idx, ok := resolveQubitArg(got, named, regs)
		if !ok {
			return nil, fmt.Errorf("line %d: control %q is not a qubit", line, tok)
		}
		ctrls[i] = idx
	}
	return controlInstructions(insts, ctrls, line)
}

func splitQuantumMods(fields []string) (mods []string, rest []string) {
	i := 0
	for i < len(fields) {
		word := strings.ToLower(strings.TrimRight(fields[i], ","))
		if !isQuantumModifier(word) {
			break
		}
		mods = append(mods, word)
		i++
	}
	return mods, fields[i:]
}

func expandPlainQuantum(fields []string, qfns map[string]*QuantumFn, stack []string, binds map[string]qbind, named map[string]int, regs map[string]QubitRegister, line int) ([]Instruction, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("line %d: empty quantum call", line)
	}
	name := strings.TrimRight(fields[0], ",")
	if fn, ok := qfns[name]; ok {
		args := make([]string, 0, len(fields)-1)
		for _, tok := range fields[1:] {
			got, err := resolveCallTok(strings.TrimRight(tok, ","), binds, named, regs)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			args = append(args, got)
		}
		return expandQuantumCall(fn, args, qfns, stack, named, regs, line)
	}
	rewritten := make([]string, len(fields))
	for i, tok := range fields {
		tok = strings.TrimRight(tok, ",")
		got, err := substQuantumTok(tok, binds, i == 0)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rewritten[i] = got
	}
	sub, err := Parse(strings.Join(rewritten, " "))
	if err != nil {
		return nil, fmt.Errorf("line %d: quantum fn body: %w", line, err)
	}
	if len(sub.Functions) > 0 || len(sub.Host) > 0 || len(sub.Quantum) > 0 {
		return nil, fmt.Errorf("line %d: quantum fn body cannot contain host code", line)
	}
	for i := range sub.Instructions {
		sub.Instructions[i].Line = line
	}
	return sub.Instructions, nil
}

func controlInstructions(in []Instruction, controls []int, line int) ([]Instruction, error) {
	var out []Instruction
	for _, inst := range in {
		next, err := controlOne(inst, controls, line)
		if err != nil {
			return nil, err
		}
		out = append(out, next)
	}
	return out, nil
}

func controlOne(in Instruction, controls []int, line int) (Instruction, error) {
	switch in.Gate {
	case "MEASURE", "RESET", "IF", "WHILE", "SWITCH", "PAR", "ASSERT":
		return Instruction{}, fmt.Errorf("line %d: controlled quantum fn rejected non-unitary %s", line, in.Gate)
	}
	if len(in.Qubits) == 0 {
		return Instruction{}, fmt.Errorf("line %d: cannot control %s without a target", line, in.Gate)
	}
	for _, c := range controls {
		for _, q := range in.Qubits {
			if c == q {
				return Instruction{}, fmt.Errorf("line %d: control qubit %d is also a target", line, c)
			}
		}
	}
	all := append(append([]int{}, controls...), in.Qubits...)
	switch in.Gate {
	case "X":
		if len(controls) == 1 {
			in.Gate = "CNOT"
			in.Qubits = all
			return in, nil
		}
		if len(controls) == 2 {
			in.Gate = "CCX"
			in.Qubits = all
			return in, nil
		}
	case "Z":
		if len(controls) == 1 {
			in.Gate = "CZ"
			in.Qubits = all
			return in, nil
		}
	case "RX":
		if len(controls) == 1 {
			in.Gate = "CRX"
			in.Qubits = all
			return in, nil
		}
	case "RY":
		if len(controls) == 1 {
			in.Gate = "CRY"
			in.Qubits = all
			return in, nil
		}
	case "RZ":
		if len(controls) == 1 {
			in.Gate = "CRZ"
			in.Qubits = all
			return in, nil
		}
	case "SWAP":
		if len(controls) == 1 && len(in.Qubits) == 2 {
			in.Gate = "CSWAP"
			in.Qubits = all
			return in, nil
		}
	case "CNOT":
		if len(controls) == 1 && len(in.Qubits) == 2 {
			in.Gate = "CCX"
			in.Qubits = []int{controls[0], in.Qubits[0], in.Qubits[1]}
			return in, nil
		}
	}
	return Instruction{}, fmt.Errorf("line %d: no safe controlled form for %s with %d control(s)", line, in.Gate, len(controls))
}
