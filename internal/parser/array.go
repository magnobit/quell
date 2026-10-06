// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strconv"
	"strings"
)

// MaxArrayLen is the maximum number of elements in one host array.
const MaxArrayLen = 256

// ArrayExpr is a host array literal. It is not a quantum register.
type ArrayExpr struct {
	Elems       []Expr
	Col, EndCol int
}

func (p *exprParser) parsePostfix() (Expr, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		if !p.peekByte('[') {
			return e, nil
		}
		start, _ := e.Span()
		p.i++
		idx, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skip()
		if !p.peekByte(']') {
			return nil, hostSyntax(p.line, p.col(), p.col()+1, "missing ] after index")
		}
		p.i++
		_, end := idx.Span()
		e = &IndexExpr{X: e, Index: idx, Col: start, EndCol: end + 1}
	}
}

func (p *exprParser) scanArray() (Expr, error) {
	start := p.col()
	p.i++
	var elems []Expr
	p.skip()
	if p.peekByte(']') {
		p.i++
		return &ArrayExpr{Col: start, EndCol: p.col()}, nil
	}
	for {
		if len(elems) >= MaxArrayLen {
			return nil, hostLimit(p.line, start, "array literal exceeds "+strconv.Itoa(MaxArrayLen)+" elements")
		}
		el, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, el)
		p.skip()
		if p.peekByte(',') {
			p.i++
			p.skip()
			continue
		}
		if p.peekByte(']') {
			p.i++
			return &ArrayExpr{Elems: elems, Col: start, EndCol: p.col()}, nil
		}
		return nil, hostSyntax(p.line, p.col(), p.col()+1, "missing ] after array literal")
	}
}
func (e *ArrayExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *ArrayExpr) Format(int) string {
	if e == nil {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, el := range e.Elems {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(FormatExpr(el))
	}
	b.WriteByte(']')
	return b.String()
}

// IndexExpr is xs[i].
type IndexExpr struct {
	X, Index    Expr
	Col, EndCol int
}

func (e *IndexExpr) Span() (int, int) { return e.Col, e.EndCol }
func (e *IndexExpr) Format(int) string {
	if e == nil {
		return ""
	}
	return FormatExpr(e.X) + "[" + FormatExpr(e.Index) + "]"
}

// ArrayType reports the array type of a scalar, or "" if typ is not a scalar.
func ArrayType(scalar string) string {
	if isScalarType(scalar) {
		return scalar + "[]"
	}
	return ""
}

// ElemType reports the element type of an array type.
func ElemType(typ string) (string, bool) {
	if strings.HasSuffix(typ, "[]") {
		elem := strings.TrimSuffix(typ, "[]")
		if isScalarType(elem) {
			return elem, true
		}
	}
	return "", false
}

func isScalarType(s string) bool {
	switch s {
	case TypeBool, TypeInt, TypeFloat, TypeString:
		return true
	default:
		return false
	}
}

// scanHostType reads bool, int, float, string, or those with a [] suffix.
func scanHostType(s string) (string, int, bool) {
	typ, n := scanIdent(s)
	typ = strings.ToLower(typ)
	if typ == "task" {
		return "task", n, true
	}
	if !isScalarType(typ) {
		return "", 0, false
	}
	i := n
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i+1 < len(s) && s[i] == '[' && s[i+1] == ']' {
		return typ + "[]", i + 2, true
	}
	return typ, n, true
}
