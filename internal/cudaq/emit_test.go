// Copyright 2026 Magnobit, Inc. All rights reserved.

package cudaq

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func TestEmitKernel(t *testing.T) {
	c, err := parser.Parse("H 0\nCNOT 0 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	text, err := Emit(ir.Lower(c))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "qvector") || !strings.Contains(text, "h(q[0])") || !strings.Contains(text, "cx(q[0], q[1])") {
		t.Fatal(text)
	}
}

func TestEmitRejectsDynamic(t *testing.T) {
	c, err := parser.Parse("H 0\nMEASURE\nIF c[0]==1 X 0\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Emit(ir.Lower(c)); err == nil {
		t.Fatal("dynamic control must not be claimed as CUDA-Q")
	}
}
