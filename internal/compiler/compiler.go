// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"fmt"
	"math"
	"strings"

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

// Compile compiles a parsed Circuit to the specified target language.
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
		return "", fmt.Errorf("unknown target: %s (valid: openqasm, qiskit, cirq, braket)", target)
	}
}

func numQubits(c *parser.Circuit) int {
	if c.NumQubits < 1 {
		return 1
	}
	return c.NumQubits
}

// checkInst validates qubit and arg counts before any array access.
// The parser already validates, so this only fires if the compiler is called
// without going through Parse (defensive check).
func checkInst(inst parser.Instruction, qubits, args int) error {
	if qubits >= 0 && len(inst.Qubits) < qubits {
		return fmt.Errorf("gate %s: need %d qubit(s), got %d", inst.Gate, qubits, len(inst.Qubits))
	}
	if len(inst.Args) < args {
		return fmt.Errorf("gate %s: need %d angle arg(s), got %d", inst.Gate, args, len(inst.Args))
	}
	return nil
}

// ── OpenQASM 3 ────────────────────────────────────────────────────────────────

func toOpenQASM(c *parser.Circuit) (string, error) {
	n := numQubits(c)
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
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("h %s;", q(inst.Qubits[0])), nil
	case "X":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("x %s;", q(inst.Qubits[0])), nil
	case "Y":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("y %s;", q(inst.Qubits[0])), nil
	case "Z":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("z %s;", q(inst.Qubits[0])), nil
	case "S":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("s %s;", q(inst.Qubits[0])), nil
	case "T":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("t %s;", q(inst.Qubits[0])), nil
	case "SDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("sdg %s;", q(inst.Qubits[0])), nil
	case "TDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("tdg %s;", q(inst.Qubits[0])), nil
	case "SX":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("sx %s;", q(inst.Qubits[0])), nil
	case "RX":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("rx(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "RY":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ry(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "RZ":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("rz(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "P":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("p(%g) %s;", inst.Args[0], q(inst.Qubits[0])), nil
	case "U":
		if err := checkInst(inst, 1, 3); err != nil { return "", err }
		return fmt.Sprintf("U(%g, %g, %g) %s;", inst.Args[0], inst.Args[1], inst.Args[2], q(inst.Qubits[0])), nil
	case "CNOT":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("cx %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CZ":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("cz %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "SWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("swap %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "ISWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("iswap %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CRX":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("crx(%g) %s, %s;", inst.Args[0], q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CRY":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("cry(%g) %s, %s;", inst.Args[0], q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CRZ":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("crz(%g) %s, %s;", inst.Args[0], q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CCX":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("ccx %s, %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1]), q(inst.Qubits[2])), nil
	case "CSWAP":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("cswap %s, %s, %s;", q(inst.Qubits[0]), q(inst.Qubits[1]), q(inst.Qubits[2])), nil
	case "MEASURE":
		if len(inst.Qubits) == 0 {
			return "c = measure q;", nil
		}
		var parts []string
		for _, qi := range inst.Qubits {
			parts = append(parts, fmt.Sprintf("c[%d] = measure q[%d];", qi, qi))
		}
		return strings.Join(parts, "\n"), nil
	case "BARRIER":
		if len(inst.Qubits) == 0 {
			return "barrier q;", nil
		}
		var qs []string
		for _, qi := range inst.Qubits {
			qs = append(qs, q(qi))
		}
		return fmt.Sprintf("barrier %s;", strings.Join(qs, ", ")), nil
	case "RESET":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("reset %s;", q(inst.Qubits[0])), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", inst.Gate)
	}
}

// ── Qiskit (IBM) ──────────────────────────────────────────────────────────────

func toQiskit(c *parser.Circuit) (string, error) {
	n := numQubits(c)
	var b strings.Builder
	b.WriteString("from qiskit import QuantumCircuit\n")
	for _, inst := range c.Instructions {
		if inst.Gate == "ISWAP" {
			b.WriteString("from qiskit.circuit.library import iSwapGate\n")
			break
		}
	}
	b.WriteString("\n")
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
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.h(%d)", q[0]), nil
	case "X":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.x(%d)", q[0]), nil
	case "Y":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.y(%d)", q[0]), nil
	case "Z":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.z(%d)", q[0]), nil
	case "S":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.s(%d)", q[0]), nil
	case "T":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.t(%d)", q[0]), nil
	case "SDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.sdg(%d)", q[0]), nil
	case "TDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.tdg(%d)", q[0]), nil
	case "SX":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.sx(%d)", q[0]), nil
	case "RX":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.rx(%g, %d)", inst.Args[0], q[0]), nil
	case "RY":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.ry(%g, %d)", inst.Args[0], q[0]), nil
	case "RZ":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.rz(%g, %d)", inst.Args[0], q[0]), nil
	case "P":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.p(%g, %d)", inst.Args[0], q[0]), nil
	case "U":
		if err := checkInst(inst, 1, 3); err != nil { return "", err }
		return fmt.Sprintf("qc.u(%g, %g, %g, %d)", inst.Args[0], inst.Args[1], inst.Args[2], q[0]), nil
	case "CNOT":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.cx(%d, %d)", q[0], q[1]), nil
	case "CZ":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.cz(%d, %d)", q[0], q[1]), nil
	case "SWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.swap(%d, %d)", q[0], q[1]), nil
	case "ISWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.append(iSwapGate(), [%d, %d])", q[0], q[1]), nil
	case "CRX":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.crx(%g, %d, %d)", inst.Args[0], q[0], q[1]), nil
	case "CRY":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.cry(%g, %d, %d)", inst.Args[0], q[0], q[1]), nil
	case "CRZ":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("qc.crz(%g, %d, %d)", inst.Args[0], q[0], q[1]), nil
	case "CCX":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.ccx(%d, %d, %d)", q[0], q[1], q[2]), nil
	case "CSWAP":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.cswap(%d, %d, %d)", q[0], q[1], q[2]), nil
	case "MEASURE":
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
	case "BARRIER":
		if len(q) == 0 {
			return "qc.barrier()", nil
		}
		indices := make([]string, len(q))
		for i, qi := range q {
			indices[i] = fmt.Sprintf("%d", qi)
		}
		return fmt.Sprintf("qc.barrier(%s)", strings.Join(indices, ", ")), nil
	case "RESET":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("qc.reset(%d)", q[0]), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", inst.Gate)
	}
}

