// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/magnobit/quell/qerr"
)

// Host-expression limits. These apply to let initializers, not to gate angles.
const (
	MaxExprDepth = 64
	MaxStringLit = 4096
	MaxIdentLen  = 128
	MaxHostDecls = 1024
	MaxFunctions = 1024
	MaxFnParams  = 16
	MaxCallDepth = 32
)

// Host scalar types. These are not PARAM types and not qubit types.
const (
	TypeBool   = "bool"
	TypeInt    = "int"
	TypeFloat  = "float"
	TypeString = "string"
)

const (
	precOr      = 1
	precAnd     = 2
	precEq      = 3
	precRel     = 4
	precAdd     = 5
	precMul     = 6
	precUnary   = 7
	precPrimary = 8
)

// Expr is a host expression. Gate-angle tokens such as PI/2 are not Expr values.
type Expr interface {
	Span() (col, end int)
	Format(parentPrec int) string
}

// HostDecl is one program-scope host statement. Kind is "let" or "assign".
// Assignments are recorded so the checker can reject them. They are not IR ops.
type HostDecl struct {
	Kind   string
	Name   string
	Type   string
	Expr   Expr
	Index  Expr
	Line   int
	Col    int
	EndCol int
}

type LiteralExpr struct {
	Type   string
	Int    int64
	Float  float64
	Bool   bool
	Str    string
	Col    int
	EndCol int
}

