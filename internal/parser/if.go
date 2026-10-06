// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"

	"github.com/magnobit/quell/qerr"
)

// BlockExpr is one branch of a host if. Lets are visible only in this block.
// Result is the single value the branch yields.
type BlockExpr struct {
	Lets       []HostDecl
	Result     Expr
	Line       int
	EndLine    int
	ResultLine int
	Col        int
	EndCol     int
}

// IfExpr is a host conditional expression. It is not a QPU IF operation.
type IfExpr struct {
	Cond    Expr
	Then    *BlockExpr
	Else    *BlockExpr
	Line    int
	EndLine int
	Col     int
	EndCol  int
}

func (e *IfExpr) Span() (int, int)  { return e.Col, e.EndCol }
func (e *IfExpr) Format(int) string { return e.FormatAt("") }

// FormatAt prints the conditional with base as the indent of the if keyword's line.
func (e *IfExpr) FormatAt(base string) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("if ")
	b.WriteString(FormatExpr(e.Cond))
	b.WriteString(" {\n")
	writeHostBlock(&b, e.Then, base+"    ")
	b.WriteString(base)
	b.WriteString("} else {\n")
	writeHostBlock(&b, e.Else, base+"    ")
	b.WriteString(base)
	b.WriteByte('}')
	return b.String()
}

func writeHostBlock(b *strings.Builder, blk *BlockExpr, inner string) {
	if blk == nil {
		b.WriteString(inner)
		b.WriteByte('\n')
		return
	}
	for _, d := range blk.Lets {
		b.WriteString(inner)
		b.WriteString(formatHostLet(d, inner))
		b.WriteByte('\n')
	}
	b.WriteString(inner)
	if ie, ok := blk.Result.(*IfExpr); ok {
		b.WriteString(ie.FormatAt(inner))
	} else {
		b.WriteString(FormatExpr(blk.Result))
	}
	b.WriteByte('\n')
}

func formatHostLet(d HostDecl, base string) string {
	if ie, ok := d.Expr.(*IfExpr); ok {
		return "let " + d.Name + ": " + d.Type + " = " + ie.FormatAt(base)
	}
	return "let " + d.Name + ": " + d.Type + " = " + FormatExpr(d.Expr)
}

// WalkIf visits every host if in e, outer before inner.
func WalkIf(e Expr, fn func(*IfExpr)) {
	if e == nil || fn == nil {
		return
	}
	switch n := e.(type) {
	case *IfExpr:
		fn(n)
		walkBlock(n.Then, fn)
		walkBlock(n.Else, fn)
	case *UnaryExpr:
		WalkIf(n.X, fn)
	case *BinaryExpr:
		WalkIf(n.L, fn)
		WalkIf(n.R, fn)
	case *ConvertExpr:
		WalkIf(n.X, fn)
	case *CallExpr:
		for _, a := range n.Args {
			WalkIf(a, fn)
		}
	case *ArrayExpr:
		for _, el := range n.Elems {
			WalkIf(el, fn)
		}
	case *IndexExpr:
		WalkIf(n.X, fn)
		WalkIf(n.Index, fn)
	}
}

func walkBlock(b *BlockExpr, fn func(*IfExpr)) {
	if b == nil {
		return
	}
	for _, d := range b.Lets {
		WalkIf(d.Expr, fn)
	}
	WalkIf(b.Result, fn)
}

func isHostIfKeyword(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 2 && s[:2] == "if" && (len(s) == 2 || !isIdentCont(s[2]))
}

func isHostIfStatement(line string) bool {
	return isHostIfKeyword(line)
}

type mlCursor struct {
	lines []srcLine
	li    int
	bi    int
}

func (c *mlCursor) eof() bool { return c.li >= len(c.lines) }

func (c *mlCursor) cur() string {
	if c.eof() {
		return ""
	}
	return c.lines[c.li].Text
}

func (c *mlCursor) line() int {
	if c.eof() {
		if len(c.lines) == 0 {
			return 1
		}
		return c.lines[len(c.lines)-1].Line
	}
	return c.lines[c.li].Line
}

func (c *mlCursor) col() int { return c.bi + 1 }