// ── Cirq (Google) ─────────────────────────────────────────────────────────────

func toCirq(c *parser.Circuit) (string, error) {
	n := numQubits(c)
	var b strings.Builder
	b.WriteString("import cirq\n\n")
	fmt.Fprintf(&b, "q = cirq.LineQubit.range(%d)\n", n)
	b.WriteString("ops = []\n")
	for _, inst := range c.Instructions {
		line, err := instToCirq(inst)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s\n", line)
	}
	b.WriteString("\ncircuit = cirq.Circuit(ops)\n")
	b.WriteString("print(circuit)\n")
	return b.String(), nil
}

func instToCirq(inst parser.Instruction) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", i) }
	switch inst.Gate {
	case "H":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.H(%s))", q(inst.Qubits[0])), nil
	case "X":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.X(%s))", q(inst.Qubits[0])), nil
	case "Y":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.Y(%s))", q(inst.Qubits[0])), nil
	case "Z":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.Z(%s))", q(inst.Qubits[0])), nil
	case "S":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.S(%s))", q(inst.Qubits[0])), nil
	case "T":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.T(%s))", q(inst.Qubits[0])), nil
	case "SDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.S(%s)**-1)", q(inst.Qubits[0])), nil
	case "TDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.T(%s)**-1)", q(inst.Qubits[0])), nil
	case "SX":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.X(%s)**0.5)", q(inst.Qubits[0])), nil
	case "RX":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rx(rads=%g)(%s))", inst.Args[0], q(inst.Qubits[0])), nil
	case "RY":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ry(rads=%g)(%s))", inst.Args[0], q(inst.Qubits[0])), nil
	case "RZ":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rz(rads=%g)(%s))", inst.Args[0], q(inst.Qubits[0])), nil
	case "P":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ZPowGate(exponent=%g/%g)(%s))", inst.Args[0], math.Pi, q(inst.Qubits[0])), nil
	case "U":
		if err := checkInst(inst, 1, 3); err != nil { return "", err }
		// U(θ,φ,λ) = Rz(φ)·Ry(θ)·Rz(λ) decomposition
		return fmt.Sprintf(
			"ops.extend([cirq.rz(rads=%g)(%s), cirq.ry(rads=%g)(%s), cirq.rz(rads=%g)(%s)])",
			inst.Args[2], q(inst.Qubits[0]),
			inst.Args[0], q(inst.Qubits[0]),
			inst.Args[1], q(inst.Qubits[0]),
		), nil
	case "CNOT":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CNOT(%s, %s))", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CZ":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CZ(%s, %s))", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "SWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.SWAP(%s, %s))", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "ISWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ISWAP(%s, %s))", q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CRX":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rx(rads=%g).controlled()(%s, %s))", inst.Args[0], q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CRY":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.ry(rads=%g).controlled()(%s, %s))", inst.Args[0], q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CRZ":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.rz(rads=%g).controlled()(%s, %s))", inst.Args[0], q(inst.Qubits[0]), q(inst.Qubits[1])), nil
	case "CCX":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CCX(%s, %s, %s))", q(inst.Qubits[0]), q(inst.Qubits[1]), q(inst.Qubits[2])), nil
	case "CSWAP":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.CSWAP(%s, %s, %s))", q(inst.Qubits[0]), q(inst.Qubits[1]), q(inst.Qubits[2])), nil
	case "MEASURE":
		if len(inst.Qubits) == 0 {
			return "ops.append(cirq.measure(*q, key='result'))", nil
		}
		var qs []string
		for _, qi := range inst.Qubits {
			qs = append(qs, q(qi))
		}
		return fmt.Sprintf("ops.append(cirq.measure(%s, key='m%d'))", strings.Join(qs, ", "), inst.Qubits[0]), nil
	case "BARRIER":
		// Cirq has no barrier instruction — moments provide equivalent grouping
		return "# barrier (use cirq.Circuit([cirq.Moment(ops)]) for explicit moment separation)", nil
	case "RESET":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("ops.append(cirq.reset(%s))", q(inst.Qubits[0])), nil
	default:
		return "", fmt.Errorf("unsupported gate %q", inst.Gate)
	}
}

