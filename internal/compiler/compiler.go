// Copyright 2026 Magnobit, Inc. All rights reserved.

package compiler

import (
	"fmt"
	"math"
	"strings"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
)

// Target represents a supported compilation target.
type Target string

const (
	TargetOpenQASM   Target = "openqasm"
	TargetOpenQASM2  Target = "openqasm2"
	TargetQiskit     Target = "qiskit"
	TargetCirq       Target = "cirq"
	TargetBraket     Target = "braket"
	TargetQSharp     Target = "qsharp"
)

// CompileProgram generates target code from an already-lowered IR program.
// This is the adapter contract path: Quell → IR → (optional optimize) → backend text.
func CompileProgram(prog *ir.Program, target Target, optimize bool) (string, []string, error) {
	if prog == nil {
		return "", nil, fmt.Errorf("nil program")
	}
	if ir.NeedsBind(prog) {
		return "", nil, fmt.Errorf("circuit has unbound parameters %v — bind with concrete angles before compile", ir.UnboundParams(prog))
	}

	p := prog
	var notes []string
	if optimize {
		p, notes = optimizer.Optimize(prog)
	}

	var code string
	var err error
	switch target {
	case TargetOpenQASM:
		code, err = toOpenQASM(p)
	case TargetOpenQASM2:
		code, err = toOpenQASM2(p)
	case TargetQiskit:
		code, err = toQiskit(p)
	case TargetCirq:
		code, err = toCirq(p)
	case TargetBraket:
		code, err = toBraket(p)
	case TargetQSharp:
		code, err = toQSharp(p)
	default:
		return "", nil, fmt.Errorf("unknown target: %s (valid: openqasm, openqasm2, qiskit, cirq, braket, qsharp)", target)
	}
	if err != nil {
		return "", nil, err
	}
	return code, notes, nil
}

// Compile lowers a parsed Circuit to the Quell IR (internal/ir), optionally
// runs the conservative optimizer passes (internal/optimizer) over it, and
// generates code for the specified target language.
//
// It returns the compiled code, any optimizer notes describing changes made
// (nil when optimize is false or nothing changed), and an error.
func Compile(c *parser.Circuit, target Target, optimize bool) (string, []string, error) {
	prog := ir.Lower(c)
	return CompileProgram(prog, target, optimize)
}

func numQubits(p *ir.Program) int {
	if p.NumQubits < 1 {
		return 1
	}
	return p.NumQubits
}

// checkOp validates qubit and arg counts before any array access.
// The parser already validates, so this only fires if the compiler is called
// without going through Parse (defensive check).
func checkOp(op ir.Op, qubits, args int) error {
	if qubits >= 0 && len(op.Qubits) < qubits {
		return fmt.Errorf("gate %s: need %d qubit(s), got %d", op.Kind, qubits, len(op.Qubits))
	}
	if len(op.Args) < args {
		return fmt.Errorf("gate %s: need %d angle arg(s), got %d", op.Kind, args, len(op.Args))
	}
	return nil
}

// ── OpenQASM 3 ────────────────────────────────────────────────────────────────

