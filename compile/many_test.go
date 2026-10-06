// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"strings"
	"testing"
)

func TestResolveTargets(t *testing.T) {
	all, err := ResolveTargets("all")
	if err != nil || len(all) != len(Targets) {
		t.Fatalf("all: %v %v", all, err)
	}
	two, err := ResolveTargets("qiskit, cirq")
	if err != nil || len(two) != 2 || two[0] != Qiskit || two[1] != Cirq {
		t.Fatalf("list: %v %v", two, err)
	}
	if _, err := ResolveTargets("cudaq"); err == nil {
		t.Fatal("unknown target must fail")
	}
}

func TestCompileMany_EmitsEveryTargetFromOneParse(t *testing.T) {
	src := "H 0\nCNOT 0 1\nMEASURE\n"
	out, err := CompileMany(src, Targets, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(Targets) {
		t.Fatalf("got %d results", len(out))
	}
	for _, target := range Targets {
		code := out[target].Code
		if strings.TrimSpace(code) == "" {
			t.Fatalf("%s empty", target)
		}
	}
	if !strings.Contains(out[Qiskit].Code, "QuantumCircuit") && !strings.Contains(out[Qiskit].Code, "qc.") {
		t.Fatalf("qiskit output unexpected: %s", out[Qiskit].Code)
	}
	if !strings.Contains(out[OpenQASM].Code, "OPENQASM") {
		t.Fatalf("openqasm output unexpected: %s", out[OpenQASM].Code)
	}
}