// ── AWS Braket ────────────────────────────────────────────────────────────────

func toBraket(c *parser.Circuit) (string, error) {
	var b strings.Builder
	b.WriteString("from braket.circuits import Circuit\n")
	b.WriteString("from braket.devices import LocalSimulator\n\n")
	b.WriteString("circuit = Circuit()\n")
	for _, inst := range c.Instructions {
		line, err := instToBraket(inst)
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

func instToBraket(inst parser.Instruction) (string, error) {
	q := inst.Qubits
	switch inst.Gate {
	case "H":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.h(%d)", q[0]), nil
	case "X":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.x(%d)", q[0]), nil
	case "Y":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.y(%d)", q[0]), nil
	case "Z":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.z(%d)", q[0]), nil
	case "S":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.s(%d)", q[0]), nil
	case "T":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.t(%d)", q[0]), nil
	case "SDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.si(%d)", q[0]), nil
	case "TDG":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.ti(%d)", q[0]), nil
	case "SX":
		if err := checkInst(inst, 1, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.v(%d)", q[0]), nil
	case "RX":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.rx(%d, %g)", q[0], inst.Args[0]), nil
	case "RY":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.ry(%d, %g)", q[0], inst.Args[0]), nil
	case "RZ":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.rz(%d, %g)", q[0], inst.Args[0]), nil
	case "P":
		if err := checkInst(inst, 1, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.phaseshift(%d, %g)", q[0], inst.Args[0]), nil
	case "U":
		if err := checkInst(inst, 1, 3); err != nil { return "", err }
		// U(θ,φ,λ) decomposed as Rz(φ)·Ry(θ)·Rz(λ)
		return fmt.Sprintf(
			"circuit.rz(%d, %g); circuit.ry(%d, %g); circuit.rz(%d, %g)",
			q[0], inst.Args[2], q[0], inst.Args[0], q[0], inst.Args[1],
		), nil
	case "CNOT":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.cnot(%d, %d)", q[0], q[1]), nil
	case "CZ":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.cz(%d, %d)", q[0], q[1]), nil
	case "SWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.swap(%d, %d)", q[0], q[1]), nil
	case "ISWAP":
		if err := checkInst(inst, 2, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.iswap(%d, %d)", q[0], q[1]), nil
	case "CRX":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.crx(%d, %d, %g)", q[0], q[1], inst.Args[0]), nil
	case "CRY":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.cry(%d, %d, %g)", q[0], q[1], inst.Args[0]), nil
	case "CRZ":
		if err := checkInst(inst, 2, 1); err != nil { return "", err }
		return fmt.Sprintf("circuit.crz(%d, %d, %g)", q[0], q[1], inst.Args[0]), nil
	case "CCX":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.ccnot(%d, %d, %d)", q[0], q[1], q[2]), nil
	case "CSWAP":
		if err := checkInst(inst, 3, 0); err != nil { return "", err }
		return fmt.Sprintf("circuit.cswap(%d, %d, %d)", q[0], q[1], q[2]), nil
	case "MEASURE":
		if len(q) == 0 {
			// Braket measures all qubits at end of circuit — result.measurement_counts has all outcomes
			return "# Braket measures all qubits implicitly when running on a simulator/device", nil
		}
		var parts []string
		for _, qi := range q {
			parts = append(parts, fmt.Sprintf("circuit.measure(%d)", qi))
		}
		return strings.Join(parts, "; "), nil
	case "BARRIER":
		return "# barrier (Braket uses circuit slices; no explicit barrier instruction)", nil
	case "RESET":
		return "# reset (Braket does not support mid-circuit reset — initialize qubits before the circuit)", nil
	default:
		return "", fmt.Errorf("unsupported gate %q", inst.Gate)
	}
}
