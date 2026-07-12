// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package ir defines Quell's backend-independent quantum intermediate
// representation: a flat list of gate operations lowered from the parsed
// Circuit AST. The optimizer (internal/optimizer) and the code generators
// (internal/compiler) both operate on this IR instead of talking to the
// parser AST directly.
package ir

import "github.com/magnobit/quell/internal/parser"

// OpKind identifies a gate or circuit-control operation. Values match the
// uppercase gate names accepted by the parser (see parser.gateArity).
type OpKind string

const (
	OpH       OpKind = "H"
	OpX       OpKind = "X"
	OpY       OpKind = "Y"
	OpZ       OpKind = "Z"
	OpS       OpKind = "S"
	OpT       OpKind = "T"
	OpSDG     OpKind = "SDG"
	OpTDG     OpKind = "TDG"
	OpSX      OpKind = "SX"
	OpRX      OpKind = "RX"
	OpRY      OpKind = "RY"
	OpRZ      OpKind = "RZ"
	OpP       OpKind = "P"
	OpU       OpKind = "U"
	OpCNOT    OpKind = "CNOT"
	OpCZ      OpKind = "CZ"
	OpSWAP    OpKind = "SWAP"
	OpISWAP   OpKind = "ISWAP"
	OpCRX     OpKind = "CRX"
	OpCRY     OpKind = "CRY"
	OpCRZ     OpKind = "CRZ"
	OpCCX     OpKind = "CCX"
	OpCSWAP   OpKind = "CSWAP"
	OpMEASURE OpKind = "MEASURE"
	OpBARRIER OpKind = "BARRIER"
	OpRESET   OpKind = "RESET"
)

// Op is a single operation in the IR: a gate application or a circuit
// control instruction (MEASURE, BARRIER, RESET), with its target qubits
// and any float arguments (rotation angles / U-gate parameters).
type Op struct {
	Kind   OpKind
	Qubits []int
	Args   []float64
}

// Program is the backend-independent representation of a compiled circuit.
type Program struct {
	NumQubits int
	Ops       []Op
}

// Lower converts a parsed Circuit into an IR Program. It is a straightforward
// 1:1 translation — each parser.Instruction becomes one ir.Op, in order —
// with no optimization applied. Run the result through optimizer.Optimize
// before code generation if optimization is desired.
func Lower(c *parser.Circuit) *Program {
	ops := make([]Op, len(c.Instructions))
	for i, inst := range c.Instructions {
		var qubits []int
		if len(inst.Qubits) > 0 {
			qubits = append([]int(nil), inst.Qubits...)
		}
		var args []float64
		if len(inst.Args) > 0 {
			args = append([]float64(nil), inst.Args...)
		}
		ops[i] = Op{
			Kind:   OpKind(inst.Gate),
			Qubits: qubits,
			Args:   args,
		}
	}

	nq := c.NumQubits
	if nq < 1 {
		nq = 1
	}

	return &Program{NumQubits: nq, Ops: ops}
}
