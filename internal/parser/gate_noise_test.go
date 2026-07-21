// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser_test

import (
	"testing"

	"github.com/magnobit/quell/internal/parser"
)

func TestParse_GateDefExpand(t *testing.T) {
	src := `gate bell a b {
H a
CNOT a b
}
bell 0 1
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Instructions) != 3 {
		t.Fatalf("ops=%d want 3 (H, CNOT, MEASURE), got %+v", len(c.Instructions), c.Instructions)
	}
	if c.Instructions[0].Gate != "H" || c.Instructions[1].Gate != "CNOT" {
		t.Fatalf("unexpected expansion: %+v", c.Instructions)
	}
}

func TestParse_GateDefSingleLine(t *testing.T) {
	src := `gate bell a b { H a; CNOT a b }
bell 2 3
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if c.Instructions[0].Qubits[0] != 2 || c.Instructions[1].Qubits[1] != 3 {
		t.Fatalf("qubit map failed: %+v", c.Instructions)
	}
}

func TestParse_NOISE(t *testing.T) {
	c, err := parser.Parse("NOISE depolarizing 0.01\nNOISE amplitude_damping 0.05\nH 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.NoiseDepolarizing != 0.01 || c.NoiseAmplitudeDamping != 0.05 {
		t.Fatalf("noise fields = %g / %g", c.NoiseDepolarizing, c.NoiseAmplitudeDamping)
	}
}
