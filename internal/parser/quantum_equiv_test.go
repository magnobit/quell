// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser_test

import (
	"bytes"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
)

func TestControlledMatchesCNOT(t *testing.T) {
	kernel := "quantum fn flip(q: qubit) {\n    X q\n}\nqubit c\nqubit t\ncontrolled flip c, t\nMEASURE\n"
	direct := "CNOT 0 1\nMEASURE\n"
	a, err := parser.Parse(kernel)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parser.Parse(direct)
	if err != nil {
		t.Fatal(err)
	}
	ao, _ := optimizer.Optimize(ir.Lower(a))
	bo, _ := optimizer.Optimize(ir.Lower(b))
	if !bytes.Equal(ir.CanonicalBytes(ao), ir.CanonicalBytes(bo)) {
		t.Fatalf("controlled X did not match CNOT\n%s\n%s", ir.CanonicalBytes(ao), ir.CanonicalBytes(bo))
	}
}