func (c *mlCursor) skip() {
	for !c.eof() {
		t := c.cur()
		for c.bi < len(t) && (t[c.bi] == ' ' || t[c.bi] == '\t') {
			c.bi++
		}
		if c.bi < len(t) {
			return
		}
		c.li++
		c.bi = 0
	}
}

func (c *mlCursor) peekKeyword(word string) bool {
	c.skip()
	if c.eof() {
		return false
	}
	t := c.cur()
	if c.bi+len(word) > len(t) || t[c.bi:c.bi+len(word)] != word {
		return false
	}
	end := c.bi + len(word)
	if end < len(t) && isIdentCont(t[end]) {
		return false
	}
	return true
}

func (c *mlCursor) parseIf(depth int) (*IfExpr, error) {
	if depth > MaxExprDepth {
		return nil, qerr.New(qerr.KindParse, qerr.Diagnostic{
			Code: qerr.CodeExprDepth, Severity: qerr.SeverityError,
			Message: "expression is nested more than 64 levels",
			Line:    c.line(), Column: c.col(), EndLine: c.line(), EndColumn: c.col() + 1,
			SuggestedFix: "Split the conditional.",
			DocsURL:      qerr.DocsHostIf,
		})
	}
	c.skip()
	line, col := c.line(), c.col()
	if !c.peekKeyword("if") {
		return nil, ifDiag(qerr.CodeSyntax, line, col, col+2, "expected host if", "Write if condition { expr } else { expr }.")
	}
	c.bi += 2
	condText, condLine, condCol, err := c.readUntil('{')
	if err != nil {
		return nil, err
	}
	condText = strings.TrimSpace(condText)
	if condText == "" {
		return nil, ifDiag(qerr.CodeSyntax, line, col, col+2, "host if is missing a condition", "Write a bool condition before {.")
	}
	cond, err := ParseExpr(condText, condLine, condCol)
	if err != nil {
		return nil, err
	}
	c.skip()
	if c.eof() || c.bi >= len(c.cur()) || c.cur()[c.bi] != '{' {
		return nil, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+1, "host if is missing {", "Open the branch with {.")
	}
	c.bi++
	thenB, err := c.parseBlock(depth)
	if err != nil {
		return nil, err
	}
	c.skip()
	if !c.peekKeyword("else") {
		return nil, ifDiag(qerr.CodeIfElse, c.line(), c.col(), c.col()+1, "conditional expression requires else", "Add else { expr }. Both branches are required. This is not uppercase QPU IF.")
	}
	c.bi += 4
	c.skip()
	if c.eof() || c.bi >= len(c.cur()) || c.cur()[c.bi] != '{' {
		return nil, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+1, "host else is missing {", "Open the else branch with {.")
	}
	c.bi++
	elseB, err := c.parseBlock(depth)
	if err != nil {
		return nil, err
	}
	return &IfExpr{Cond: cond, Then: thenB, Else: elseB, Line: line, EndLine: c.line(), Col: col, EndCol: col + 2}, nil
}

func (c *mlCursor) parseBlock(depth int) (*BlockExpr, error) {
	c.skip()
	start := c.line()
	blk := &BlockExpr{Line: start, Col: c.col()}
	for {
		c.skip()
		if c.eof() {
			return nil, ifDiag(qerr.CodeSyntax, start, 1, 2, "unclosed host if branch", "Close the branch with }.")
		}
		if c.bi < len(c.cur()) && c.cur()[c.bi] == '}' {
			c.bi++
			blk.EndLine = c.line()
			blk.EndCol = c.col()
			if blk.Result == nil {
				return nil, ifDiag(qerr.CodeIfBlock, start, 1, 2, "host if branch has no value", "End the branch with one expression. A branch is not a list of statements.")
			}
			return blk, nil
		}
		if c.peekKeyword("let") {
			if blk.Result != nil {
				return nil, ifDiag(qerr.CodeIfBlock, c.line(), c.col(), c.col()+3, "host if branch has a statement after its value", "The value is the last line of the branch.")
			}
			d, err := c.parseBlockLet(depth)
			if err != nil {
				return nil, err
			}
			blk.Lets = append(blk.Lets, d)
			continue
		}
		if c.peekKeyword("return") || c.peekKeyword("fn") {
			return nil, ifDiag(qerr.CodeIfBlock, c.line(), c.col(), c.col()+6, "a host if branch yields a value and does not use return", "Delete return. A conditional expression yields a value. Use a statement-form if when the branch should return.")
		}
		if blk.Result != nil {
			return nil, ifDiag(qerr.CodeIfBlock, c.line(), c.col(), c.col()+1, "host if branch has more than one value", "Keep a single result expression in the branch.")
		}
		blk.ResultLine = c.line()
		var err error
		if c.peekKeyword("if") {
			blk.Result, err = c.parseIf(depth + 1)
		} else {
			blk.Result, err = c.parseLineExpr()
		}
		if err != nil {
			return nil, err
		}
	}
}

