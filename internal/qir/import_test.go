// Copyright 2026 Magnobit, Inc. All rights reserved.

package qir

import (
	"os"
	"os/exec"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// requireLLVM runs QIR conformance that shells out to llvm-as.
// Default unit tests skip when the tool is absent. Release certification
// sets QUELL_QIR_REQUIRE_TOOLCHAIN=1 so a missing assembler fails the suite.
func requireLLVM(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("llvm-as"); err == nil {
		return
	}
	const msg = "QIR conformance requires llvm-as on PATH"
	if os.Getenv("QUELL_QIR_REQUIRE_TOOLCHAIN") == "1" {
		t.Fatal(msg)
	}
	t.Skip(msg + "; set QUELL_QIR_REQUIRE_TOOLCHAIN=1 to fail closed")
}

func TestRoundTripBell(t *testing.T) {
	c, err := parser.Parse("H 0\nCNOT 0 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	text, err := Emit(ir.Lower(c))
	if err != nil {
		t.Fatal(err)
	}
	back, err := Import(text)
	if err != nil {
		t.Fatal(err)
	}
	want := []ir.OpKind{ir.OpH, ir.OpCNOT, ir.OpMEASURE}
	if len(back.Ops) < len(want) {
		t.Fatalf("%+v", back.Ops)
	}
	for i, k := range want {
		if back.Ops[i].Kind != k {
			t.Fatalf("op %d %s", i, back.Ops[i].Kind)
		}
	}
	if back.Ops[1].Qubits[0] != 0 || back.Ops[1].Qubits[1] != 1 {
		t.Fatal(back.Ops[1].Qubits)
	}
}

func TestRoundTripGHZ(t *testing.T) {
	requireLLVM(t)
	c, err := parser.Parse("H 0\nCNOT 0 1\nCNOT 1 2\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	text, err := Emit(ir.Lower(c))
	if err != nil {
		t.Fatal(err)
	}
	if err := Assemble(text); err != nil {
		t.Fatal(err)
	}
	back, err := Import(text)
	if err != nil {
		t.Fatal(err)
	}
	if back.Ops[0].Kind != ir.OpH || back.Ops[1].Kind != ir.OpCNOT || back.Ops[2].Kind != ir.OpCNOT {
		t.Fatalf("%+v", back.Ops)
	}
}

func TestImportRejectsUnknown(t *testing.T) {
	text := "define void @quell_main() {\nentry:\n  call void @__quantum__qis__if__body()\n  ret void\n}\n"
	if _, err := Import(text); err == nil {
		t.Fatal("dynamic call must not import")
	}
}

func TestAssembleBell(t *testing.T) {
	requireLLVM(t)
	c, err := parser.Parse("H 0\nCNOT 0 1\nRX 0.5 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	text, err := Emit(ir.Lower(c))
	if err != nil {
		t.Fatal(err)
	}
	if err := Assemble(text); err != nil {
		t.Fatal(err)
	}
	if err := Validate(text); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(text); err != nil {
		t.Fatal(err)
	}
}

func TestResetIsOutsideBaseProfile(t *testing.T) {
	c, err := parser.Parse("H 0\nRESET 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Emit(ir.Lower(c)); err == nil {
		t.Fatal("reset must not be exported as base profile")
	}
}
