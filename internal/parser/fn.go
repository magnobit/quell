// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/qerr"
)

// FnParam is one immutable host-function parameter.
type FnParam struct {
	Name   string
	Type   string
	Col    int
	EndCol int
}

// FnStmt is one statement in a host function.
// Kind is "let", "var", "assign", "return", "if", "for", "while", "break", "continue", or "call".
type FnStmt struct {
	Kind   string
	Decl   HostDecl
	Expr   Expr
	If     *IfStmt
	Loop   *LoopStmt
	Line   int
	Col    int
	EndCol int
}

// FnDecl is a host function. It is not a gate macro and not an IR operation.
type FnDecl struct {
	Name    string
	Params  []FnParam
	Ret     string
	Body    []FnStmt
	Line    int
	Col     int
	EndLine int
	EndCol  int
}

// Canonical returns the formatter's layout for fn.
func (fn FnDecl) Canonical() string {
	var b strings.Builder
	b.WriteString("fn ")
	b.WriteString(fn.Name)
	b.WriteByte('(')
	for i, p := range fn.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
		b.WriteString(": ")
		b.WriteString(p.Type)
	}
	b.WriteString(") -> ")
	b.WriteString(fn.Ret)
	b.WriteString(" {\n")
	for _, st := range fn.Body {
		b.WriteString("    ")
		b.WriteString(formatFnStmt(st, "    "))
		b.WriteByte('\n')
	}
	b.WriteByte('}')
	return b.String()
}

// CanonicalFunction formats src when it is a single host function.
func CanonicalFunction(src string) (string, bool) {
	c, err := Parse(src)
	if err != nil || c == nil || len(c.Functions) != 1 || len(c.Instructions) != 0 || len(c.Host) != 0 {
		return "", false
	}
	return c.Functions[0].Canonical(), true
}

// TakeFunctionLines returns the source lines of one function starting at start,
// including the header. n is the number of raw lines consumed. ok is false when
// the line is not a function or the body is unclosed.
func TakeFunctionLines(lines []string, start int) (block []string, n int, ok bool) {
	if start < 0 || start >= len(lines) || !isFnLine(stripLineComment(lines[start])) {
		return nil, 0, false
	}
	depth := 0
	seenBrace := false
	for i := start; i < len(lines); i++ {
		block = append(block, lines[i])
		code := stripLineComment(lines[i])
		nd, closed, saw := braceWalk(code, depth, seenBrace)
		depth, seenBrace = nd, saw
		if closed {
			return block, i - start + 1, true
		}
	}
	return block, len(block), false
}

func isFnLine(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) >= 7 && strings.EqualFold(s[:7], "quantum") && (len(s) == 7 || !isIdentCont(s[7])) {
		rest := strings.TrimSpace(s[7:])
		return isFnLine(rest)
	}
	if len(s) < 2 || !strings.EqualFold(s[:2], "fn") {
		return false
	}
	return len(s) == 2 || !isIdentCont(s[2])
}

func isReturnLine(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 6 || !strings.EqualFold(s[:6], "return") {
		return false
	}
	return len(s) == 6 || !isIdentCont(s[6])
}

func isQuantumFnLine(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 7 || !strings.EqualFold(s[:7], "quantum") || (len(s) > 7 && isIdentCont(s[7])) {
		return false
	}
	return isFnLine(strings.TrimSpace(s[7:]))
}

// parseFunctionAt parses a function whose header is header at headerLine.
// idx points at the next source line and is advanced past the body.
func parseFunctionAt(allLines []string, idx *int, header string, headerLine int) (FnDecl, error) {
	if isQuantumFnLine(header) {
		return FnDecl{}, fnSyntax(headerLine, 1, len(header)+1, "quantum fn is not supported", "Use gate for a compile-time quantum macro, or fn for a host function.")
	}
	fn, after, hasBrace, err := parseFnHeader(header, headerLine)
	if err != nil {
		return FnDecl{}, err
	}
	if !hasBrace {
		for *idx < len(allLines) {
			raw := allLines[*idx]
			lineNum := *idx + 1
			*idx++
			code := strings.TrimSpace(stripLineComment(raw))
			if code == "" {
				continue
			}
			if strings.HasPrefix(code, "{") {
				hasBrace = true
				after = strings.TrimSpace(code[1:])
				fn.EndLine = lineNum
				break
			}
			return FnDecl{}, fnSyntax(headerLine, 1, len(header)+1, "function requires a { body", "Write fn name(params) -> type { return expr }.")
		}
		if !hasBrace {
			return FnDecl{}, fnSyntax(headerLine, 1, len(header)+1, "unclosed function body", "Close the function with }.")
		}
	}
	body, endLine, err := collectFnBody(allLines, idx, after, fn.EndLine)
	if err != nil {
		return FnDecl{}, err
	}
	fn.EndLine = endLine
	stmts, err := parseFnStmts(body)
	if err != nil {
		return FnDecl{}, err
	}
	fn.Body = stmts
	if n := countFnLets(stmts); n > MaxHostDecls {
		return FnDecl{}, hostLimit(fn.Line, 1, fmt.Sprintf("more than %d host declarations", MaxHostDecls))
	}
	return fn, nil
}