func (c *mlCursor) parseBlockLet(depth int) (HostDecl, error) {
	lineNum := c.line()
	first := c.cur()[c.bi:]
	eq := strings.Index(first, "=")
	if eq >= 0 && !(eq+1 < len(first) && first[eq+1] == '=') {
		expr := strings.TrimSpace(first[eq+1:])
		if isHostIfKeyword(expr) {
			real, err := scanLetPrefix(first, lineNum)
			if err != nil {
				return HostDecl{}, err
			}
			lead := len(first[eq+1:]) - len(strings.TrimLeft(first[eq+1:], " \t"))
			c.bi += eq + 1 + lead
			ife, err := c.parseIf(depth + 1)
			if err != nil {
				return HostDecl{}, err
			}
			real.Expr = ife
			real.EndCol = ife.EndCol
			return real, nil
		}
	}
	chunk := cutAtBrace(first)
	d, err := parseLetLine(strings.TrimSpace(chunk), lineNum)
	if err != nil {
		return HostDecl{}, err
	}
	c.bi += len(chunk)
	for !c.eof() && c.bi < len(c.cur()) && (c.cur()[c.bi] == ' ' || c.cur()[c.bi] == '\t') {
		c.bi++
	}
	if !c.eof() && c.bi >= len(c.cur()) {
		c.li++
		c.bi = 0
	}
	return d, nil
}

func (c *mlCursor) parseLineExpr() (Expr, error) {
	line, col := c.line(), c.col()
	text := c.cur()[c.bi:]
	chunk := cutAtBrace(text)
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return nil, ifDiag(qerr.CodeIfBlock, line, col, col+1, "host if branch has no value", "End the branch with one expression.")
	}
	ex, err := ParseExpr(chunk, line, col)
	if err != nil {
		return nil, err
	}
	c.bi += len(cutAtBrace(text))
	return ex, nil
}

func (c *mlCursor) readUntil(target byte) (string, int, int, error) {
	startLine, startCol := c.line(), c.col()
	var b strings.Builder
	inStr, esc := false, false
	for !c.eof() {
		if c.bi >= len(c.cur()) {
			c.li++
			c.bi = 0
			if !inStr {
				b.WriteByte(' ')
			}
			continue
		}
		ch := c.cur()[c.bi]
		if inStr {
			b.WriteByte(ch)
			c.bi++
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			b.WriteByte(ch)
			c.bi++
			continue
		}
		if ch == target {
			return b.String(), startLine, startCol, nil
		}
		b.WriteByte(ch)
		c.bi++
	}
	return "", startLine, startCol, ifDiag(qerr.CodeSyntax, startLine, startCol, startCol+1, "host if is missing {", "Open the branch with {.")
}

func cutAtBrace(s string) string {
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			continue
		}
		if ch == '}' {
			return s[:i]
		}
	}
	return s
}

func scanLetPrefix(line string, lineNum int) (HostDecl, error) {
	// Parse `let name: type =` and leave the expression to the caller.
	eq := strings.Index(line, "=")
	if eq < 0 || (eq+1 < len(line) && line[eq+1] == '=') {
		return HostDecl{}, hostSyntax(lineNum, 1, len(line)+1, "let requires an initializer: let name: type = expr")
	}
	stub := strings.TrimRight(line[:eq+1], " \t") + " 0"
	d, err := parseLetLine(stub, lineNum)
	if err != nil {
		return HostDecl{}, err
	}
	d.Expr = nil
	return d, nil
}

func ifDiag(code string, line, col, end int, msg, fix string) error {
	return qerr.New(qerr.KindParse, qerr.Diagnostic{
		Code: code, Severity: qerr.SeverityError, Message: msg,
		Line: line, Column: col, EndLine: line, EndColumn: end,
		SuggestedFix: fix, DocsURL: qerr.DocsHostIf,
	})
}