func (e *LiteralExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *LiteralExpr) Format(int) string {
	switch e.Type {
	case TypeBool:
		if e.Bool {
			return "true"
		}
		return "false"
	case TypeInt:
		return strconv.FormatInt(e.Int, 10)
	case TypeFloat:
		s := strconv.FormatFloat(e.Float, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	default:
		return quoteString(e.Str)
	}
}

type IdentExpr struct {
	Name   string
	Col    int
	EndCol int
}

func (e *IdentExpr) Span() (int, int)  { return e.Col, e.EndCol }
func (e *IdentExpr) Format(int) string { return e.Name }

type UnaryExpr struct {
	Op     string
	X      Expr
	Col    int
	EndCol int
}

func (e *UnaryExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *UnaryExpr) Format(parent int) string {
	s := e.Op + e.X.Format(precUnary)
	if precUnary < parent {
		return "(" + s + ")"
	}
	return s
}

type BinaryExpr struct {
	Op     string
	L, R   Expr
	Prec   int
	Col    int
	EndCol int
}

func (e *BinaryExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *BinaryExpr) Format(parent int) string {
	s := e.L.Format(e.Prec) + " " + e.Op + " " + e.R.Format(e.Prec+1)
	if e.Prec < parent {
		return "(" + s + ")"
	}
	return s
}

type ConvertExpr struct {
	To     string
	X      Expr
	Col    int
	EndCol int
}

func (e *ConvertExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *ConvertExpr) Format(int) string {
	return e.To + "(" + FormatExpr(e.X) + ")"
}

// CallExpr is a host function call. Conversions such as int(...) stay ConvertExpr.
type CallExpr struct {
	Name   string
	Args   []Expr
	Col    int
	EndCol int
}

func (e *CallExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *CallExpr) Format(int) string {
	parts := make([]string, len(e.Args))
	for i, a := range e.Args {
		parts[i] = FormatExpr(a)
	}
	return e.Name + "(" + strings.Join(parts, ", ") + ")"
}

// FormatHostLine canonicalizes a let or an assignment. Other lines return false
// so the formatter can keep its existing keyword rules.
func FormatHostLine(line string) (string, bool) {
	trim := strings.TrimSpace(line)
	if len(trim) >= 3 && strings.EqualFold(trim[:3], "let") && (len(trim) == 3 || !isIdentCont(trim[3])) {
		d, err := parseLetLine(trim, 1)
		if err != nil {
			return trim, true
		}
		return "let " + d.Name + ": " + d.Type + " = " + FormatExpr(d.Expr), true
	}
	d, ok, err := parseAssignLine(trim, 1)
	if !ok {
		return "", false
	}
	if err != nil {
		return trim, true
	}
	return d.Name + " = " + FormatExpr(d.Expr), true
}

// FormatExpr prints an expression in canonical form.
func FormatExpr(e Expr) string {
	if e == nil {
		return ""
	}
	return e.Format(0)
}

func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// IsHostType reports whether s is a Phase 3 host scalar type.
func IsHostType(s string) bool { return isHostType(s) }

// IsReservedHostName reports whether s cannot be used as a let name.
func IsReservedHostName(s string) bool { return isReservedHostName(s) }

func isHostType(s string) bool {
	switch s {
	case TypeBool, TypeInt, TypeFloat, TypeString, "task":
		return true
	default:
		if _, ok := ElemType(s); ok {
			return true
		}
		return false
	}
}

func isReservedHostName(s string) bool {
	switch s {
	case "let", "fn", "return", "if", "else", "true", "false", "for", "while", "var", "break", "continue", "print", "println", "format", TypeBool, TypeInt, TypeFloat, TypeString:
		return true
	default:
		return false
	}
}

// parseLetLine parses `let name: type = expr` on one physical line.
// line is the comment-stripped source line.
func parseLetLine(line string, lineNum int) (HostDecl, error) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if len(line)-i < 3 || !strings.EqualFold(line[i:i+3], "let") || (len(line)-i > 3 && isIdentCont(line[i+3])) {
		return HostDecl{}, hostSyntax(lineNum, 1, len(line)+1, "let requires a name, a type, and an initializer")
	}
	i += 3
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || !isIdentStart(line[i]) {
		return HostDecl{}, hostSyntax(lineNum, 1, len(line)+1, "let requires a name: let name: type = expr")
	}
	name, nlen := scanIdent(line[i:])
	col := i + 1
	if len(name) > MaxIdentLen {
		return HostDecl{}, hostLimit(lineNum, col, "identifier longer than "+strconv.Itoa(MaxIdentLen)+" bytes")
	}
	if isReservedHostName(name) {
		return HostDecl{}, hostSyntax(lineNum, col, col+len(name), fmt.Sprintf("%q is reserved and cannot be a local name", name))
	}
	i += nlen
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || line[i] != ':' {
		return HostDecl{}, hostSyntax(lineNum, col, col+len(name), "let requires an explicit type: let name: int = expr")
	}
	i++
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	typ, tlen, ok := scanHostType(line[i:])
	if !ok {
		return HostDecl{}, hostSyntax(lineNum, col, col+len(name), "let type must be bool, int, float, string, task, or an array of bool, int, float, or string")
	}
	i += tlen
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || line[i] != '=' || (i+1 < len(line) && line[i+1] == '=') {
		return HostDecl{}, hostSyntax(lineNum, col, col+len(name), "let requires an initializer: let name: type = expr")
	}
	i++
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) {
		return HostDecl{}, hostSyntax(lineNum, col, len(line)+1, "let initializer is empty")
	}
	ex, err := ParseExpr(line[i:], lineNum, i+1)
	if err != nil {
		return HostDecl{}, err
	}
	_, end := ex.Span()
	return HostDecl{Kind: "let", Name: name, Type: typ, Expr: ex, Line: lineNum, Col: col, EndCol: end}, nil
}

func parseAssignLine(line string, lineNum int) (HostDecl, bool, error) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || !isIdentStart(line[i]) {
		return HostDecl{}, false, nil
	}
	name, nlen := scanIdent(line[i:])
	j := i + nlen
	for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
		j++
	}
	var index Expr
	if j < len(line) && line[j] == '[' {
		idxSrc := line[j+1:]
		endBr := strings.IndexByte(idxSrc, ']')
		if endBr < 0 {
			return HostDecl{}, true, hostSyntax(lineNum, i+1, j+2, "missing ] in index assignment")
		}
		ix, err := ParseExpr(strings.TrimSpace(idxSrc[:endBr]), lineNum, j+2)
		if err != nil {
			return HostDecl{}, true, err
		}
		index = ix
		j = j + 1 + endBr + 1
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
	}
	if j >= len(line) || line[j] != '=' || (j+1 < len(line) && line[j+1] == '=') {
		return HostDecl{}, false, nil
	}
	k := j + 1
	for k < len(line) && (line[k] == ' ' || line[k] == '\t') {
		k++
	}
	if k >= len(line) {
		return HostDecl{}, true, hostSyntax(lineNum, i+1, j+2, "assignment is missing an expression")
	}
	ex, err := ParseExpr(line[k:], lineNum, k+1)
	if err != nil {
		return HostDecl{}, true, err
	}
	_, end := ex.Span()
	return HostDecl{Kind: "assign", Name: name, Index: index, Expr: ex, Line: lineNum, Col: i + 1, EndCol: end}, true, nil
}

