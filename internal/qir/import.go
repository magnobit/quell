// Copyright 2026 Magnobit, Inc. All rights reserved.

package qir

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/magnobit/quell/internal/ir"
)

// Import reads the LLVM subset Emit writes and rebuilds canonical ops.
// Initialization, branches, and output recording are structural and are not
// ops. Any other quantum call is rejected. Dynamic control is not in this subset.
func Import(text string) (*ir.Program, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("qir import: empty module")
	}
	var ops []ir.Op
	maxQ := -1
	note := func(q int) {
		if q > maxQ {
			maxQ = q
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if skipLine(line) {
			continue
		}
		name, ok := callName(line)
		if !ok {
			if strings.Contains(line, "call ") {
				return nil, fmt.Errorf("qir import: unsupported line %q", line)
			}
			continue
		}
		if isRuntime(name) {
			continue
		}
		ids := quantumPtrs(line)
		switch name {
		case "__quantum__qis__h__body":
			ops = append(ops, one(ir.OpH, ids))
		case "__quantum__qis__x__body":
			ops = append(ops, one(ir.OpX, ids))
		case "__quantum__qis__y__body":
			ops = append(ops, one(ir.OpY, ids))
		case "__quantum__qis__z__body":
			ops = append(ops, one(ir.OpZ, ids))
		case "__quantum__qis__s__body":
			ops = append(ops, one(ir.OpS, ids))
		case "__quantum__qis__t__body":
			ops = append(ops, one(ir.OpT, ids))
		case "__quantum__qis__s__adj":
			ops = append(ops, one(ir.OpSDG, ids))
		case "__quantum__qis__t__adj":
			ops = append(ops, one(ir.OpTDG, ids))
		case "__quantum__qis__rx__body":
			ops = append(ops, rot(ir.OpRX, line, ids))
		case "__quantum__qis__ry__body":
			ops = append(ops, rot(ir.OpRY, line, ids))
		case "__quantum__qis__rz__body":
			ops = append(ops, rot(ir.OpRZ, line, ids))
		case "__quantum__qis__cnot__body":
			ops = append(ops, two(ir.OpCNOT, ids))
		case "__quantum__qis__cz__body":
			ops = append(ops, two(ir.OpCZ, ids))
		case "__quantum__qis__swap__body":
			ops = append(ops, two(ir.OpSWAP, ids))
		case "__quantum__qis__mz__body":
			ops = append(ops, one(ir.OpMEASURE, ids))
		default:
			return nil, fmt.Errorf("qir import: unsupported call %s", name)
		}
		if len(ops) > 0 {
			for _, q := range ops[len(ops)-1].Qubits {
				note(q)
			}
		}
	}
	n := maxQ + 1
	if n < 1 {
		n = 1
	}
	return &ir.Program{NumQubits: n, Ops: ops}, nil
}

func skipLine(line string) bool {
	if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "declare ") || strings.HasPrefix(line, "define ") || strings.HasPrefix(line, "source_") || strings.HasPrefix(line, "attributes ") || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "@") {
		return true
	}
	switch line {
	case "entry:", "body:", "measurements:", "output:", "}":
		return true
	}
	if strings.HasPrefix(line, "ret ") || strings.HasPrefix(line, "br ") {
		return true
	}
	return false
}

func callName(line string) (string, bool) {
	i := strings.Index(line, "@__quantum__")
	if i < 0 {
		return "", false
	}
	rest := line[i+1:]
	j := strings.IndexByte(rest, '(')
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

func isRuntime(name string) bool {
	switch name {
	case "__quantum__rt__initialize", "__quantum__rt__tuple_record_output", "__quantum__rt__result_record_output":
		return true
	default:
		return false
	}
}

func quantumPtrs(line string) []int {
	i := strings.Index(line, "(")
	if i < 0 {
		return nil
	}
	inside := strings.TrimSuffix(line[i+1:], ")")
	var ids []int
	for _, part := range splitArgs(inside) {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "double ") {
			continue
		}
		if id, ok := ptrID(part); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func splitArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func ptrID(s string) (int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "ptr writeonly ")
	s = strings.TrimPrefix(s, "ptr ")
	if s == "null" {
		return 0, true
	}
	const prefix = "inttoptr (i64 "
	const suffix = " to ptr)"
	if strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix) {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(s, prefix), suffix))
		return n, err == nil
	}
	return 0, false
}

func one(k ir.OpKind, ids []int) ir.Op {
	q := 0
	if len(ids) > 0 {
		q = ids[0]
	}
	return ir.Op{Kind: k, Qubits: []int{q}}
}

func two(k ir.OpKind, ids []int) ir.Op {
	q0, q1 := 0, 0
	if len(ids) > 0 {
		q0 = ids[0]
	}
	if len(ids) > 1 {
		q1 = ids[1]
	}
	return ir.Op{Kind: k, Qubits: []int{q0, q1}}
}

func rot(k ir.OpKind, line string, ids []int) ir.Op {
	op := one(k, ids)
	if i := strings.Index(line, "double "); i >= 0 {
		rest := line[i+len("double "):]
		if j := strings.Index(rest, ","); j >= 0 {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest[:j]), 64)
			if err == nil {
				op.Args = []float64{v}
			}
		}
	}
	return op
}

// Assemble runs llvm-as on the module. A missing tool is an error so a
// skipped assembler cannot be reported as a successful profile check.
func Assemble(text string) error {
	bin, err := exec.LookPath("llvm-as")
	if err != nil {
		return fmt.Errorf("llvm-as is not installed")
	}
	cmd := exec.Command(bin, "-o", "-", "-")
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("llvm-as: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if len(out) == 0 {
		return fmt.Errorf("llvm-as produced no bitcode")
	}
	return nil
}
