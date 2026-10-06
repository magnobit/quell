// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// Template is a circuit parsed and lowered once. Bind applies concrete
// PARAM values without parsing the source again. The template is not
// mutated, so many parameter sets can share it.
type Template struct {
	src  string
	prog *ir.Program
}

// Prepare parses and lowers src once. Call Bind for each parameter set.
func Prepare(src string) (*Template, error) {
	if src == "" {
		return &Template{src: src}, nil
	}
	circ, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	if err := check.Fail(circ); err != nil {
		return nil, err
	}
	return &Template{src: src, prog: ir.Lower(circ)}, nil
}

// Bind returns Quell source with PARAM names replaced by values.
// Semantics match BindSource. An empty source or a circuit with nothing
// left to bind returns the original source.
func (t *Template) Bind(params map[string]float64) (string, error) {
	if t == nil || t.prog == nil || !ir.NeedsBind(t.prog) {
		if t == nil {
			return "", nil
		}
		return t.src, nil
	}
	bound, err := ir.Bind(t.prog, params)
	if err != nil {
		return "", err
	}
	return ToQuell(bound), nil
}

// BindMany parses src once and binds each parameter set.
func BindMany(src string, sets []map[string]float64) ([]string, error) {
	t, err := Prepare(src)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(sets))
	for i, params := range sets {
		s, err := t.Bind(params)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// BindSource parses Quell, binds PARAM values to concrete angles, and
// raises a fixed circuit. This is QubitLabs bind-then-submit, not native
// provider symbolic parameters. Returns src unchanged when nothing is unbound.
// Repeated sweeps should use Prepare and Bind so the source is parsed once.
func BindSource(src string, params map[string]float64) (string, error) {
	t, err := Prepare(src)
	if err != nil {
		return "", err
	}
	return t.Bind(params)
}
