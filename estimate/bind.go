// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// BindSource parses Quell, binds PARAM values to concrete angles, and
// raises a fixed circuit. This is QubitLabs bind-then-submit, not native
// provider symbolic parameters. Returns src unchanged when nothing is unbound.
func BindSource(src string, params map[string]float64) (string, error) {
	if src == "" {
		return src, nil
	}
	circ, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	prog := ir.Lower(circ)
	if !ir.NeedsBind(prog) {
		return src, nil
	}
	bound, err := ir.Bind(prog, params)
	if err != nil {
		return "", err
	}
	return ToQuell(bound), nil
}
