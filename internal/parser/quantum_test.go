// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"
	"testing"
)

func TestQuantumFnBell(t *testing.T) {
	src := strings.Join([]string{
		"quantum fn bell(a: qubit, b: qubit) {",
		"    H a",
		"    CNOT a, b",
		"}",
		"qubit q0",
		"qubit q1",
		"bell q0, q1",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Quantum) != 1 || c.Quantum[0].Name != "bell" {
		t.Fatalf("quantum: %+v", c.Quantum)
	}
	if len(c.Instructions) < 2 || c.Instructions[0].Gate != "H" || c.Instructions[1].Gate != "CNOT" {
		t.Fatalf("inst: %+v", c.Instructions)
	}
}

func TestQubitRegister(t *testing.T) {
	src := "qubit q[2]\nH q[0]\nCNOT q[0], q[1]\nMEASURE\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Registers) != 1 || c.Registers[0].Size != 2 {
		t.Fatalf("regs %+v", c.Registers)
	}
	if c.Instructions[0].Qubits[0] != 0 || c.Instructions[1].Qubits[1] != 1 {
		t.Fatalf("%+v", c.Instructions)
	}
}

func TestQuantumFnRejectsHost(t *testing.T) {
	src := "quantum fn bad(q: qubit) {\n    println(\"no\")\n}\nH 0\nMEASURE\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected host rejection")
	}
}

func TestControlledX(t *testing.T) {
	src := strings.Join([]string{
		"quantum fn flip(q: qubit) {",
		"    X q",
		"}",
		"qubit c",
		"qubit t",
		"controlled flip c, t",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if c.Instructions[0].Gate != "CNOT" || c.Instructions[0].Qubits[0] != 0 || c.Instructions[0].Qubits[1] != 1 {
		t.Fatalf("%+v", c.Instructions[0])
	}
}

func TestNestedControlledX(t *testing.T) {
	src := strings.Join([]string{
		"quantum fn flip(q: qubit) {",
		"    X q",
		"}",
		"qubit a",
		"qubit b",
		"qubit t",
		"controlled controlled flip a, b, t",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Instructions[0]
	if got.Gate != "CCX" || len(got.Qubits) != 3 || got.Qubits[2] != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestControlledRejectsMeasure(t *testing.T) {
	src := strings.Join([]string{
		"quantum fn meas(q: qubit) {",
		"    MEASURE q",
		"}",
		"qubit c",
		"qubit t",
		"controlled meas c, t",
		"",
	}, "\n")
	if _, err := Parse(src); err == nil {
		t.Fatal("measurement under controlled must be rejected")
	}
}

func TestAdjointS(t *testing.T) {
	src := strings.Join([]string{
		"quantum fn phase(q: qubit) {",
		"    S q",
		"}",
		"qubit q0",
		"adjoint phase q0",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if c.Instructions[0].Gate != "SDG" {
		t.Fatalf("%+v", c.Instructions[0])
	}
}
