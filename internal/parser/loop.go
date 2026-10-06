// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"

	"github.com/magnobit/quell/qerr"
)

// MaxHostLoopIters caps one host for/while so a mistake cannot spin forever.
const MaxHostLoopIters = 10000

// LoopStmt is a host runtime loop. It is not compile-time FOR and not QPU WHILE.
type LoopStmt struct {
	Kind              string // "for" or "while"
	Name              string
	From, To          Expr
	Cond              Expr
	Body              []FnStmt
	Line, EndLine     int
	Col, EndCol       int
	BodyLine, BodyEnd int
}

// hostOnlyWord reports an exact lowercase host keyword that must not be
// parsed as a gate or as uppercase QPU/compile-time control.
func hostOnlyWord(tok string) (string, bool) {
	switch tok {
	case "for", "while", "var", "print", "println", "break", "continue":
		return tok, true
	default:
		return "", false
	}
}

func hostOnlyRejected(line int, word string) error {
	msg := "host " + word + " is only allowed inside fn"
	fix := "Move it into a host function. Uppercase FOR unrolls at parse time. Uppercase WHILE is QPU dynamic control."
	switch word {
	case "var":
		fix = "Move var into a host function. let stays immutable."
	case "print", "println":
		fix = "Move print into a host function. Quantum gates and QPU IF cannot print."
	case "break", "continue":
		fix = "break and continue apply only to host for and while."
	}
	return ifDiag(qerr.CodeSyntax, line, 1, 1+len(word), msg, fix)
}

func (c *mlCursor) parseLoopStmt(depth int) (FnStmt, error) {
	if depth > MaxExprDepth {
		return FnStmt{}, qerr.New(qerr.KindParse, qerr.Diagnostic{
			Code: qerr.CodeExprDepth, Severity: qerr.SeverityError,
			Message: "expression is nested more than 64 levels",
			Line:    c.line(), Column: c.col(), EndLine: c.line(), EndColumn: c.col() + 1,
			SuggestedFix: "Split the loop.",
			DocsURL:      qerr.DocsHostLoops,
		})
	}
	c.skip()
	line, col := c.line(), c.col()
	kind := ""
	if c.peekKeyword("for") {
		kind = "for"
	} else if c.peekKeyword("while") {
		kind = "while"
	} else {
		return FnStmt{}, ifDiag(qerr.CodeSyntax, line, col, col+1, "expected host for or while", "Write for name in start..end { ... } or while condition { ... }.")
	}
	c.bi += len(kind)
	loop := &LoopStmt{Kind: kind, Line: line, Col: col}
	if kind == "for" {
		c.skip()
		name, n := scanIdent(c.cur()[c.bi:])
		if name == "" {
			return FnStmt{}, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+1, "host for is missing an index name", "Write for i in 0..10 { ... }.")
		}
		c.bi += n
		loop.Name = name
		c.skip()
		if !c.peekKeyword("in") {
			return FnStmt{}, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+2, "host for is missing in", "Write for i in 0..10 { ... }.")
		}
		c.bi += 2
		rangeText, rangeLine, rangeCol, err := c.readUntil('{')
		if err != nil {
			return FnStmt{}, err
		}
		parts := strings.Split(rangeText, "..")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return FnStmt{}, ifDiag(qerr.CodeSyntax, rangeLine, rangeCol, rangeCol+len(strings.TrimSpace(rangeText)), "host for range must be start..end", "Use an inclusive int range, for example 0..10.")
		}
		loop.From, err = ParseExpr(strings.TrimSpace(parts[0]), rangeLine, rangeCol)
		if err != nil {
			return FnStmt{}, err
		}
		loop.To, err = ParseExpr(strings.TrimSpace(parts[1]), rangeLine, rangeCol+len(parts[0])+2)
		if err != nil {
			return FnStmt{}, err
		}
	} else {
		condText, condLine, condCol, err := c.readUntil('{')
		if err != nil {
			return FnStmt{}, err
		}
		condText = strings.TrimSpace(condText)
		if condText == "" {
			return FnStmt{}, ifDiag(qerr.CodeSyntax, line, col, col+5, "host while is missing a condition", "Write while condition { ... }.")
		}
		loop.Cond, err = ParseExpr(condText, condLine, condCol)
		if err != nil {
			return FnStmt{}, err
		}
	}
	c.skip()
	if c.eof() || c.bi >= len(c.cur()) || c.cur()[c.bi] != '{' {
		return FnStmt{}, ifDiag(qerr.CodeSyntax, c.line(), c.col(), c.col()+1, "host loop is missing {", "Open the loop body with {.")
	}
	c.bi++
	body, bodyLine, bodyEnd, err := c.parseStmtBlock(depth)
	if err != nil {
		return FnStmt{}, err
	}
	loop.Body = body
	loop.BodyLine = bodyLine
	loop.BodyEnd = bodyEnd
	loop.EndLine = c.line()
	loop.EndCol = c.col()
	return FnStmt{Kind: kind, Loop: loop, Line: line, Col: col, EndCol: col + len(kind)}, nil
}

func parseVarLine(line string, lineNum int) (HostDecl, error) {
	trim := strings.TrimSpace(line)
	if len(trim) < 3 || trim[:3] != "var" || (len(trim) > 3 && isIdentCont(trim[3])) {
		return HostDecl{}, hostSyntax(lineNum, 1, len(line)+1, "var requires a name, a type, and an initializer")
	}
	d, err := parseLetLine("let"+trim[3:], lineNum)
	if err != nil {
		return HostDecl{}, err
	}
	d.Kind = "var"
	return d, nil
}

func parsePrintStmt(line string, lineNum int) (FnStmt, error) {
	trim := strings.TrimSpace(line)
	ex, err := ParseExpr(trim, lineNum, 1)
	if err != nil {
		return FnStmt{}, err
	}
	call, ok := ex.(*CallExpr)
	if !ok || (call.Name != "print" && call.Name != "println") {
		return FnStmt{}, fnSyntax(lineNum, 1, len(trim)+1, "print and println are host calls", "Write println(\"text\") or print(\"text\").")
	}
	_, end := call.Span()
	return FnStmt{Kind: "call", Expr: call, Line: lineNum, Col: 1, EndCol: end}, nil
}

func formatLoop(n *LoopStmt, base string) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	if n.Kind == "for" {
		b.WriteString("for ")
		b.WriteString(n.Name)
		b.WriteString(" in ")
		b.WriteString(FormatExpr(n.From))
		b.WriteString("..")
		b.WriteString(FormatExpr(n.To))
	} else {
		b.WriteString("while ")
		b.WriteString(FormatExpr(n.Cond))
	}
	b.WriteString(" {\n")
	writeFnStmts(&b, n.Body, base+"    ")
	b.WriteString(base)
	b.WriteByte('}')
	return b.String()
}
