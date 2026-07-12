// Copyright 2026 Magnobit, Inc. All rights reserved.

package compiler_test

import (
	"testing"

	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/parser"
)

// bellSrc is a minimal Bell-pair circuit used as the golden-output fixture
// for every target. Expected strings below were captured from the compiler
// prior to the ir.Program rewrite, to prove the rewrite is behavior-
// preserving for the unoptimized (optimize=false) path.
const bellSrc = "H 0\nCNOT 0 1\nMEASURE\n"

func compileBell(t *testing.T, target compiler.Target) string {
	t.Helper()
	circ, err := parser.Parse(bellSrc)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	code, _, err := compiler.Compile(circ, target, false)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return code
}

func TestCompileBellOpenQASM(t *testing.T) {
	want := "OPENQASM 3;\n" +
		"qubit[2] q;\n" +
		"bit[2] c;\n" +
		"\n" +
		"h q[0];\n" +
		"cx q[0], q[1];\n" +
		"c = measure q;\n"

	got := compileBell(t, compiler.TargetOpenQASM)
	if got != want {
		t.Fatalf("OpenQASM output mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestCompileBellQiskit(t *testing.T) {
	want := "from qiskit import QuantumCircuit\n" +
		"\n" +
		"qc = QuantumCircuit(2, 2)\n" +
		"qc.h(0)\n" +
		"qc.cx(0, 1)\n" +
		"qc.measure_all()\n"

	got := compileBell(t, compiler.TargetQiskit)
	if got != want {
		t.Fatalf("Qiskit output mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestCompileBellCirq(t *testing.T) {
	want := "import cirq\n" +
		"\n" +
		"q = cirq.LineQubit.range(2)\n" +
		"ops = []\n" +
		"ops.append(cirq.H(q[0]))\n" +
		"ops.append(cirq.CNOT(q[0], q[1]))\n" +
		"ops.append(cirq.measure(*q, key='result'))\n" +
		"\n" +
		"circuit = cirq.Circuit(ops)\n" +
		"print(circuit)\n"

	got := compileBell(t, compiler.TargetCirq)
	if got != want {
		t.Fatalf("Cirq output mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestCompileBellBraket(t *testing.T) {
	want := "from braket.circuits import Circuit\n" +
		"from braket.devices import LocalSimulator\n" +
		"\n" +
		"circuit = Circuit()\n" +
		"circuit.h(0)\n" +
		"circuit.cnot(0, 1)\n" +
		"# Braket measures all qubits implicitly when running on a simulator/device\n" +
		"\n" +
		"device = LocalSimulator()\n" +
		"result = device.run(circuit, shots=1024).result()\n" +
		"print(result.measurement_counts)\n"

	got := compileBell(t, compiler.TargetBraket)
	if got != want {
		t.Fatalf("Braket output mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestCompileOptimizeRemovesRedundantGates is a smoke test proving the
// optimize=true path actually runs the optimizer (as opposed to the
// optimize=false golden tests above, which prove it doesn't run).
func TestCompileOptimizeRemovesRedundantGates(t *testing.T) {
	src := "X 0\nX 0\nMEASURE\n"
	circ, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	code, notes, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected optimizer notes when optimize=true, got none")
	}

	want := "OPENQASM 3;\nqubit[1] q;\nbit[1] c;\n\nc = measure q;\n"
	if code != want {
		t.Fatalf("optimized output mismatch:\ngot:\n%q\nwant:\n%q", code, want)
	}
}
