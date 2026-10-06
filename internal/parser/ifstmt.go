// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"

	"github.com/magnobit/quell/qerr"
)

// IfStmt is a host conditional statement. It is not an IfExpr and not QPU IF.
// Else is nil when the source has no else. An empty else block is a non-nil empty slice.
type IfStmt struct {
	Cond              Expr
	Then              []FnStmt
	Else              []FnStmt
	Line, EndLine     int
	Col, EndCol       int
	ThenLine, ThenEnd int
	ElseLine, ElseEnd int
}

func (c *mlCursor) parseIfStmt(depth int) (*IfStmt, error) {
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
		return nil, ifDiag(qerr.CodeSyntax, line, col, col+2, "expected host if", "Write if condition { statements }.")
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
	thenStmts, thenLine, thenEnd, err := c.parseStmtBlock(depth)
	if err != nil {
		return nil, err
	}
	stmt := &IfStmt{
		Cond: cond, Then: thenStmts, Line: line, Col: col, EndCol: col + 2,
		ThenLine: thenLine, ThenEnd: thenEnd,
	}
	endLine, endCol := c.line(), c.col()
	c.skip()
	if c.peekKeyword("else") {
		c.bi += 4
		c.skip()
		if c.peekKeyword("if") {
			nested, err := c.parseIfStmt(depth + 1)
			if err != nil {
				return nil, err
			}
			stmt.Else = []FnStmt{{
				Kind: "if", If: nested, Line: nested.Line, Col: nested.Col, EndCol: nested.EndCol,
			}}
			stmt.ElseLine = nested.Line
			stmt.ElseEnd = nested.EndLine
			endLine, endCol = nested.EndLine, nested.EndCol
		} else {
			if c.eof() || c.bi >= len(c.cur()) || c.cur()[c.bi] != '{' {
				return nil, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+1, "host else is missing {", "Open the else branch with {, or write else if.")
			}
			c.bi++
			elseStmts, elseLine, elseEnd, err := c.parseStmtBlock(depth)
			if err != nil {
				return nil, err
			}
			if elseStmts == nil {
				elseStmts = []FnStmt{}
			}
			stmt.Else = elseStmts
			stmt.ElseLine = elseLine
			stmt.ElseEnd = elseEnd
			endLine, endCol = c.line(), c.col()
		}
	}
	stmt.EndLine = endLine
	stmt.EndCol = endCol
	return stmt, nil
}

func (c *mlCursor) parseStmtBlock(depth int) ([]FnStmt, int, int, error) {
	c.skip()
	start := c.line()
	var stmts []FnStmt
	for {
		c.skip()
		if c.eof() {
			return nil, start, start, ifDiag(qerr.CodeSyntax, start, 1, 2, "unclosed host if branch", "Close the branch with }.")
		}
		if c.bi < len(c.cur()) && c.cur()[c.bi] == '}' {
			c.bi++
			return stmts, start, c.line(), nil
		}
		beforeLi, beforeBi := c.li, c.bi
		st, err := c.parseOneStmt(depth + 1)
		if err != nil {
			return nil, 0, 0, err
		}
		if c.li == beforeLi && c.bi == beforeBi {
			return nil, 0, 0, fnSyntax(c.line(), c.col(), c.col()+1, "empty host statement", "Write let, if, or return.")
		}
		stmts = append(stmts, st)
	}
}

func (c *mlCursor) parseOneStmt(depth int) (FnStmt, error) {
	c.skip()
	if c.eof() {
		return FnStmt{}, fnSyntax(c.line(), 1, 2, "empty host statement", "Write let, if, or return.")
	}
	if c.peekKeyword("if") {
		ifs, err := c.parseIfStmt(depth)
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "if", If: ifs, Line: ifs.Line, Col: ifs.Col, EndCol: ifs.EndCol}, nil
	}
	if c.peekKeyword("let") {
		d, err := c.parseBlockLet(depth)
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "let", Decl: d, Line: d.Line, Col: d.Col, EndCol: d.EndCol}, nil
	}
	if c.peekKeyword("return") {
		return c.parseReturnStmt()
	}
	if c.peekKeyword("for") || c.peekKeyword("while") {
		return c.parseLoopStmt(depth)
	}
	if c.peekKeyword("var") {
		line := c.line()
		text := strings.TrimSpace(cutAtBrace(c.cur()[c.bi:]))
		d, err := parseVarLine(text, line)
		if err != nil {
			return FnStmt{}, err
		}
		c.bi += len(cutAtBrace(c.cur()[c.bi:]))
		return FnStmt{Kind: "var", Decl: d, Line: line, Col: d.Col, EndCol: d.EndCol}, nil
	}
	if c.peekKeyword("break") {
		line, col := c.line(), c.col()
		c.bi += len("break")
		return FnStmt{Kind: "break", Line: line, Col: col, EndCol: col + 5}, nil
	}
	if c.peekKeyword("continue") {
		line, col := c.line(), c.col()
		c.bi += len("continue")
		return FnStmt{Kind: "continue", Line: line, Col: col, EndCol: col + 8}, nil
	}
	if c.peekKeyword("print") || c.peekKeyword("println") {
		line := c.line()
		text := strings.TrimSpace(cutAtBrace(c.cur()[c.bi:]))
		st, err := parsePrintStmt(text, line)
		if err != nil {
			return FnStmt{}, err
		}
		c.bi += len(cutAtBrace(c.cur()[c.bi:]))
		return st, nil
	}
	if c.peekKeyword("fn") {
		return FnStmt{}, fnSyntax(c.line(), c.col(), c.col()+2, "a host function cannot contain another function", "Keep each fn at program scope.")
	}
	line := c.line()
	text := c.cur()[c.bi:]
	chunk := strings.TrimSpace(cutAtBrace(text))
	if d, ok, err := parseAssignLine(chunk, line); ok {
		c.bi += len(cutAtBrace(text))
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "assign", Decl: d, Line: line, Col: d.Col, EndCol: d.EndCol}, nil
	}
	end := c.col() + len(chunk)
	if chunk == "" {
		end = c.col() + 1
	}
	return FnStmt{}, fnSyntax(line, c.col(), end, "a host function may only contain let, if, and return", "Move quantum gates out of the function. gate is a compile-time macro, not a host function.")
}