func formatFnLet(d HostDecl, base string) string {
	if ie, ok := d.Expr.(*IfExpr); ok {
		return "let " + d.Name + ": " + d.Type + " = " + ie.FormatAt(base)
	}
	return "let " + d.Name + ": " + d.Type + " = " + FormatExpr(d.Expr)
}

func parseFnStmts(body []srcLine) ([]FnStmt, error) {
	var stmts []FnStmt
	for i := 0; i < len(body); {
		st, n, err := parseFnStmtSpan(body[i:])
		if err != nil {
			return nil, err
		}
		if n < 1 {
			return nil, fnSyntax(body[i].Line, 1, 2, "empty host statement", "Write let, if, or return.")
		}
		stmts = append(stmts, st)
		i += n
	}
	return stmts, nil
}

func parseFnStmtSpan(lines []srcLine) (FnStmt, int, error) {
	trim := lines[0].Text
	lineNum := lines[0].Line
	if isHostIfStatement(trim) {
		c := &mlCursor{lines: lines}
		ifs, err := c.parseIfStmt(1)
		if err != nil {
			return FnStmt{}, 0, err
		}
		if ifs.EndLine == c.line() {
			rest := strings.TrimSpace(c.cur()[c.bi:])
			if rest != "" {
				return FnStmt{}, 0, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+len(rest), "unexpected tokens after host if", "End the statement at the closing }.")
			}
		}
		n := ifs.EndLine - lines[0].Line + 1
		if n < 1 {
			n = 1
		}
		if n > len(lines) {
			n = len(lines)
		}
		return FnStmt{Kind: "if", If: ifs, Line: ifs.Line, Col: ifs.Col, EndCol: ifs.EndCol}, n, nil
	}
	if isExactWord(trim, "for") || isExactWord(trim, "while") {
		c := &mlCursor{lines: lines}
		st, err := c.parseLoopStmt(1)
		if err != nil {
			return FnStmt{}, 0, err
		}
		n := 1
		if st.Loop != nil {
			n = st.Loop.EndLine - lines[0].Line + 1
		}
		if n < 1 {
			n = 1
		}
		if n > len(lines) {
			n = len(lines)
		}
		return st, n, nil
	}
	if len(trim) >= 3 && strings.EqualFold(trim[:3], "let") && (len(trim) == 3 || !isIdentCont(trim[3])) {
		d, n, err := parseLetSpan(lines)
		if err != nil {
			return FnStmt{}, 0, err
		}
		return FnStmt{Kind: "let", Decl: d, Line: lineNum, Col: d.Col, EndCol: d.EndCol}, n, nil
	}
	if isReturnLine(trim) {
		i := 6
		for i < len(trim) && (trim[i] == ' ' || trim[i] == '\t') {
			i++
		}
		if i >= len(trim) {
			return FnStmt{}, 0, fnSyntax(lineNum, 1, 7, "return requires an expression", "Return one scalar: return expr.")
		}
		if isHostIfKeyword(trim[i:]) {
			ife, n, err := parseIfFrom(lines, i, 1)
			if err != nil {
				return FnStmt{}, 0, err
			}
			return FnStmt{Kind: "return", Expr: ife, Line: lineNum, Col: 1, EndCol: ife.EndCol}, n, nil
		}
		ex, err := ParseExpr(trim[i:], lineNum, i+1)
		if err != nil {
			return FnStmt{}, 0, err
		}
		_, end := ex.Span()
		return FnStmt{Kind: "return", Expr: ex, Line: lineNum, Col: 1, EndCol: end}, 1, nil
	}
	st, err := parseFnStmt(trim, lineNum)
	if err != nil {
		return FnStmt{}, 0, err
	}
	return st, 1, nil
}

type srcLine struct {
	Line int
	Text string
}

