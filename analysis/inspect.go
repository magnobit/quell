// Copyright 2026 Magnobit, Inc. All rights reserved.

package analysis

import (
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/qir"
)

// Canonical is the quell-ir-v1 text of the unbound program.
func (m *Model) Canonical() string {
	return string(ir.CanonicalBytes(m.prog))
}

// CanonicalOptimized is the IR after the conservative optimizer.
func (m *Model) CanonicalOptimized() string {
	opt, _ := optimizer.Optimize(m.prog)
	return string(ir.CanonicalBytes(opt))
}

// QIR emits the documented base-profile subset. Dynamic control and RESET
// are errors. This is not qir-runner execution.
func (m *Model) QIR() (string, error) {
	return qir.Emit(m.prog)
}

// Summary is the host-function and instruction counts of the parsed source.
func (m *Model) Summary() (funcs, gates int) {
	return len(m.circ.Functions), len(m.circ.Instructions)
}