func parseIfFrom(lines []srcLine, startByte, depth int) (*IfExpr, int, error) {
	c := &mlCursor{lines: lines, bi: startByte}
	ife, err := c.parseIf(depth)
	if err != nil {
		return nil, 0, err
	}
	if !c.eof() {
		rest := strings.TrimSpace(c.cur()[c.bi:])
		if rest != "" {
			return nil, 0, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+len(rest), "unexpected tokens after host if", "End the statement at the closing } of else.")
		}
	}
	n := c.li + 1
	if n < 1 {
		n = 1
	}
	if n > len(lines) {
		n = len(lines)
	}
	return ife, n, nil
}

func hostIfStatementRejected(line int) error {
	return ifDiag(qerr.CodeSyntax, line, 1, 3, "statement-form host if is only allowed inside fn",
		"Move this if into a function. A file-level value uses let name: type = if condition { expr } else { expr }. Uppercase IF is QPU dynamic control.")
}

func parseLetSpan(lines []srcLine) (HostDecl, int, error) {
	if len(lines) == 0 {
		return HostDecl{}, 0, hostSyntax(1, 1, 2, "let requires a name, a type, and an initializer")
	}
	line := lines[0].Text
	lineNum := lines[0].Line
	eq := strings.Index(line, "=")
	if eq >= 0 && !(eq+1 < len(line) && line[eq+1] == '=') && isHostIfKeyword(strings.TrimSpace(line[eq+1:])) {
		real, err := scanLetPrefix(line, lineNum)
		if err != nil {
			return HostDecl{}, 0, err
		}
		lead := len(line[eq+1:]) - len(strings.TrimLeft(line[eq+1:], " \t"))
		ife, n, err := parseIfFrom(lines, eq+1+lead, 1)
		if err != nil {
			return HostDecl{}, 0, err
		}
		real.Expr = ife
		real.EndCol = ife.EndCol
		return real, n, nil
	}
	d, err := parseLetLine(line, lineNum)
	if err != nil {
		return HostDecl{}, 0, err
	}
	return d, 1, nil
}

func parseHostLetAt(allLines []string, idx *int, line string, lineNum int) (HostDecl, error) {
	lines := []srcLine{{Line: lineNum, Text: line}}
	for j := *idx; j < len(allLines); j++ {
		raw := allLines[j]
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		lines = append(lines, srcLine{Line: j + 1, Text: strings.TrimSpace(raw)})
	}
	d, n, err := parseLetSpan(lines)
	if err != nil {
		return HostDecl{}, err
	}
	if n > 1 {
		*idx += n - 1
	}
	return d, nil
}

// TakeHostIfLet formats a file-level let whose initializer is a host if.
// n is the number of source lines consumed. ok is false for any other line.
func TakeHostIfLet(lines []string, start int) (canon string, n int, ok bool) {
	if start < 0 || start >= len(lines) {
		return "", 0, false
	}
	first := strings.TrimSpace(stripLineComment(lines[start]))
	if len(first) < 3 || !strings.EqualFold(first[:3], "let") || (len(first) > 3 && isIdentCont(first[3])) {
		return "", 0, false
	}
	eq := strings.Index(first, "=")
	if eq < 0 || (eq+1 < len(first) && first[eq+1] == '=') || !isHostIfKeyword(strings.TrimSpace(first[eq+1:])) {
		return "", 0, false
	}
	src := make([]srcLine, 0, len(lines)-start)
	for j := start; j < len(lines); j++ {
		raw := lines[j]
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		src = append(src, srcLine{Line: j + 1, Text: strings.TrimSpace(raw)})
	}
	d, consumed, err := parseLetSpan(src)
	if err != nil || consumed < 1 {
		return "", 0, false
	}
	return formatHostDecl(d), consumed, true
}

func formatHostDecl(d HostDecl) string {
	if ie, ok := d.Expr.(*IfExpr); ok {
		return "let " + d.Name + ": " + d.Type + " = " + ie.FormatAt("")
	}
	return "let " + d.Name + ": " + d.Type + " = " + FormatExpr(d.Expr)
}
