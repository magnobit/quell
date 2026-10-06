// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package host is the classical program that sits beside quantum IR.
// Functions and globals are not ir.Op values and are not PARAM names.
package host

import "github.com/magnobit/quell/internal/parser"

// Global is one file-level immutable let.
type Global struct {
	Name string
	Type string
	Line int
}

// Param is one function parameter.
type Param struct {
	Name string
	Type string
}

// Func is one host function signature. The body stays on the parser declaration.
// Evaluation uses the checker, not this struct.
type Func struct {
	Name   string
	Params []Param
	Ret    string
	Line   int
}

// Program is the host side of a Quell program.
type Program struct {
	Globals []Global
	Funcs   []Func
}

// Build copies host declarations out of a parsed circuit.
// Quantum instructions are ignored. Import splice is already resolved by the parser.
func Build(c *parser.Circuit) Program {
	if c == nil {
		return Program{}
	}
	var g []Global
	for _, h := range c.Host {
		if h.Kind != "let" {
			continue
		}
		g = append(g, Global{Name: h.Name, Type: h.Type, Line: h.Line})
	}
	var fs []Func
	for _, fn := range c.Functions {
		f := Func{Name: fn.Name, Ret: fn.Ret, Line: fn.Line}
		for _, p := range fn.Params {
			f.Params = append(f.Params, Param{Name: p.Name, Type: p.Type})
		}
		fs = append(fs, f)
	}
	return Program{Globals: g, Funcs: fs}
}
