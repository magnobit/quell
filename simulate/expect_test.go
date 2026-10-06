// Copyright 2026 Magnobit, Inc. All rights reserved.

package simulate

import (
	"math"
	"testing"

	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/parser"
)

func TestExpectationZ(t *testing.T) {
	c, err := parser.Parse("observable h = Z(0)\nH 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectationOf(c, "h")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got) > 1e-9 {
		t.Fatalf("H then Z expectation = %v, want 0", got)
	}
}

func TestExpectationBell(t *testing.T) {
	c, err := parser.Parse("observable zz = Z(0) * Z(1)\nH 0\nCNOT 0 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectationOf(c, "zz")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("bell ZZ = %v", got)
	}
}

func TestHostExpectation(t *testing.T) {
	src := "observable h = Z(0)\nfn energy() -> float { return expectation(h) }\nH 0\nMEASURE\n"
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if ds := check.Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr("energy()", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := check.EvalIn(c, e)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if math.Abs(v.Float) > 1e-9 {
		t.Fatalf("energy %v", v.Float)
	}
}
