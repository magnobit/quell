// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package ir defines Quell's backend-independent quantum intermediate
// representation: a flat list of gate operations lowered from the parsed
// Circuit AST. The optimizer (internal/optimizer) and the code generators
// (internal/compiler) both operate on this IR instead of talking to the
// parser AST directly.
package ir

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/parser"
)

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
	OpIF      OpKind = "IF"
)

// Op is a single operation in the IR: a gate application or a circuit
// control instruction (MEASURE, BARRIER, RESET, IF), with its target qubits
// and any float arguments (rotation angles / U-gate parameters).
type Op struct {
	Kind     OpKind
	Qubits   []int
	Args     []float64
	ArgNames []string // parallel to Args; non-empty = unbound symbolic param
	// IF: classical condition c[CondCbit]==CondEq then apply Body (one op).
	CondCbit int
	CondEq   int
	Body     *Op
}

// Program is the backend-independent representation of a compiled circuit.
type Program struct {
	NumQubits int
	Ops       []Op
	Params    []string
	// Stochastic noise for local simulation (ignored by hardware compile targets).
	NoiseDepolarizing     float64
	NoiseAmplitudeDamping float64
	NoisePhaseDamping     float64
	NoiseReadout          float64
}

// Lower converts a parsed Circuit into an IR Program.
func Lower(c *parser.Circuit) *Program {
	ops := make([]Op, 0, len(c.Instructions))
	for _, inst := range c.Instructions {
		ops = append(ops, lowerInst(inst))
	}

	nq := c.NumQubits
	if nq < 1 {
		nq = 1
	}

	params := append([]string(nil), c.Params...)
	return &Program{
		NumQubits:             nq,
		Ops:                   ops,
		Params:                params,
		NoiseDepolarizing:     c.NoiseDepolarizing,
		NoiseAmplitudeDamping: c.NoiseAmplitudeDamping,
		NoisePhaseDamping:     c.NoisePhaseDamping,
		NoiseReadout:          c.NoiseReadout,
	}
}

func lowerInst(inst parser.Instruction) Op {
	op := Op{
		Kind:     OpKind(inst.Gate),
		Qubits:   append([]int(nil), inst.Qubits...),
		Args:     append([]float64(nil), inst.Args...),
		ArgNames: append([]string(nil), inst.ArgNames...),
		CondCbit: inst.CondCbit,
		CondEq:   inst.CondEq,
	}
	if inst.Body != nil {
		body := lowerInst(*inst.Body)
		op.Body = &body
	}
	return op
}

// UnboundParams returns symbolic angle names still present in the program.
func UnboundParams(p *Program) []string {
	seen := map[string]bool{}
	var out []string
	var walk func([]Op)
	walk = func(ops []Op) {
		for _, op := range ops {
			for _, name := range op.ArgNames {
				if name != "" && !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
			}
			if op.Body != nil {
				walk([]Op{*op.Body})
			}
		}
	}
	walk(p.Ops)
	return out
}

// Bind substitutes symbolic angle parameters with concrete floats.
// Returns a deep copy with ArgNames cleared. Errors if any name is missing.
func Bind(p *Program, values map[string]float64) (*Program, error) {
	if p == nil {
		return nil, fmt.Errorf("ir: nil program")
	}
	out := &Program{
		NumQubits:             p.NumQubits,
		Params:                nil,
		NoiseDepolarizing:     p.NoiseDepolarizing,
		NoiseAmplitudeDamping: p.NoiseAmplitudeDamping,
		NoisePhaseDamping:     p.NoisePhaseDamping,
		NoiseReadout:          p.NoiseReadout,
	}
	ops, err := bindOps(p.Ops, values)
	if err != nil {
		return nil, err
	}
	out.Ops = ops
	return out, nil
}

func bindOps(ops []Op, values map[string]float64) ([]Op, error) {
	out := make([]Op, len(ops))
	for i, op := range ops {
		cp := op
		cp.Qubits = append([]int(nil), op.Qubits...)
		cp.Args = append([]float64(nil), op.Args...)
		if len(op.ArgNames) > 0 {
			names := make([]string, len(op.ArgNames))
			for j, name := range op.ArgNames {
				if name == "" {
					continue
				}
				v, ok := values[name]
				if !ok {
					// also try case-insensitive
					for k, fv := range values {
						if strings.EqualFold(k, name) {
							v, ok = fv, true
							break
						}
					}
				}
				if !ok {
					return nil, fmt.Errorf("ir: unbound parameter %q — pass --param %s=<radians>", name, name)
				}
				if j >= len(cp.Args) {
					cp.Args = append(cp.Args, 0)
				}
				cp.Args[j] = v
				names[j] = ""
			}
			cp.ArgNames = nil
		}
		if op.Body != nil {
			bodyOps, err := bindOps([]Op{*op.Body}, values)
			if err != nil {
				return nil, err
			}
			cp.Body = &bodyOps[0]
		}
		out[i] = cp
	}
	return out, nil
}

// NeedsBind reports whether the program still has symbolic angles.
func NeedsBind(p *Program) bool {
	return len(UnboundParams(p)) > 0
}