func scanIdent(s string) (string, int) {
	i := 0
	for i < len(s) {
		r := s[i]
		if i == 0 {
			if !isIdentStart(r) {
				break
			}
		} else if !isIdentCont(r) {
			break
		}
		i++
	}
	return s[:i], i
}

func isIdentStart(r byte) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isIdentCont(r byte) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

// ParseExpr parses one host expression. line and col are the 1-based
// location of s[0] in the original source line.
func ParseExpr(s string, line, col int) (Expr, error) {
	p := &exprParser{s: s, line: line, base: col}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i < len(p.s) {
		return nil, hostSyntax(line, p.col(), p.col()+1, fmt.Sprintf("unexpected %q in expression", p.s[p.i:]))
	}
	return e, nil
}

type exprParser struct {
	s     string
	i     int
	line  int
	base  int
	depth int
}

func (p *exprParser) col() int {
	return p.base + p.i
}

func (p *exprParser) skip() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

func (p *exprParser) enter() error {
	p.depth++
	if p.depth > MaxExprDepth {
		return qerr.New(qerr.KindParse, qerr.Diagnostic{
			Code: qerr.CodeExprDepth, Severity: qerr.SeverityError,
			Message: fmt.Sprintf("expression is nested more than %d levels", MaxExprDepth),
			Line:    p.line, Column: p.col(), EndLine: p.line, EndColumn: p.col() + 1,
			SuggestedFix: "Split the expression across more than one let.",
		})
	}
	return nil
}

func (p *exprParser) parseOr() (Expr, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		if !p.consume("||") {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		c, _ := left.Span()
		_, e := right.Span()
		left = &BinaryExpr{Op: "||", L: left, R: right, Prec: precOr, Col: c, EndCol: e}
	}
}

func (p *exprParser) parseAnd() (Expr, error) {
	left, err := p.parseEq()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		if !p.consume("&&") {
			return left, nil
		}
		right, err := p.parseEq()
		if err != nil {
			return nil, err
		}
		c, _ := left.Span()
		_, e := right.Span()
		left = &BinaryExpr{Op: "&&", L: left, R: right, Prec: precAnd, Col: c, EndCol: e}
	}
}

func (p *exprParser) parseEq() (Expr, error) {
	left, err := p.parseRel()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		op := ""
		switch {
		case p.consume("=="):
			op = "=="
		case p.consume("!="):
			op = "!="
		default:
			return left, nil
		}
		right, err := p.parseRel()
		if err != nil {
			return nil, err
		}
		c, _ := left.Span()
		_, e := right.Span()
		left = &BinaryExpr{Op: op, L: left, R: right, Prec: precEq, Col: c, EndCol: e}
	}
}

func (p *exprParser) parseRel() (Expr, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		op := ""
		switch {
		case p.consume("<="):
			op = "<="
		case p.consume(">="):
			op = ">="
		case p.consume("<"):
			op = "<"
		case p.consume(">"):
			op = ">"
		default:
			return left, nil
		}
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		c, _ := left.Span()
		_, e := right.Span()
		left = &BinaryExpr{Op: op, L: left, R: right, Prec: precRel, Col: c, EndCol: e}
	}
}

func (p *exprParser) parseAdd() (Expr, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		op := ""
		if p.peekByte('+') {
			p.i++
			op = "+"
		} else if p.peekByte('-') {
			p.i++
			op = "-"
		} else {
			return left, nil
		}
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		c, _ := left.Span()
		_, e := right.Span()
		left = &BinaryExpr{Op: op, L: left, R: right, Prec: precAdd, Col: c, EndCol: e}
	}
}

