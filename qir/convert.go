// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qir exposes the documented QIR subset as Quell source.
// This is a structural import of the subset Emit writes. It is not full
// QIR compatibility and it does not certify execution.
package qir

import (
	"strconv"
	"strings"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/qir"
)

// ToQuell lowers the supported QIR text subset to Quell source.
// Unsupported constructs return an error. Dynamic control is outside the subset.
func ToQuell(text string) (string, error) {
	prog, err := qir.Import(text)
	if err != nil {
		return "", err
	}
	return programToQuell(prog), nil
}

func programToQuell(p *ir.Program) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	for _, op := range p.Ops {
		if op.Kind == "" {
			continue
		}
		b.WriteString(string(op.Kind))
		for _, q := range op.Qubits {
			b.WriteByte(' ')
			b.WriteString(strconv.Itoa(q))
		}
		for _, a := range op.Args {
			b.WriteByte(' ')
			b.WriteString(strconv.FormatFloat(a, 'f', -1, 64))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