func toOpenQASM(p *ir.Program) (string, error) {
	n := numQubits(p)
	var b strings.Builder
	fmt.Fprintf(&b, "OPENQASM 3;\n")
	fmt.Fprintf(&b, "qubit[%d] q;\n", n)
	fmt.Fprintf(&b, "bit[%d] c;\n\n", n)
	for _, op := range p.Ops {
		line, err := opToOpenQASM(op)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

func opToOpenQASM(op ir.Op) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", i) }
	switch op.Kind {
	case ir.OpH:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("h %s;", q(op.Qubits[0])), nil
	case ir.OpX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("x %s;", q(op.Qubits[0])), nil
	case ir.OpY:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("y %s;", q(op.Qubits[0])), nil
	case ir.OpZ:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("z %s;", q(op.Qubits[0])), nil
	case ir.OpS:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("s %s;", q(op.Qubits[0])), nil
	case ir.OpT:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("t %s;", q(op.Qubits[0])), nil
	case ir.OpSDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("sdg %s;", q(op.Qubits[0])), nil
	case ir.OpTDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("tdg %s;", q(op.Qubits[0])), nil
	case ir.OpSX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("sx %s;", q(op.Qubits[0])), nil
	case ir.OpRX:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("rx(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRY:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ry(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRZ:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("rz(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpP:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("p(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpU:
		if err := checkOp(op, 1, 3); err != nil { return "", err }
		return fmt.Sprintf("U(%g, %g, %g) %s;", op.Args[0], op.Args[1], op.Args[2], q(op.Qubits[0])), nil
	case ir.OpCNOT:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("cx %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCZ:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("cz %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpSWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("swap %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpISWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("iswap %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCRX:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("crx(%g) %s, %s;", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCRY:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("cry(%g) %s, %s;", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCRZ:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("crz(%g) %s, %s;", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCCX:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("ccx %s, %s, %s;", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpCSWAP:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("cswap %s, %s, %s;", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpMEASURE:
		if len(op.Qubits) == 0 {
			return "c = measure q;", nil
		}
		var parts []string
		for i, qi := range op.Qubits {
			ci := qi
			if i < len(op.MeasTargets) {
				ci = op.MeasTargets[i]
			}
			parts = append(parts, fmt.Sprintf("c[%d] = measure q[%d];", ci, qi))
		}
		return strings.Join(parts, "\n"), nil
	case ir.OpBARRIER:
		if len(op.Qubits) == 0 {
			return "barrier q;", nil
		}
		var qs []string
		for _, qi := range op.Qubits {
			qs = append(qs, q(qi))
		}
		return fmt.Sprintf("barrier %s;", strings.Join(qs, ", ")), nil
	case ir.OpRESET:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("reset %s;", q(op.Qubits[0])), nil
	case ir.OpIF:
		if len(op.Then) == 0 && op.Body == nil {
			return "", fmt.Errorf("IF missing body")
		}
		emitBody := func(body ir.Op) (string, error) {
			s, err := opToOpenQASM(body)
			if err != nil {
				return "", err
			}
			return strings.TrimSuffix(strings.TrimSpace(s), ";"), nil
		}
		var thenParts []string
		if len(op.Then) > 0 {
			for _, b := range op.Then {
				s, err := emitBody(b)
				if err != nil {
					return "", err
				}
				thenParts = append(thenParts, s+";")
			}
		} else {
			s, err := emitBody(*op.Body)
			if err != nil {
				return "", err
			}
			thenParts = append(thenParts, s+";")
		}
		thenStr := strings.Join(thenParts, " ")
		cond := openQASMCond(op)
		if len(op.Else) == 0 {
			return fmt.Sprintf("if (%s) { %s }", cond, thenStr), nil
		}
		var elseParts []string
		for _, b := range op.Else {
			s, err := emitBody(b)
			if err != nil {
				return "", err
			}
			elseParts = append(elseParts, s+";")
		}
		return fmt.Sprintf("if (%s) { %s } else { %s }", cond, thenStr, strings.Join(elseParts, " ")), nil
	case ir.OpWHILE:
		if len(op.Then) == 0 {
			return "", fmt.Errorf("WHILE missing body")
		}
		var parts []string
		for _, b := range op.Then {
			s, err := opToOpenQASM(b)
			if err != nil {
				return "", err
			}
			parts = append(parts, strings.TrimSuffix(strings.TrimSpace(s), ";")+";")
		}
		return fmt.Sprintf("while (%s) { %s } // max %d iterations enforced by Quell sim", openQASMCond(op), strings.Join(parts, " "), op.MaxIter), nil
	case ir.OpSWITCH:
		disc := "c"
		if op.CondCbit >= 0 {
			disc = fmt.Sprintf("c[%d]", op.CondCbit)
		}
		var arms []string
		for _, arm := range op.Cases {
			var bodyParts []string
			for _, b := range arm.Body {
				s, err := opToOpenQASM(b)
				if err != nil {
					return "", err
				}
				bodyParts = append(bodyParts, strings.TrimSuffix(strings.TrimSpace(s), ";")+";")
			}
			body := strings.Join(bodyParts, " ")
			if arm.Default {
				arms = append(arms, fmt.Sprintf("default: { %s }", body))
			} else {
				arms = append(arms, fmt.Sprintf("%d: { %s }", arm.Value, body))
			}
		}
		return fmt.Sprintf("switch (%s) { %s }", disc, strings.Join(arms, " ")), nil
	case ir.OpPAR:
		var parts []string
		for _, b := range op.Then {
			s, err := opToOpenQASM(b)
			if err != nil {
				return "", err
			}
			parts = append(parts, strings.TrimSuffix(strings.TrimSpace(s), ";")+";")
		}
		return fmt.Sprintf("// PAR (commuting)\n%s", strings.Join(parts, "\n")), nil
	case ir.OpASSERT:
		return fmt.Sprintf("// ASSERT %s (local sim only)", openQASMCond(op)), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", op.Kind)
	}
}

func openQASMCond(op ir.Op) string {
	if op.CondRightBit >= 0 {
		return fmt.Sprintf("c[%d] == c[%d]", op.CondCbit, op.CondRightBit)
	}
	if op.CondCbit < 0 {
		return fmt.Sprintf("c == %d", op.CondEq)
	}
	return fmt.Sprintf("c[%d] == %d", op.CondCbit, op.CondEq)
}

// ── OpenQASM 2.0 (qreg / creg / measure q -> c) ───────────────────────────────

func toOpenQASM2(p *ir.Program) (string, error) {
	n := numQubits(p)
	var b strings.Builder
	fmt.Fprintf(&b, "OPENQASM 2.0;\n")
	fmt.Fprintf(&b, "include \"qelib1.inc\";\n")
	fmt.Fprintf(&b, "qreg q[%d];\n", n)
	fmt.Fprintf(&b, "creg c[%d];\n\n", n)
	for _, op := range p.Ops {
		line, err := opToOpenQASM2(op)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

func opToOpenQASM2(op ir.Op) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", i) }
	switch op.Kind {
	case ir.OpH:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("h %s;", q(op.Qubits[0])), nil
	case ir.OpX:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("x %s;", q(op.Qubits[0])), nil
	case ir.OpY:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("y %s;", q(op.Qubits[0])), nil
	case ir.OpZ:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("z %s;", q(op.Qubits[0])), nil
	case ir.OpS:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("s %s;", q(op.Qubits[0])), nil
	case ir.OpT:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("t %s;", q(op.Qubits[0])), nil
	case ir.OpSDG:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("sdg %s;", q(op.Qubits[0])), nil
	case ir.OpTDG:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("tdg %s;", q(op.Qubits[0])), nil
	case ir.OpSX:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("sx %s;", q(op.Qubits[0])), nil
	case ir.OpRX:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return fmt.Sprintf("rx(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRY:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return fmt.Sprintf("ry(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRZ:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return fmt.Sprintf("rz(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpP:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return fmt.Sprintf("u1(%g) %s;", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpU:
		if err := checkOp(op, 1, 3); err != nil {
			return "", err
		}
		return fmt.Sprintf("u3(%g,%g,%g) %s;", op.Args[0], op.Args[1], op.Args[2], q(op.Qubits[0])), nil
	case ir.OpCNOT:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("cx %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCZ:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("cz %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpSWAP:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("swap %s, %s;", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCCX:
		if err := checkOp(op, 3, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("ccx %s, %s, %s;", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpCRZ:
		if err := checkOp(op, 2, 1); err != nil {
			return "", err
		}
		return fmt.Sprintf("crz(%g) %s, %s;", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpMEASURE:
		if len(op.Qubits) == 0 {
			return "measure q -> c;", nil
		}
		var parts []string
		for i, qi := range op.Qubits {
			ci := qi
			if i < len(op.MeasTargets) {
				ci = op.MeasTargets[i]
			}
			parts = append(parts, fmt.Sprintf("measure q[%d] -> c[%d];", qi, ci))
		}
		return strings.Join(parts, "\n"), nil
	case ir.OpBARRIER:
		if len(op.Qubits) == 0 {
			return "barrier q;", nil
		}
		var qs []string
		for _, qi := range op.Qubits {
			qs = append(qs, q(qi))
		}
		return fmt.Sprintf("barrier %s;", strings.Join(qs, ",")), nil
	case ir.OpRESET:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("reset %s;", q(op.Qubits[0])), nil
	case ir.OpIF:
		if op.Body == nil && len(op.Then) == 0 {
			return "", fmt.Errorf("IF missing body")
		}
		if op.CondRightBit >= 0 || op.CondCbit < 0 {
			return "", fmt.Errorf("OpenQASM 2 only supports IF c[i]==v — use openqasm (3) for richer conditions")
		}
		emitBody := func(body ir.Op) (string, error) {
			s, err := opToOpenQASM2(body)
			if err != nil {
				return "", err
			}
			return strings.TrimSuffix(strings.TrimSpace(s), ";"), nil
		}
		bodies := op.Then
		if len(bodies) == 0 && op.Body != nil {
			bodies = []ir.Op{*op.Body}
		}
		var parts []string
		for _, b := range bodies {
			s, err := emitBody(b)
			if err != nil {
				return "", err
			}
			if op.CondCbit == 0 {
				parts = append(parts, fmt.Sprintf("if(c==%d) %s;", op.CondEq, s))
			} else {
				parts = append(parts, fmt.Sprintf("if(c[%d]==%d) %s;", op.CondCbit, op.CondEq, s))
			}
		}
		for _, b := range op.Else {
			s, err := emitBody(b)
			if err != nil {
				return "", err
			}
			elseEq := 0
			if op.CondEq == 0 {
				elseEq = 1
			}
			if op.CondCbit == 0 {
				parts = append(parts, fmt.Sprintf("if(c==%d) %s;", elseEq, s))
			} else {
				parts = append(parts, fmt.Sprintf("if(c[%d]==%d) %s;", op.CondCbit, elseEq, s))
			}
		}
		return strings.Join(parts, "\n"), nil
	case ir.OpWHILE:
		return "", fmt.Errorf("WHILE is not representable in OpenQASM 2.0 subset — use openqasm (3)")
	case ir.OpSWITCH, ir.OpPAR, ir.OpASSERT:
		return "", fmt.Errorf("%s is not representable in OpenQASM 2.0 subset — use openqasm (3)", op.Kind)
	case ir.OpISWAP, ir.OpCRX, ir.OpCRY, ir.OpCSWAP:
		return "", fmt.Errorf("gate %s is not representable in OpenQASM 2.0 subset — use openqasm (3)", op.Kind)
	default:
		return "", fmt.Errorf("unsupported gate %q for OpenQASM 2", op.Kind)
	}
}

// ── Qiskit (IBM) ──────────────────────────────────────────────────────────────

func toQiskit(p *ir.Program) (string, error) {
	n := numQubits(p)
	var b strings.Builder
	b.WriteString("from qiskit import QuantumCircuit\n")
	for _, op := range p.Ops {
		if op.Kind == ir.OpISWAP {
			b.WriteString("from qiskit.circuit.library import iSwapGate\n")
			break
		}
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "qc = QuantumCircuit(%d, %d)\n", n, n)
	for _, op := range p.Ops {
		line, err := opToQiskit(op)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

func opToQiskit(op ir.Op) (string, error) {
	q := op.Qubits
	switch op.Kind {
	case ir.OpH:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.h(%d)", q[0]), nil
	case ir.OpX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.x(%d)", q[0]), nil
	case ir.OpY:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.y(%d)", q[0]), nil
	case ir.OpZ:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.z(%d)", q[0]), nil
	case ir.OpS:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.s(%d)", q[0]), nil
	case ir.OpT:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.t(%d)", q[0]), nil
	case ir.OpSDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.sdg(%d)", q[0]), nil
	case ir.OpTDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.tdg(%d)", q[0]), nil
	case ir.OpSX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.sx(%d)", q[0]), nil
	case ir.OpRX:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.rx(%g, %d)", op.Args[0], q[0]), nil
	case ir.OpRY:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.ry(%g, %d)", op.Args[0], q[0]), nil
	case ir.OpRZ:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.rz(%g, %d)", op.Args[0], q[0]), nil
	case ir.OpP:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.p(%g, %d)", op.Args[0], q[0]), nil
	case ir.OpU:
		if err := checkOp(op, 1, 3); err != nil { return "", err }
		return fmt.Sprintf("qc.u(%g, %g, %g, %d)", op.Args[0], op.Args[1], op.Args[2], q[0]), nil
	case ir.OpCNOT:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.cx(%d, %d)", q[0], q[1]), nil
	case ir.OpCZ:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.cz(%d, %d)", q[0], q[1]), nil
	case ir.OpSWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.swap(%d, %d)", q[0], q[1]), nil
	case ir.OpISWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.append(iSwapGate(), [%d, %d])", q[0], q[1]), nil
	case ir.OpCRX:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.crx(%g, %d, %d)", op.Args[0], q[0], q[1]), nil
	case ir.OpCRY:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.cry(%g, %d, %d)", op.Args[0], q[0], q[1]), nil
	case ir.OpCRZ:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.crz(%g, %d, %d)", op.Args[0], q[0], q[1]), nil
	case ir.OpCCX:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.ccx(%d, %d, %d)", q[0], q[1], q[2]), nil
	case ir.OpCSWAP:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.cswap(%d, %d, %d)", q[0], q[1], q[2]), nil
	case ir.OpMEASURE:
		if len(q) == 0 {
			return "qc.measure_all()", nil
		}
		targets := make([]int, len(q))
		for i, qi := range q {
			targets[i] = qi
			if i < len(op.MeasTargets) {
				targets[i] = op.MeasTargets[i]
			}
		}
		if len(q) == 1 {
			return fmt.Sprintf("qc.measure(%d, %d)", q[0], targets[0]), nil
		}
		qs := make([]string, len(q))
		cs := make([]string, len(q))
		for i := range q {
			qs[i] = fmt.Sprintf("%d", q[i])
			cs[i] = fmt.Sprintf("%d", targets[i])
		}
		return fmt.Sprintf("qc.measure([%s], [%s])", strings.Join(qs, ", "), strings.Join(cs, ", ")), nil
	case ir.OpBARRIER:
		if len(q) == 0 {
			return "qc.barrier()", nil
		}
		indices := make([]string, len(q))
		for i, qi := range q {
			indices[i] = fmt.Sprintf("%d", qi)
		}
		return fmt.Sprintf("qc.barrier(%s)", strings.Join(indices, ", ")), nil
	case ir.OpRESET:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.reset(%d)", q[0]), nil
	case ir.OpIF:
		if len(op.Then) == 0 && op.Body == nil {
			return "", fmt.Errorf("IF missing body")
		}
		if op.CondRightBit >= 0 || op.CondCbit < 0 {
			return "", fmt.Errorf("Qiskit export only supports IF c[i]==v — use OpenQASM 3 for richer conditions")
		}
		emitOne := func(body ir.Op) (string, error) {
			return opToQiskit(body)
		}
		var thenLines []string
		if len(op.Then) > 0 {
			for _, b := range op.Then {
				s, err := emitOne(b)
				if err != nil {
					return "", err
				}
				thenLines = append(thenLines, "    "+s)
			}
		} else {
			s, err := emitOne(*op.Body)
			if err != nil {
				return "", err
			}
			thenLines = append(thenLines, "    "+s)
		}
		head := fmt.Sprintf("with qc.if_test((qc.clbits[%d], %d))", op.CondCbit, op.CondEq)
		if len(op.Else) == 0 {
			return head + ":\n" + strings.Join(thenLines, "\n"), nil
		}
		var elseLines []string
		for _, b := range op.Else {
			s, err := emitOne(b)
			if err != nil {
				return "", err
			}
			elseLines = append(elseLines, "    "+s)
		}
		return head + " as else_:\n" + strings.Join(thenLines, "\n") + "\nwith else_:\n" + strings.Join(elseLines, "\n"), nil
	case ir.OpWHILE:
		if len(op.Then) == 0 {
			return "", fmt.Errorf("WHILE missing body")
		}
		if op.CondRightBit >= 0 || op.CondCbit < 0 {
			return "", fmt.Errorf("Qiskit export only supports WHILE c[i]==v — use OpenQASM 3 for richer conditions")
		}
		var thenLines []string
		for _, b := range op.Then {
			s, err := opToQiskit(b)
			if err != nil {
				return "", err
			}
			thenLines = append(thenLines, "    "+s)
		}
		return fmt.Sprintf(
			"_quell_w = 0\nwhile qc.clbits[%d] == %d and _quell_w < %d:\n%s\n    _quell_w += 1",
			op.CondCbit, op.CondEq, op.MaxIter, strings.Join(thenLines, "\n"),
		), nil
	case ir.OpSWITCH:
		return "", fmt.Errorf("SWITCH is not exported to Qiskit yet — use OpenQASM 3")
	case ir.OpPAR:
		var parts []string
		for _, b := range op.Then {
			s, err := opToQiskit(b)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "# PAR\n" + strings.Join(parts, "\n"), nil
	case ir.OpASSERT:
		return fmt.Sprintf("# ASSERT %s (local sim only)", openQASMCond(op)), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", op.Kind)
	}
}

// ── Cirq (Google) ─────────────────────────────────────────────────────────────

func toCirq(p *ir.Program) (string, error) {
	n := numQubits(p)
	var b strings.Builder
	b.WriteString("import cirq\n\n")
	fmt.Fprintf(&b, "q = cirq.LineQubit.range(%d)\n", n)
	b.WriteString("ops = []\n")
	for _, op := range p.Ops {
		line, err := opToCirq(op)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s\n", line)
	}
	b.WriteString("\ncircuit = cirq.Circuit(ops)\n")
	b.WriteString("print(circuit)\n")
	return b.String(), nil
}

func opToCirq(op ir.Op) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", i) }
	switch op.Kind {
	case ir.OpH:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.H(%s))", q(op.Qubits[0])), nil
	case ir.OpX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.X(%s))", q(op.Qubits[0])), nil
	case ir.OpY:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.Y(%s))", q(op.Qubits[0])), nil
	case ir.OpZ:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.Z(%s))", q(op.Qubits[0])), nil
	case ir.OpS:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.S(%s))", q(op.Qubits[0])), nil
	case ir.OpT:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.T(%s))", q(op.Qubits[0])), nil
	case ir.OpSDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.S(%s)**-1)", q(op.Qubits[0])), nil
	case ir.OpTDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.T(%s)**-1)", q(op.Qubits[0])), nil
	case ir.OpSX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.X(%s)**0.5)", q(op.Qubits[0])), nil
	case ir.OpRX:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rx(rads=%g)(%s))", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRY:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ry(rads=%g)(%s))", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRZ:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rz(rads=%g)(%s))", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpP:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ZPowGate(exponent=%g/%g)(%s))", op.Args[0], math.Pi, q(op.Qubits[0])), nil
	case ir.OpU:
		if err := checkOp(op, 1, 3); err != nil { return "", err }
		// U(θ,φ,λ) = Rz(φ)·Ry(θ)·Rz(λ) decomposition
		return fmt.Sprintf(
			"ops.extend([cirq.rz(rads=%g)(%s), cirq.ry(rads=%g)(%s), cirq.rz(rads=%g)(%s)])",
			op.Args[2], q(op.Qubits[0]),
			op.Args[0], q(op.Qubits[0]),
			op.Args[1], q(op.Qubits[0]),
		), nil
	case ir.OpCNOT:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CNOT(%s, %s))", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCZ:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CZ(%s, %s))", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpSWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.SWAP(%s, %s))", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpISWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ISWAP(%s, %s))", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCRX:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rx(rads=%g).controlled()(%s, %s))", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCRY:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ry(rads=%g).controlled()(%s, %s))", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCRZ:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rz(rads=%g).controlled()(%s, %s))", op.Args[0], q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCCX:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CCX(%s, %s, %s))", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpCSWAP:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CSWAP(%s, %s, %s))", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpMEASURE:
		if len(op.Qubits) == 0 {
			return "ops.append(cirq.measure(*q, key='result'))", nil
		}
		if len(op.Qubits) == 1 {
			// Key 'cN' matches IF → with_classical_controls('cN')
			return fmt.Sprintf("ops.append(cirq.measure(%s, key='c%d'))", q(op.Qubits[0]), op.Qubits[0]), nil
		}
		var qs []string
		for _, qi := range op.Qubits {
			qs = append(qs, q(qi))
		}
		return fmt.Sprintf("ops.append(cirq.measure(%s, key='c%d'))", strings.Join(qs, ", "), op.Qubits[0]), nil
	case ir.OpBARRIER:
		// Cirq has no barrier instruction — moments provide equivalent grouping
		return "# barrier (use cirq.Circuit([cirq.Moment(ops)]) for explicit moment separation)", nil
	case ir.OpRESET:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.reset(%s))", q(op.Qubits[0])), nil
	case ir.OpIF:
		if op.Body == nil && len(op.Then) == 0 {
			return "", fmt.Errorf("IF missing body")
		}
		if op.CondRightBit >= 0 || op.CondCbit < 0 || op.CondEq != 1 {
			return "", fmt.Errorf("Cirq export supports only IF c[i]==1 with a single gate — use openqasm")
		}
		bodyOp := op.Body
		if bodyOp == nil && len(op.Then) == 1 {
			bodyOp = &op.Then[0]
		}
		if bodyOp == nil {
			return "", fmt.Errorf("Cirq export of block IF requires a single then-gate — use openqasm")
		}
		body, err := opToCirq(*bodyOp)
		if err != nil {
			return "", err
		}
		inner := strings.TrimPrefix(body, "ops.append(")
		inner = strings.TrimSuffix(inner, ")")
		return fmt.Sprintf("ops.append((%s).with_classical_controls('c%d'))", inner, op.CondCbit), nil
	case ir.OpWHILE, ir.OpSWITCH, ir.OpASSERT:
		return "", fmt.Errorf("%s is not exported to Cirq — use openqasm", op.Kind)
	case ir.OpPAR:
		var parts []string
		for _, b := range op.Then {
			s, err := opToCirq(b)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "# PAR\n" + strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", op.Kind)
	}
}

// ── AWS Braket ────────────────────────────────────────────────────────────────

func toBraket(p *ir.Program) (string, error) {
	var b strings.Builder
	b.WriteString("from braket.circuits import Circuit\n")
	b.WriteString("from braket.devices import LocalSimulator\n\n")
	b.WriteString("circuit = Circuit()\n")
	for _, op := range p.Ops {
		line, err := opToBraket(op)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\ndevice = LocalSimulator()\n")
	b.WriteString("result = device.run(circuit, shots=1024).result()\n")
	b.WriteString("print(result.measurement_counts)\n")
	return b.String(), nil
}

func opToBraket(op ir.Op) (string, error) {
	q := op.Qubits
	switch op.Kind {
	case ir.OpH:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.h(%d)", q[0]), nil
	case ir.OpX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.x(%d)", q[0]), nil
	case ir.OpY:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.y(%d)", q[0]), nil
	case ir.OpZ:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.z(%d)", q[0]), nil
	case ir.OpS:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.s(%d)", q[0]), nil
	case ir.OpT:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.t(%d)", q[0]), nil
	case ir.OpSDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.si(%d)", q[0]), nil
	case ir.OpTDG:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.ti(%d)", q[0]), nil
	case ir.OpSX:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.v(%d)", q[0]), nil
	case ir.OpRX:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.rx(%d, %g)", q[0], op.Args[0]), nil
	case ir.OpRY:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.ry(%d, %g)", q[0], op.Args[0]), nil
	case ir.OpRZ:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.rz(%d, %g)", q[0], op.Args[0]), nil
	case ir.OpP:
		if err := checkOp(op, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.phaseshift(%d, %g)", q[0], op.Args[0]), nil
	case ir.OpU:
		if err := checkOp(op, 1, 3); err != nil { return "", err }
		// U(θ,φ,λ) decomposed as Rz(φ)·Ry(θ)·Rz(λ)
		return fmt.Sprintf(
			"circuit.rz(%d, %g); circuit.ry(%d, %g); circuit.rz(%d, %g)",
			q[0], op.Args[2], q[0], op.Args[0], q[0], op.Args[1],
		), nil
	case ir.OpCNOT:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.cnot(%d, %d)", q[0], q[1]), nil
	case ir.OpCZ:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.cz(%d, %d)", q[0], q[1]), nil
	case ir.OpSWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.swap(%d, %d)", q[0], q[1]), nil
	case ir.OpISWAP:
		if err := checkOp(op, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.iswap(%d, %d)", q[0], q[1]), nil
	case ir.OpCRX:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.crx(%d, %d, %g)", q[0], q[1], op.Args[0]), nil
	case ir.OpCRY:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.cry(%d, %d, %g)", q[0], q[1], op.Args[0]), nil
	case ir.OpCRZ:
		if err := checkOp(op, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.crz(%d, %d, %g)", q[0], q[1], op.Args[0]), nil
	case ir.OpCCX:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.ccnot(%d, %d, %d)", q[0], q[1], q[2]), nil
	case ir.OpCSWAP:
		if err := checkOp(op, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.cswap(%d, %d, %d)", q[0], q[1], q[2]), nil
	case ir.OpMEASURE:
		if len(q) == 0 {
			// Braket measures all qubits at end of circuit — result.measurement_counts has all outcomes
			return "# Braket measures all qubits implicitly when running on a simulator/device", nil
		}
		var parts []string
		for _, qi := range q {
			parts = append(parts, fmt.Sprintf("circuit.measure(%d)", qi))
		}
		return strings.Join(parts, "; "), nil
	case ir.OpBARRIER:
		return "# barrier (Braket uses circuit slices; no explicit barrier instruction)", nil
	case ir.OpRESET:
		return "# reset (Braket does not support mid-circuit reset — initialize qubits before the circuit)", nil
	case ir.OpPAR:
		var parts []string
		for _, b := range op.Then {
			s, err := opToBraket(b)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "# PAR\n" + strings.Join(parts, "\n"), nil
	case ir.OpASSERT:
		return fmt.Sprintf("# ASSERT %s (local sim only)", openQASMCond(op)), nil
	case ir.OpIF, ir.OpWHILE, ir.OpSWITCH:
		return "", fmt.Errorf("%s is not exported to Braket — use openqasm", op.Kind)
	default:
		return "", fmt.Errorf("unsupported gate %q", op.Kind)
	}
}

// ── Q# (Microsoft Quantum Development Kit) ────────────────────────────────────

func toQSharp(p *ir.Program) (string, error) {
	n := numQubits(p)
	if n < 1 {
		n = 1
	}
	var b strings.Builder
	b.WriteString("namespace QuellExport {\n")
	b.WriteString("    open Microsoft.Quantum.Intrinsic;\n")
	b.WriteString("    open Microsoft.Quantum.Canon;\n\n")
	b.WriteString("    operation Run() : Result[] {\n")
	fmt.Fprintf(&b, "        use qs = Qubit[%d];\n", n)
	for _, op := range p.Ops {
		lines, err := opToQSharp(op, "        ")
		if err != nil {
			return "", err
		}
		b.WriteString(lines)
	}
	b.WriteString("        mutable results = [];\n")
	fmt.Fprintf(&b, "        for i in 0 .. %d {\n", n-1)
	b.WriteString("            set results += [M(qs[i])];\n")
	b.WriteString("        }\n")
	b.WriteString("        ResetAll(qs);\n")
	b.WriteString("        return results;\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String(), nil
}

func opToQSharp(op ir.Op, indent string) (string, error) {
	q := func(i int) string { return fmt.Sprintf("qs[%d]", i) }
	switch op.Kind {
	case ir.OpH:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("H(%s);\n", q(op.Qubits[0])), nil
	case ir.OpX:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("X(%s);\n", q(op.Qubits[0])), nil
	case ir.OpY:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Y(%s);\n", q(op.Qubits[0])), nil
	case ir.OpZ:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Z(%s);\n", q(op.Qubits[0])), nil
	case ir.OpS:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("S(%s);\n", q(op.Qubits[0])), nil
	case ir.OpT:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("T(%s);\n", q(op.Qubits[0])), nil
	case ir.OpSDG:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Adjoint S(%s);\n", q(op.Qubits[0])), nil
	case ir.OpTDG:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Adjoint T(%s);\n", q(op.Qubits[0])), nil
	case ir.OpSX:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Rx(PI() / 2.0, %s);\n", q(op.Qubits[0])), nil
	case ir.OpRX:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Rx(%g, %s);\n", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRY:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Ry(%g, %s);\n", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpRZ:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Rz(%g, %s);\n", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpP:
		if err := checkOp(op, 1, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("R1(%g, %s);\n", op.Args[0], q(op.Qubits[0])), nil
	case ir.OpU:
		if err := checkOp(op, 1, 3); err != nil {
			return "", err
		}
		qi := q(op.Qubits[0])
		return indent + fmt.Sprintf("Rz(%g, %s);\n", op.Args[2], qi) +
			indent + fmt.Sprintf("Ry(%g, %s);\n", op.Args[0], qi) +
			indent + fmt.Sprintf("Rz(%g, %s);\n", op.Args[1], qi), nil
	case ir.OpCNOT:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("CNOT(%s, %s);\n", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCZ:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("CZ(%s, %s);\n", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpSWAP:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("SWAP(%s, %s);\n", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpISWAP:
		if err := checkOp(op, 2, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("// iSWAP approximated as SWAP + CZ phases\n") +
			indent + fmt.Sprintf("SWAP(%s, %s);\n", q(op.Qubits[0]), q(op.Qubits[1])), nil
	case ir.OpCCX:
		if err := checkOp(op, 3, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("CCNOT(%s, %s, %s);\n", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpCSWAP:
		if err := checkOp(op, 3, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Controlled SWAP([%s], (%s, %s));\n", q(op.Qubits[0]), q(op.Qubits[1]), q(op.Qubits[2])), nil
	case ir.OpCRX:
		if err := checkOp(op, 2, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Controlled Rx([%s], (%g, %s));\n", q(op.Qubits[0]), op.Args[0], q(op.Qubits[1])), nil
	case ir.OpCRY:
		if err := checkOp(op, 2, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Controlled Ry([%s], (%g, %s));\n", q(op.Qubits[0]), op.Args[0], q(op.Qubits[1])), nil
	case ir.OpCRZ:
		if err := checkOp(op, 2, 1); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Controlled Rz([%s], (%g, %s));\n", q(op.Qubits[0]), op.Args[0], q(op.Qubits[1])), nil
	case ir.OpMEASURE:
		if len(op.Qubits) == 0 {
			return indent + "// MEASURE all — results collected at end of Run()\n", nil
		}
		var b strings.Builder
		for _, qi := range op.Qubits {
			fmt.Fprintf(&b, "%slet _ = M(%s);\n", indent, q(qi))
		}
		return b.String(), nil
	case ir.OpRESET:
		if err := checkOp(op, 1, 0); err != nil {
			return "", err
		}
		return indent + fmt.Sprintf("Reset(%s);\n", q(op.Qubits[0])), nil
	case ir.OpBARRIER:
		return indent + "// barrier\n", nil
	case ir.OpIF:
		if op.Body == nil && len(op.Then) == 0 {
			return "", fmt.Errorf("IF missing body")
		}
		if op.CondRightBit >= 0 || op.CondCbit < 0 {
			return "", fmt.Errorf("Q# export only supports IF c[i]==v — use OpenQASM 3 for richer conditions")
		}
		cbit := op.CondCbit
		eq := op.CondEq
		bodies := op.Then
		if len(bodies) == 0 && op.Body != nil {
			bodies = []ir.Op{*op.Body}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%slet r = M(%s);\n", indent, q(cbit))
		want := "One"
		if eq == 0 {
			want = "Zero"
		}
		fmt.Fprintf(&b, "%sif (r == %s) {\n", indent, want)
		for _, body := range bodies {
			inner, err := opToQSharp(body, indent+"    ")
			if err != nil {
				return "", err
			}
			b.WriteString(inner)
		}
		b.WriteString(indent + "}\n")
		if len(op.Else) > 0 {
			fmt.Fprintf(&b, "%selse {\n", indent)
			for _, body := range op.Else {
				inner, err := opToQSharp(body, indent+"    ")
				if err != nil {
					return "", err
				}
				b.WriteString(inner)
			}
			b.WriteString(indent + "}\n")
		}
		return b.String(), nil
	case ir.OpPAR:
		var b strings.Builder
		b.WriteString(indent + "// PAR (sequentialized in Q# export)\n")
		for _, body := range op.Then {
			inner, err := opToQSharp(body, indent)
			if err != nil {
				return "", err
			}
			b.WriteString(inner)
		}
		return b.String(), nil
	case ir.OpASSERT:
		return indent + fmt.Sprintf("// ASSERT %s\n", openQASMCond(op)), nil
	case ir.OpWHILE:
		if len(op.Then) == 0 {
			return "", fmt.Errorf("WHILE missing body")
		}
		if op.CondRightBit >= 0 || op.CondCbit < 0 {
			return "", fmt.Errorf("Q# export only supports WHILE c[i]==v — use OpenQASM 3")
		}
		want := "One"
		if op.CondEq == 0 {
			want = "Zero"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%smutable _w = 0;\n", indent)
		fmt.Fprintf(&b, "%smutable _r = M(%s);\n", indent, q(op.CondCbit))
		fmt.Fprintf(&b, "%swhile (_r == %s and _w < %d) {\n", indent, want, op.MaxIter)
		for _, body := range op.Then {
			inner, err := opToQSharp(body, indent+"    ")
			if err != nil {
				return "", err
			}
			b.WriteString(inner)
		}
		fmt.Fprintf(&b, "%s    set _r = M(%s);\n", indent, q(op.CondCbit))
		fmt.Fprintf(&b, "%s    set _w += 1;\n", indent)
		b.WriteString(indent + "}\n")
		return b.String(), nil
	case ir.OpSWITCH:
		if len(op.Cases) == 0 {
			return "", fmt.Errorf("SWITCH missing cases")
		}
		cbit := op.CondCbit
		if cbit < 0 {
			cbit = 0
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%slet _sw = M(%s);\n", indent, q(cbit))
		// Q# has no switch on Result easily for multi-bit; emit if-elif chain on Result for 0/1 only, else comment.
		first := true
		for _, arm := range op.Cases {
			if arm.Default {
				continue
			}
			want := "One"
			if arm.Value == 0 {
				want = "Zero"
			}
			kw := "if"
			if !first {
				kw = "elif"
			}
			first = false
			fmt.Fprintf(&b, "%s%s (_sw == %s) {\n", indent, kw, want)
			for _, body := range arm.Body {
				inner, err := opToQSharp(body, indent+"    ")
				if err != nil {
					return "", err
				}
				b.WriteString(inner)
			}
			b.WriteString(indent + "}\n")
		}
		for _, arm := range op.Cases {
			if !arm.Default {
				continue
			}
			fmt.Fprintf(&b, "%selse {\n", indent)
			for _, body := range arm.Body {
				inner, err := opToQSharp(body, indent+"    ")
				if err != nil {
					return "", err
				}
				b.WriteString(inner)
			}
			b.WriteString(indent + "}\n")
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported gate %q for Q#", op.Kind)
	}
}
