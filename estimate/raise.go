// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/ir"
)

// ToQuell raises an IR program back to Quell source (one gate per line).
func ToQuell(p *ir.Program) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	for _, name := range p.Params {
		fmt.Fprintf(&b, "PARAM %s\n", name)
	}
	if p.NoiseDepolarizing > 0 {
		fmt.Fprintf(&b, "NOISE depolarizing %g\n", p.NoiseDepolarizing)
	}
	if p.NoiseAmplitudeDamping > 0 {
		fmt.Fprintf(&b, "NOISE amplitude_damping %g\n", p.NoiseAmplitudeDamping)
	}
	if p.NoisePhaseDamping > 0 {
		fmt.Fprintf(&b, "NOISE phase_damping %g\n", p.NoisePhaseDamping)
	}
	if p.NoiseReadout > 0 {
		fmt.Fprintf(&b, "NOISE readout %g\n", p.NoiseReadout)
	}
	for _, op := range p.Ops {
		b.WriteString(formatOp(op))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatOp(op ir.Op) string {
	if op.Kind == ir.OpIF {
		body := "X 0"
		if op.Body != nil {
			body = formatOp(*op.Body)
		}
		return fmt.Sprintf("IF c[%d]==%d %s", op.CondCbit, op.CondEq, body)
	}
	parts := []string{string(op.Kind)}
	for i, a := range op.Args {
		if i < len(op.ArgNames) && op.ArgNames[i] != "" {
			parts = append(parts, op.ArgNames[i])
		} else {
			parts = append(parts, fmt.Sprintf("%g", a))
		}
	}
	for _, q := range op.Qubits {
		parts = append(parts, fmt.Sprintf("%d", q))
	}
	return strings.Join(parts, " ")
}
