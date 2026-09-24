// Copyright 2026 Magnobit, Inc. All rights reserved.

package adapter

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func TestProgramQASM_CouplingReachesSubmittedOpenQASM(t *testing.T) {
	circ, err := parser.Parse("H 0\nCNOT 0 3\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	prog := ir.Lower(circ)

	generic, _, err := programQASM(&Job{Program: prog, Optimize: true})
	if err != nil {
		t.Fatalf("generic programQASM: %v", err)
	}
	routed, _, err := programQASM(&Job{
		Program: prog, Optimize: true,
		Coupling:     [][2]int{{0, 1}, {1, 2}, {2, 3}},
		CouplingName: "ibm",
	})
	if err != nil {
		t.Fatalf("routed programQASM: %v", err)
	}
	if routed == generic {
		t.Fatal("IBM/adapter programQASM must submit topology-aware OpenQASM, not a generic recompile of the original IR")
	}
	if !strings.Contains(strings.ToLower(routed), "swap") {
		t.Errorf("linear coupling 0–3 should insert SWAP(s) into submitted QASM, got:\n%s", routed)
	}

	native, _, err := programQASM(&Job{
		Program: ir.Lower(mustParse(t, "H 0\nCNOT 0 1\nMEASURE\n")), Optimize: true,
		Coupling:     [][2]int{{0, 1}, {1, 2}},
		CouplingName: "ibm",
	})
	if err != nil {
		t.Fatalf("native programQASM: %v", err)
	}
	if strings.Contains(strings.ToLower(native), "swap") {
		t.Errorf("already-adjacent CNOT 0 1 must not receive unnecessary SWAPs, got:\n%s", native)
	}
}

func mustParse(t *testing.T, src string) *parser.Circuit {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
