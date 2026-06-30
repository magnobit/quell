package compiler

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/parser"
)

type Target string

const (
	TargetOpenQASM Target = "openqasm"
	TargetQiskit   Target = "qiskit"
	TargetCirq     Target = "cirq"
	TargetBraket   Target = "braket"
)

func Compile(c *parser.Circuit, target Target) (string, error) {
	switch target {
	case TargetOpenQASM:
		return toOpenQASM(c)
	case TargetQiskit:
		return toQiskit(c)
	case TargetCirq:
		return toCirq(c)
	case TargetBraket:
		return toBraket(c)
	default:
		return "", fmt.Errorf("unknown target: %s", target)
	}
}

func toOpenQASM(c *parser.Circuit) (string, error) {
	n := c.NumQubits
	if n == 0 {
		n = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "OPENQASM 3;\n")
	fmt.Fprintf(&b, "qubit[%d] q;\n", n)
	fmt.Fprintf(&b, "bit[%d] c;\n\n", n)

	for _, inst := range c.Instructions {
		line, err := instToOpenQASM(inst)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

func instToOpenQASM(inst parser.Instruction) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", i) }
	switch inst.Gate {
	case "H":
		return fmt.Sprintf("h %s;", q(inst.Qubits[0])), nil
	case "X":
		return fmt.Sprintf("x %s;", q(inst.Qubits[0])), nil
	case "Y":
		return fmt.Sprintf("y %s;", q(inst.Qubits[0])), nil
	case "Z":
		return fmt.Sprintf("z %s;", q(inst.Qubits[0])), nil
	case "S":
		return fmt.Sprintf("s %s;", q(inst.Qubits[0])), nil
	case "T":
		return fmt.Sprintf("t %s;", q(inst.Qubits[0])), nil
	case "SDG":
		return fmt.Sprintf("sdg %s;", q(inst.Qubits[0])), nil
	case "TDG":
		return fmt.Sprintf("tdg %s;", q(inst.Qubits[0])), nil
	case "SX":
		return fmt.Sprintf("sx %s;", q(inst.Qubits[0])), nil
	case "RX":
		return fmt.Sprintf("rx(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "RY":
		return fmt.Sprintf("ry(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "RZ":
		return fmt.Sprintf("rz(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "P":
		return fmt.Sprintf("p(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "CNOT", "CX":
		return fmt.Sprintf("cx %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CZ":
		return fmt.Sprintf("cz %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "SWAP":
		return fmt.Sprintf("swap %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CCX":
		return fmt.Sprintf("ccx %s, %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1]), q(inst.Qubits[2])), nil
	case "MEASURE":
		if len(inst.Qubits) == 0 {
			return "c = measure q;", nil
		}
		return fmt.Sprintf("c[%d] = measure %s;", inst.Qubits[0], q(inst.Qubits[0])), nil
	default:
		return "", fmt.Errorf("unknown gate: %s", inst.Gate)
	}
}

func toQiskit(c *parser.Circuit) (string, error) {
	n := c.NumQubits
	if n == 0 {
		n = 1
	}
	var b strings.Builder
	b.WriteString("from qiskit import QuantumCircuit\n\n")
	fmt.Fprintf(&b, "qc = QuantumCircuit(%d, %d)\n", n, n)

	for _, inst := range c.Instructions {
		line, err := instToQiskit(inst)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

func instToQiskit(inst parser.Instruction) (string, error) {
	q := inst.Qubits
	switch inst.Gate {
	case "H":
		return fmt.Sprintf("qc.h(%d)", q[0]), nil
	case "X":
		return fmt.Sprintf("qc.x(%d)", q[0]), nil
	case "Y":
		return fmt.Sprintf("qc.y(%d)", q[0]), nil
	case "Z":
		return fmt.Sprintf("qc.z(%d)", q[0]), nil
	case "S":
		return fmt.Sprintf("qc.s(%d)", q[0]), nil
	case "T":
		return fmt.Sprintf("qc.t(%d)", q[0]), nil
	case "SDG":
		return fmt.Sprintf("qc.sdg(%d)", q[0]), nil
	case "TDG":
		return fmt.Sprintf("qc.tdg(%d)", q[0]), nil
	case "SX":
		return fmt.Sprintf("qc.sx(%d)", q[0]), nil
	case "RX":
		return fmt.Sprintf("qc.rx(%g, %d)", inst.Args[0], q[0]), nil
	case "RY":
		return fmt.Sprintf("qc.ry(%g, %d)", inst.Args[0], q[0]), nil
	case "RZ":
		return fmt.Sprintf("qc.rz(%g, %d)", inst.Args[0], q[0]), nil
	case "P":
		return fmt.Sprintf("qc.p(%g, %d)", inst.Args[0], q[0]), nil
	case "CNOT", "CX":
		return fmt.Sprintf("qc.cx(%d, %d)", q[0], q[1]), nil
	case "CZ":
		return fmt.Sprintf("qc.cz(%d, %d)", q[0], q[1]), nil
	case "SWAP":
		return fmt.Sprintf("qc.swap(%d, %d)", q[0], q[1]), nil
	case "CCX":
		return fmt.Sprintf("qc.ccx(%d, %d, %d)", q[0], q[1], q[2]), nil
	case "MEASURE":
		if len(q) == 0 {
			return fmt.Sprintf("qc.measure(range(%d), range(%d))", inst.Qubits[0], inst.Qubits[0]), nil
		}
		return fmt.Sprintf("qc.measure(%d, %d)", q[0], q[0]), nil
	default:
		return "", fmt.Errorf("unknown gate: %s", inst.Gate)
	}
}

func toCirq(c *parser.Circuit) (string, error) {
	n := c.NumQubits
	if n == 0 {
		n = 1
	}
	var b strings.Builder
	b.WriteString("import cirq\n\n")
	fmt.Fprintf(&b, "q = cirq.LineQubit.range(%d)\n", n)
	b.WriteString("circuit = cirq.Circuit([\n")

	for _, inst := range c.Instructions {
		line, err := instToCirq(inst)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "    %s,\n", line)
	}
	b.WriteString("])\n")
	return b.String(), nil
}

func instToCirq(inst parser.Instruction) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", i) }
	switch inst.Gate {
	case "H":
		return fmt.Sprintf("cirq.H(%s)", q(inst.Qubits[0])), nil
	case "X":
		return fmt.Sprintf("cirq.X(%s)", q(inst.Qubits[0])), nil
	case "Y":
		return fmt.Sprintf("cirq.Y(%s)", q(inst.Qubits[0])), nil
	case "Z":
		return fmt.Sprintf("cirq.Z(%s)", q(inst.Qubits[0])), nil
	case "S":
		return fmt.Sprintf("cirq.S(%s)", q(inst.Qubits[0])), nil
	case "T":
		return fmt.Sprintf("cirq.T(%s)", q(inst.Qubits[0])), nil
	case "CNOT", "CX":
		return fmt.Sprintf("cirq.CNOT(%s, %s)", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CZ":
		return fmt.Sprintf("cirq.CZ(%s, %s)", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "SWAP":
		return fmt.Sprintf("cirq.SWAP(%s, %s)", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CCX":
		return fmt.Sprintf("cirq.CCX(%s, %s, %s)", q(inst.Qubits[0]), q(inst.Qubits[1]), q(inst.Qubits[2])), nil
	case "MEASURE":
		if len(inst.Qubits) == 0 {
			return "cirq.measure(*q, key='result')", nil
		}
		return fmt.Sprintf("cirq.measure(%s, key='q%d')", q(inst.Qubits[0]), inst.Qubits[0]), nil
	default:
		return "", fmt.Errorf("unknown gate: %s", inst.Gate)
	}
}

func toBraket(c *parser.Circuit) (string, error) {
	var b strings.Builder
	b.WriteString("from braket.circuits import Circuit\n\n")
	b.WriteString("circuit = Circuit()\n")

	for _, inst := range c.Instructions {
		line, err := instToBraket(inst)
		if err != nil {
			return "", err
		}
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

func instToBraket(inst parser.Instruction) (string, error) {
	q := inst.Qubits
	switch inst.Gate {
	case "H":
		return fmt.Sprintf("circuit.h(%d)", q[0]), nil
	case "X":
		return fmt.Sprintf("circuit.x(%d)", q[0]), nil
	case "Y":
		return fmt.Sprintf("circuit.y(%d)", q[0]), nil
	case "Z":
		return fmt.Sprintf("circuit.z(%d)", q[0]), nil
	case "S":
		return fmt.Sprintf("circuit.s(%d)", q[0]), nil
	case "T":
		return fmt.Sprintf("circuit.t(%d)", q[0]), nil
	case "RX":
		return fmt.Sprintf("circuit.rx(%d, %g)", q[0], inst.Args[0]), nil
	case "RY":
		return fmt.Sprintf("circuit.ry(%d, %g)", q[0], inst.Args[0]), nil
	case "RZ":
		return fmt.Sprintf("circuit.rz(%d, %g)", q[0], inst.Args[0]), nil
	case "CNOT", "CX":
		return fmt.Sprintf("circuit.cnot(%d, %d)", q[0], q[1]), nil
	case "CZ":
		return fmt.Sprintf("circuit.cz(%d, %d)", q[0], q[1]), nil
	case "SWAP":
		return fmt.Sprintf("circuit.swap(%d, %d)", q[0], q[1]), nil
	case "CCX":
		return fmt.Sprintf("circuit.ccnot(%d, %d, %d)", q[0], q[1], q[2]), nil
	case "MEASURE":
		return "# Braket: use result_types.Probability() or .Sample() after circuit", nil
	default:
		return "", fmt.Errorf("unknown gate: %s", inst.Gate)
	}
}
