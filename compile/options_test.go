// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"strings"
	"testing"
)

func TestCompileWithOptions_KnownTopologyInsertsSWAPs(t *testing.T) {
	src := "H 0\nCNOT 0 3\nMEASURE"
	generic, err := CompileWithOptions(src, OpenQASM, CompileOptions{Optimize: true})
	if err != nil {
		t.Fatalf("generic compile: %v", err)
	}
	routed, err := CompileWithOptions(src, OpenQASM, CompileOptions{
		Optimize: true, Coupling: [][2]int{{0, 1}, {1, 2}, {2, 3}}, CouplingName: "ibm",
	})
	if err != nil {
		t.Fatalf("routed compile: %v", err)
	}
	if strings.Contains(generic.Code, "swap") {
		t.Errorf("generic Optimize must not insert SWAPs, got:\n%s", generic.Code)
	}
	if !strings.Contains(routed.Code, "swap") {
		t.Errorf("known linear-4 topology must insert SWAPs for CNOT 0 3, got:\n%s notes=%v", routed.Code, routed.OptimizerNotes)
	}
	joined := strings.Join(routed.OptimizerNotes, " ")
	if !strings.Contains(joined, "routing") || !strings.Contains(joined, "ibm") {
		t.Errorf("optimizer notes should mention routing on the selected backend, got %v", routed.OptimizerNotes)
	}
}

func TestCompileWithOptions_UnknownTopologyMatchesGeneric(t *testing.T) {
	src := "H 0\nCNOT 0 3\nMEASURE"
	a, err := CompileWithWarnings(src, OpenQASM, true)
	if err != nil {
		t.Fatalf("CompileWithWarnings: %v", err)
	}
	b, err := CompileWithOptions(src, OpenQASM, CompileOptions{Optimize: true})
	if err != nil {
		t.Fatalf("CompileWithOptions: %v", err)
	}
	if a.Code != b.Code {
		t.Errorf("nil coupling must match existing generic optimize\n--- warnings ---\n%s\n--- options ---\n%s", a.Code, b.Code)
	}
}