func (p *exprParser) parseMul() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		op := ""
		switch {
		case p.peekByte('*'):
			p.i++
			op = "*"
		case p.peekByte('/'):
			p.i++
			op = "/"
		case p.peekByte('%'):
			p.i++
			op = "%"
		default:
			return left, nil
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		c, _ := left.Span()
		_, e := right.Span()
		left = &BinaryExpr{Op: op, L: left, R: right, Prec: precMul, Col: c, EndCol: e}
	}
}

func (p *exprParser) parseUnary() (Expr, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	p.skip()
	start := p.col()
	if p.peekByte('!') || p.peekByte('-') {
		op := string(p.s[p.i])
		p.i++
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		_, end := x.Span()
		return &UnaryExpr{Op: op, X: x, Col: start, EndCol: end}, nil
	}
	return p.parsePostfix()
}

func (p *exprParser) parsePrimary() (Expr, error) {
	p.skip()
	if p.i >= len(p.s) {
		return nil, hostSyntax(p.line, p.col(), p.col()+1, "expression ended early")
	}
	if p.peekByte('(') {
		p.i++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skip()
		if !p.peekByte(')') {
			return nil, hostSyntax(p.line, p.col(), p.col()+1, "missing closing parenthesis")
		}
		p.i++
		return inner, nil
	}
	if p.peekByte('[') {
		return p.scanArray()
	}
	if p.peekByte('"') {
		return p.scanString()
	}
	if p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		return p.scanNumber()
	}
	if isIdentStart(p.s[p.i]) {
		start := p.col()
		name, n := scanIdent(p.s[p.i:])
		p.i += n
		if len(name) > MaxIdentLen {
			return nil, hostLimit(p.line, start, "identifier longer than "+strconv.Itoa(MaxIdentLen)+" bytes")
		}
		p.skip()
		if isHostType(name) && p.peekByte('(') {
			p.i++
			inner, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			p.skip()
			if !p.peekByte(')') {
				return nil, hostSyntax(p.line, p.col(), p.col()+1, "missing closing parenthesis in conversion")
			}
			p.i++
			_, end := inner.Span()
			return &ConvertExpr{To: name, X: inner, Col: start, EndCol: end + 1}, nil
		}
		if p.peekByte('(') {
			p.i++
			args, err := p.parseCallArgs()
			if err != nil {
				return nil, err
			}
			return &CallExpr{Name: name, Args: args, Col: start, EndCol: p.col()}, nil
		}
		switch name {
		case "true":
			return &LiteralExpr{Type: TypeBool, Bool: true, Col: start, EndCol: start + n}, nil
		case "false":
			return &LiteralExpr{Type: TypeBool, Bool: false, Col: start, EndCol: start + n}, nil
		}
		return &IdentExpr{Name: name, Col: start, EndCol: start + n}, nil
	}
	return nil, hostSyntax(p.line, p.col(), p.col()+1, fmt.Sprintf("unexpected character %q in expression", p.s[p.i:p.i+1]))
}

func (p *exprParser) parseCallArgs() ([]Expr, error) {
	var args []Expr
	p.skip()
	if p.peekByte(')') {
		p.i++
		return args, nil
	}
	for {
		if len(args) >= MaxFnParams {
			return nil, hostLimit(p.line, p.col(), fmt.Sprintf("call has more than %d arguments", MaxFnParams))
		}
		a, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		p.skip()
		if p.peekByte(',') {
			p.i++
			p.skip()
			if p.peekByte(')') {
				return nil, hostSyntax(p.line, p.col(), p.col()+1, "trailing comma in call")
			}
			continue
		}
		if p.peekByte(')') {
			p.i++
			return args, nil
		}
		return nil, hostSyntax(p.line, p.col(), p.col()+1, "missing closing parenthesis in call")
	}
}

func (p *exprParser) peekByte(b byte) bool {
	return p.i < len(p.s) && p.s[p.i] == b
}

func (p *exprParser) consume(op string) bool {
	if strings.HasPrefix(p.s[p.i:], op) {
		p.i += len(op)
		return true
	}
	return false
}