func collectFnBody(allLines []string, idx *int, firstInner string, firstLine int) ([]srcLine, int, error) {
	depth := 1
	var buf []srcLine
	endLine := firstLine
	push := func(s string, line int) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		buf = append(buf, srcLine{Line: line, Text: strings.TrimSuffix(s, ";")})
	}
	before, depth, closed := applyBraces(firstInner, depth)
	if strings.TrimSpace(firstInner) != "" || closed {
		if closed {
			push(before, firstLine)
			return buf, firstLine, nil
		}
		push(before, firstLine)
	}
	for *idx < len(allLines) {
		raw := allLines[*idx]
		lineNum := *idx + 1
		*idx++
		code := stripLineComment(raw)
		before, depth, closed = applyBraces(code, depth)
		endLine = lineNum
		if closed {
			push(before, lineNum)
			return buf, endLine, nil
		}
		push(before, lineNum)
	}
	return nil, 0, fnSyntax(firstLine, 1, 2, "unclosed function body", "Close the function with }.")
}

// applyBraces walks s, counting braces outside strings. depth starts above 0.
// When depth reaches 0, before is the text before that closing brace.
func applyBraces(s string, depth int) (before string, newDepth int, closed bool) {
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			depth++
			continue
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return s[:i], 0, true
			}
		}
	}
	return s, depth, false
}

// braceWalk is the formatter's view of the same brace rule. seenBrace becomes
// true at the first {. closed is true when that function body ends.
func braceWalk(s string, depth int, seenBrace bool) (newDepth int, closed, saw bool) {
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			seenBrace = true
			depth++
			continue
		}
		if c == '}' && seenBrace {
			depth--
			if depth == 0 {
				return 0, true, true
			}
		}
	}
	return depth, false, seenBrace
}

func stripLineComment(raw string) string {
	if ci := strings.Index(raw, "//"); ci >= 0 {
		return raw[:ci]
	}
	return raw
}

func parseFnHeader(line string, lineNum int) (FnDecl, string, bool, error) {
	s := strings.TrimSpace(line)
	lead := 0
	for lead < len(line) && (line[lead] == ' ' || line[lead] == '\t') {
		lead++
	}
	if !isFnLine(s) {
		return FnDecl{}, "", false, fnSyntax(lineNum, 1, len(line)+1, "expected fn", "Write fn name(params) -> type { return expr }.")
	}
	i := 2
	if len(s) >= 7 && strings.EqualFold(s[:7], "quantum") {
		return FnDecl{}, "", false, fnSyntax(lineNum, lead+1, lead+7, "quantum fn is not supported", "Use gate for a compile-time quantum macro, or fn for a host function.")
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) || !isIdentStart(s[i]) {
		return FnDecl{}, "", false, fnSyntax(lineNum, lead+i+1, lead+len(s)+1, "function requires a name", "Write fn name(params) -> type { return expr }.")
	}
	name, nlen := scanIdent(s[i:])
	nameCol := lead + i + 1
	if len(name) > MaxIdentLen {
		return FnDecl{}, "", false, hostLimit(lineNum, nameCol, "identifier longer than "+fmt.Sprint(MaxIdentLen)+" bytes")
	}
	if isReservedHostName(name) {
		return FnDecl{}, "", false, fnSyntax(lineNum, nameCol, nameCol+len(name), fmt.Sprintf("%q is reserved and cannot be a function name", name), "Choose a different function name.")
	}
	i += nlen
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) || s[i] != '(' {
		return FnDecl{}, "", false, fnSyntax(lineNum, nameCol, nameCol+len(name), "function requires a parameter list", "Write fn name(params) -> type { return expr }.")
	}
	i++
	params, next, err := parseFnParams(s[i:], lineNum, lead+i+1)
	if err != nil {
		return FnDecl{}, "", false, err
	}
	i += next
	if i >= len(s) || s[i] != ')' {
		return FnDecl{}, "", false, fnSyntax(lineNum, lead+i+1, lead+len(s)+1, "function parameter list is missing )", "Close the parameter list before ->.")
	}
	i++
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i+1 >= len(s) || s[i] != '-' || s[i+1] != '>' {
		return FnDecl{}, "", false, fnSyntax(lineNum, nameCol, nameCol+len(name), "function requires a return type", "Write fn name(params) -> type { return expr }.")
	}
	i += 2
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	typ, tlen, ok := scanHostType(s[i:])
	if !ok {
		return FnDecl{}, "", false, fnSyntax(lineNum, lead+i+1, lead+len(s)+1, "function return type must be bool, int, float, string, or an array of those", "Declare the return type explicitly.")
	}
	i += tlen
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	fn := FnDecl{Name: name, Params: params, Ret: typ, Line: lineNum, Col: nameCol, EndLine: lineNum, EndCol: nameCol + len(name)}
	if i < len(s) && s[i] == '{' {
		return fn, s[i+1:], true, nil
	}
	if i < len(s) {
		return FnDecl{}, "", false, fnSyntax(lineNum, lead+i+1, lead+len(s)+1, "unexpected tokens after the function signature", "Put the body in { } after the return type.")
	}
	return fn, "", false, nil
}