func (c *mlCursor) parseReturnStmt() (FnStmt, error) {
	line, col := c.line(), c.col()
	c.bi += len("return")
	c.skip()
	if c.eof() {
		return FnStmt{}, fnSyntax(line, col, col+6, "return requires an expression", "Return one scalar: return expr.")
	}
	if c.peekKeyword("if") {
		ife, err := c.parseIf(1)
		if err != nil {
			return FnStmt{}, err
		}
		return FnStmt{Kind: "return", Expr: ife, Line: line, Col: col, EndCol: ife.EndCol}, nil
	}
	text := c.cur()[c.bi:]
	raw := cutAtBrace(text)
	chunk := strings.TrimSpace(raw)
	if chunk == "" {
		return FnStmt{}, fnSyntax(line, col, col+6, "return requires an expression", "Return one scalar: return expr.")
	}
	ex, err := ParseExpr(chunk, c.line(), c.col())
	if err != nil {
		return FnStmt{}, err
	}
	c.bi += len(raw)
	_, end := ex.Span()
	return FnStmt{Kind: "return", Expr: ex, Line: line, Col: col, EndCol: end}, nil
}

func formatFnStmt(st FnStmt, base string) string {
	switch st.Kind {
	case "let":
		return formatFnLet(st.Decl, base)
	case "var":
		body := formatFnLet(st.Decl, base)
		if strings.HasPrefix(body, "let ") {
			return "var " + body[len("let "):]
		}
		return body
	case "return":
		if ie, ok := st.Expr.(*IfExpr); ok {
			return "return " + ie.FormatAt(base)
		}
		return "return " + FormatExpr(st.Expr)
	case "if":
		return formatIfStmt(st.If, base)
	case "for", "while":
		return formatLoop(st.Loop, base)
	case "break":
		return "break"
	case "continue":
		return "continue"
	case "call":
		return FormatExpr(st.Expr)
	case "assign":
		if st.Decl.Index != nil {
			return st.Decl.Name + "[" + FormatExpr(st.Decl.Index) + "] = " + FormatExpr(st.Decl.Expr)
		}
		return st.Decl.Name + " = " + FormatExpr(st.Decl.Expr)
	default:
		return ""
	}
}

func formatIfStmt(n *IfStmt, base string) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("if ")
	b.WriteString(FormatExpr(n.Cond))
	b.WriteString(" {\n")
	writeFnStmts(&b, n.Then, base+"    ")
	if n.Else != nil {
		b.WriteString(base)
		b.WriteString("} else {\n")
		writeFnStmts(&b, n.Else, base+"    ")
	}
	b.WriteString(base)
	b.WriteByte('}')
	return b.String()
}

func writeFnStmts(b *strings.Builder, stmts []FnStmt, inner string) {
	for _, st := range stmts {
		b.WriteString(inner)
		b.WriteString(formatFnStmt(st, inner))
		b.WriteByte('\n')
	}
}

func countFnLets(stmts []FnStmt) int {
	n := 0
	for _, st := range stmts {
		if st.Kind == "let" || st.Kind == "var" {
			n++
		}
		if st.If != nil {
			n += countFnLets(st.If.Then)
			n += countFnLets(st.If.Else)
		}
		if st.Loop != nil {
			n += countFnLets(st.Loop.Body)
		}
	}
	return n
}

// WalkFnStmts visits every statement, outer before inner.
func WalkFnStmts(stmts []FnStmt, fn func(FnStmt)) {
	for _, st := range stmts {
		fn(st)
		if st.If != nil {
			WalkFnStmts(st.If.Then, fn)
			WalkFnStmts(st.If.Else, fn)
		}
		if st.Loop != nil {
			WalkFnStmts(st.Loop.Body, fn)
		}
	}
}

// HostIfKindAt reports "expr" or "stmt" when a host if keyword starts on line.
func HostIfKindAt(c *Circuit, line int) string {
	if c == nil || line < 1 {
		return ""
	}
	kind := ""
	noteExpr := func(e Expr) {
		WalkIf(e, func(n *IfExpr) {
			if n.Line == line {
				kind = "expr"
			}
		})
	}
	for _, h := range c.Host {
		noteExpr(h.Expr)
	}
	var walk func([]FnStmt)
	walk = func(stmts []FnStmt) {
		for _, st := range stmts {
			switch st.Kind {
			case "let", "assign":
				noteExpr(st.Decl.Expr)
			case "return":
				noteExpr(st.Expr)
			case "if":
				if st.If != nil && st.If.Line == line {
					if kind != "expr" {
						kind = "stmt"
					}
				}
				if st.If != nil {
					noteExpr(st.If.Cond)
					walk(st.If.Then)
					walk(st.If.Else)
				}
			case "for", "while":
				if st.Loop != nil {
					noteExpr(st.Loop.Cond)
					noteExpr(st.Loop.From)
					noteExpr(st.Loop.To)
					walk(st.Loop.Body)
				}
			}
		}
	}
	for _, f := range c.Functions {
		walk(f.Body)
	}
	return kind
}