func (p *exprParser) scanNumber() (Expr, error) {
	start := p.col()
	i := p.i
	for i < len(p.s) && p.s[i] >= '0' && p.s[i] <= '9' {
		i++
	}
	floatLit := false
	if i < len(p.s) && p.s[i] == '.' {
		floatLit = true
		i++
		for i < len(p.s) && p.s[i] >= '0' && p.s[i] <= '9' {
			i++
		}
	}
	if i < len(p.s) && (p.s[i] == 'e' || p.s[i] == 'E') {
		floatLit = true
		i++
		if i < len(p.s) && (p.s[i] == '+' || p.s[i] == '-') {
			i++
		}
		digs := i
		for i < len(p.s) && p.s[i] >= '0' && p.s[i] <= '9' {
			i++
		}
		if i == digs {
			return nil, hostSyntax(p.line, start, p.base+i, "exponent is missing digits")
		}
	}
	lit := p.s[p.i:i]
	p.i = i
	end := p.base + i
	if floatLit {
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return nil, hostSyntax(p.line, start, end, "float literal is out of range")
		}
		return &LiteralExpr{Type: TypeFloat, Float: f, Col: start, EndCol: end}, nil
	}
	n, err := strconv.ParseInt(lit, 10, 64)
	if err != nil {
		return nil, hostSyntax(p.line, start, end, "integer literal is out of range")
	}
	return &LiteralExpr{Type: TypeInt, Int: n, Col: start, EndCol: end}, nil
}

func (p *exprParser) scanString() (Expr, error) {
	start := p.col()
	p.i++ // opening quote
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '"' {
			p.i++
			if b.Len() > MaxStringLit {
				return nil, hostLimit(p.line, start, fmt.Sprintf("string literal longer than %d bytes", MaxStringLit))
			}
			return &LiteralExpr{Type: TypeString, Str: b.String(), Col: start, EndCol: p.col()}, nil
		}
		if c == '\\' {
			if p.i+1 >= len(p.s) {
				break
			}
			switch p.s[p.i+1] {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				return nil, hostSyntax(p.line, p.col(), p.col()+2, "unsupported string escape")
			}
			p.i += 2
			continue
		}
		if c == '\n' || c == '\r' {
			break
		}
		r, size := utf8.DecodeRuneInString(p.s[p.i:])
		if r == utf8.RuneError && size == 1 {
			return nil, hostSyntax(p.line, p.col(), p.col()+1, "invalid UTF-8 in string")
		}
		b.WriteRune(r)
		p.i += size
	}
	return nil, hostSyntax(p.line, start, p.col(), "unterminated string")
}

func hostSyntax(line, col, end int, msg string) error {
	return qerr.New(qerr.KindParse, qerr.Diagnostic{
		Code: qerr.CodeSyntax, Severity: qerr.SeverityError, Message: msg,
		Line: line, Column: col, EndLine: line, EndColumn: end,
		SuggestedFix: "Use let name: type = expr with type bool, int, float, or string.",
	})
}

func hostLimit(line, col int, msg string) error {
	return qerr.New(qerr.KindParse, qerr.Diagnostic{
		Code: qerr.CodeLimit, Severity: qerr.SeverityError, Message: msg,
		Line: line, Column: col, EndLine: line, EndColumn: col + 1,
		SuggestedFix: "Shorten the input so it fits the host-language limit.",
	})
}

func noNestedHost(c *Circuit, lineNum int, where string) error {
	if c == nil || (len(c.Host) == 0 && len(c.Functions) == 0) {
		return nil
	}
	what := "let"
	fix := "Move the let declaration to program scope. A let inside a quantum block is not a function local."
	if len(c.Functions) > 0 {
		what = "fn"
		fix = "Move the function to program scope. Host functions are not allowed inside quantum blocks."
	}
	return qerr.New(qerr.KindParse, qerr.Diagnostic{
		Code: qerr.CodeSyntax, Severity: qerr.SeverityError,
		Message: fmt.Sprintf("%s is only allowed at program scope, not inside %s", what, where),
		Line:    lineNum, Column: 1, EndLine: lineNum, EndColumn: 2,
		SuggestedFix: fix,
	})
}