func parseFnParams(s string, lineNum, colBase int) ([]FnParam, int, error) {
	i := 0
	var params []FnParam
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return nil, i, fnSyntax(lineNum, colBase+i, colBase+i+1, "function parameter list is missing )", "Close the parameter list before ->.")
		}
		if s[i] == ')' {
			return params, i, nil
		}
		if len(params) >= MaxFnParams {
			return nil, i, hostLimit(lineNum, colBase+i, fmt.Sprintf("function has more than %d parameters", MaxFnParams))
		}
		if !isIdentStart(s[i]) {
			return nil, i, fnSyntax(lineNum, colBase+i, colBase+i+1, "invalid parameter name", "Write parameters as name: type.")
		}
		name, nlen := scanIdent(s[i:])
		col := colBase + i
		if isReservedHostName(name) {
			return nil, i, fnSyntax(lineNum, col, col+len(name), fmt.Sprintf("%q is reserved and cannot be a parameter name", name), "Choose a different parameter name.")
		}
		i += nlen
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) || s[i] != ':' {
			return nil, i, fnSyntax(lineNum, col, col+len(name), "parameter requires a type", "Write name: type.")
		}
		i++
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		typ, tlen, ok := scanHostType(s[i:])
		if !ok {
			return nil, i, fnSyntax(lineNum, col, col+len(name), "parameter type must be bool, int, float, string, or an array of those", "Use a host type.")
		}
		i += tlen
		params = append(params, FnParam{Name: name, Type: typ, Col: col, EndCol: col + len(name)})
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && s[i] == ',' {
			i++
			continue
		}
		if i < len(s) && s[i] == ')' {
			return params, i, nil
		}
		return nil, i, fnSyntax(lineNum, colBase+i, colBase+i+1, "expected , or ) in the parameter list", "Separate parameters with commas.")
	}
}

func parseFnStmt(line string, lineNum int) (FnStmt, error) {
	trim := strings.TrimSpace(line)
	if isFnLine(trim) || isQuantumFnLine(trim) {
		return FnStmt{}, fnSyntax(lineNum, 1, len(trim)+1, "a host function cannot contain another function", "Keep each fn at program scope.")
	}
	if isExactWord(trim, "var") {
		d, err := parseVarLine(trim, lineNum)
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "var", Decl: d, Line: lineNum, Col: d.Col, EndCol: d.EndCol}, nil
	}
	if isExactWord(trim, "break") {
		return FnStmt{Kind: "break", Line: lineNum, Col: 1, EndCol: 6}, nil
	}
	if isExactWord(trim, "continue") {
		return FnStmt{Kind: "continue", Line: lineNum, Col: 1, EndCol: 9}, nil
	}
	if isExactWord(trim, "print") || isExactWord(trim, "println") {
		return parsePrintStmt(trim, lineNum)
	}
	if len(trim) >= 3 && strings.EqualFold(trim[:3], "let") && (len(trim) == 3 || !isIdentCont(trim[3])) {
		d, err := parseLetLine(trim, lineNum)
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "let", Decl: d, Line: lineNum, Col: d.Col, EndCol: d.EndCol}, nil
	}
	if isReturnLine(trim) {
		i := 6
		for i < len(trim) && (trim[i] == ' ' || trim[i] == '\t') {
			i++
		}
		if i >= len(trim) {
			return FnStmt{}, fnSyntax(lineNum, 1, 7, "return requires an expression", "Return one scalar: return expr.")
		}
		ex, err := ParseExpr(trim[i:], lineNum, i+1)
		if err != nil {
			return FnStmt{}, err
		}
		_, end := ex.Span()
		return FnStmt{Kind: "return", Expr: ex, Line: lineNum, Col: 1, EndCol: end}, nil
	}
	if d, ok, err := parseAssignLine(trim, lineNum); ok {
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "assign", Decl: d, Line: lineNum, Col: d.Col, EndCol: d.EndCol}, nil
	}
	return FnStmt{}, fnSyntax(lineNum, 1, len(trim)+1, "a host function may only contain let, var, if, for, while, print, and return", "Move quantum gates out of the function. gate is a compile-time macro, not a host function.")
}

func isExactWord(s, word string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= len(word) && s[:len(word)] == word && (len(s) == len(word) || !isIdentCont(s[len(word)]))
}

func fnSyntax(line, col, end int, msg, fix string) error {
	return qerr.New(qerr.KindParse, qerr.Diagnostic{
		Code: qerr.CodeSyntax, Severity: qerr.SeverityError, Message: msg,
		Line: line, Column: col, EndLine: line, EndColumn: end,
		SuggestedFix: fix, DocsURL: qerr.DocsHostFunctions,
	})
}
