// Copyright 2026 Magnobit, Inc. All rights reserved.

package qir

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func TestEmitLLVM(t *testing.T) {
	c, err := parser.Parse("H 0\nCNOT 0 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	text, err := Emit(ir.Lower(c))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "__quantum__qis__h__body") || !strings.Contains(text, "__quantum__qis__cnot__body") || !strings.Contains(text, "__quantum__qis__mz__body") {
		t.Fatal(text)
	}
	if strings.Contains(text, "; unsupported") || strings.Contains(text, "%Qubit*") {
		t.Fatal(text)
	}
	if !strings.Contains(text, `qir_profiles"="base_profile"`) || !strings.Contains(text, "ret i64 0") {
		t.Fatal(text)
	}
}

func TestEmitRejectsDynamic(t *testing.T) {
	c, err := parser.Parse("H 0\nMEASURE\nIF c[0]==1 X 0\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Emit(ir.Lower(c)); err == nil {
		t.Fatal("dynamic control must not be reported as complete QIR")
	}
}
