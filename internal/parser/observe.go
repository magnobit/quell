// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// PauliOp is one factor in an observable term. Axis is I, X, Y, or Z.
type PauliOp struct {
	Axis  string
	Qubit int
}

// ObsTerm is coeff * P0 * P1 * ...
type ObsTerm struct {
	Coeff float64
	Ops   []PauliOp
}

// ObservableDecl is a provider-neutral observable. It is not an IR opcode.
type ObservableDecl struct {
	Name  string
	Terms []ObsTerm
	Line  int
}

func parseObservable(line string, lineNum int) (ObservableDecl, error) {
	s := strings.TrimSpace(line)
	if len(s) < len("observable") || !strings.EqualFold(s[:len("observable")], "observable") {
		return ObservableDecl{}, fmt.Errorf("line %d: expected observable", lineNum)
	}
	rest := strings.TrimSpace(s[len("observable"):])
	name, nlen := scanIdent(rest)
	if name == "" || !isIdent(name) {
		return ObservableDecl{}, fmt.Errorf("line %d: observable requires a name", lineNum)
	}
	rest = strings.TrimSpace(rest[nlen:])
	if !strings.HasPrefix(rest, "=") {
		return ObservableDecl{}, fmt.Errorf("line %d: observable %s requires =", lineNum, name)
	}
	expr := strings.TrimSpace(rest[1:])
	terms, err := parseObsExpr(expr)
	if err != nil {
		return ObservableDecl{}, fmt.Errorf("line %d: %w", lineNum, err)
	}
	return ObservableDecl{Name: name, Terms: terms, Line: lineNum}, nil
}

func parseObsExpr(expr string) ([]ObsTerm, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("observable expression is empty")
	}
	var parts []string
	var signs []float64
	start := 0
	sign := 1.0
	if expr[0] == '+' || expr[0] == '-' {
		if expr[0] == '-' {
			sign = -1
		}
		start = 1
	}
	depth := 0
	for i := start; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '+', '-':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(expr[start:i]))
				signs = append(signs, sign)
				if expr[i] == '-' {
					sign = -1
				} else {
					sign = 1
				}
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	signs = append(signs, sign)
	var terms []ObsTerm
	for i, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("missing observable term")
		}
		term, err := parseObsTerm(part, signs[i])
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, nil
}

func parseObsTerm(part string, sign float64) (ObsTerm, error) {
	fields := strings.Fields(strings.ReplaceAll(part, "*", " * "))
	coeff := sign
	var ops []PauliOp
	seenCoeff := false
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "*" {
			continue
		}
		if !seenCoeff && isObsNumber(f) {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return ObsTerm{}, err
			}
			coeff *= v
			seenCoeff = true
			continue
		}
		op, err := parsePauli(f)
		if err != nil {
			return ObsTerm{}, err
		}
		ops = append(ops, op)
		seenCoeff = true
	}
	if len(ops) == 0 {
		return ObsTerm{}, fmt.Errorf("observable term %q has no Pauli", part)
	}
	return ObsTerm{Coeff: coeff, Ops: ops}, nil
}

func isObsNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return false
		}
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func parsePauli(tok string) (PauliOp, error) {
	tok = strings.Trim(tok, ", \t")
	if len(tok) < 4 || tok[1] != '(' || tok[len(tok)-1] != ')' {
		return PauliOp{}, fmt.Errorf("expected X(q), Y(q), Z(q), or I(q), got %q", tok)
	}
	axis := strings.ToUpper(tok[:1])
	switch axis {
	case "I", "X", "Y", "Z":
	default:
		return PauliOp{}, fmt.Errorf("unknown Pauli %q", axis)
	}
	q, err := strconv.Atoi(tok[2 : len(tok)-1])
	if err != nil || q < 0 {
		return PauliOp{}, fmt.Errorf("Pauli qubit must be an int index, got %q", tok)
	}
	return PauliOp{Axis: axis, Qubit: q}, nil
}
