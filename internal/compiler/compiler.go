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
	TargetOpenQASM Target = "openqasm"
	TargetQiskit   Target = "qiskit"
	TargetCirq     Target = "cirq"
	TargetBraket   Target = "braket"
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
	case TargetQiskit:
		code, err = toQiskit(p)
	case TargetCirq:
		code, err = toCirq(p)
	case TargetBraket:
		code, err = toBraket(p)
	default:
		return "", nil, fmt.Errorf("unknown target: %s (valid: openqasm, qiskit, cirq, braket)", target)
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
		for _, qi := range op.Qubits {
			parts = append(parts, fmt.Sprintf("c[%d] = measure q[%d];", qi, qi))
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
		if op.Body == nil {
			return "", fmt.Errorf("IF missing body")
		}
		body, err := opToOpenQASM(*op.Body)
		if err != nil {
			return "", err
		}
		body = strings.TrimSuffix(strings.TrimSpace(body), ";")
		return fmt.Sprintf("if (c[%d] == %d) { %s; }", op.CondCbit, op.CondEq, body), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", op.Kind)
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
		if len(q) == 1 {
			return fmt.Sprintf("qc.measure(%d, %d)", q[0], q[0]), nil
		}
		indices := make([]string, len(q))
		for i, qi := range q {
			indices[i] = fmt.Sprintf("%d", qi)
		}
		list := strings.Join(indices, ", ")
		return fmt.Sprintf("qc.measure([%s], [%s])", list, list), nil
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
		var qs []string
		for _, qi := range op.Qubits {
			qs = append(qs, q(qi))
		}
		return fmt.Sprintf("ops.append(cirq.measure(%s, key='m%d'))", strings.Join(qs, ", "), op.Qubits[0]), nil
	case ir.OpBARRIER:
		// Cirq has no barrier instruction — moments provide equivalent grouping
		return "# barrier (use cirq.Circuit([cirq.Moment(ops)]) for explicit moment separation)", nil
	case ir.OpRESET:
		if err := checkOp(op, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.reset(%s))", q(op.Qubits[0])), nil
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
	default:
		return "", fmt.Errorf("unsupported gate %q", op.Kind)
	}
}
