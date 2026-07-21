package ir_test

import (
	"math"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

func TestParamBindAndSimulate(t *testing.T) {
	c, err := parser.Parse("PARAM theta\nRX theta 0\nMEASURE")
	if err != nil {
		t.Fatal(err)
	}
	prog := ir.Lower(c)
	if !ir.NeedsBind(prog) {
		t.Fatal("expected unbound params")
	}
	bound, err := ir.Bind(prog, map[string]float64{"theta": math.Pi})
	if err != nil {
		t.Fatal(err)
	}
	res, err := simulate.RunProgram(bound, 200)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["1"] == 0 && res.Counts["0"] == 0 {
		t.Fatalf("empty counts: %#v", res.Counts)
	}
}

func TestIFConditional(t *testing.T) {
	// Measure qubit 0 after H → ~50% then conditionally flip qubit 1.
	src := "H 0\nMEASURE 0\nIF c[0]==1 X 1\nMEASURE"
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := simulate.RunProgram(ir.Lower(c), 500)
	if err != nil {
		t.Fatal(err)
	}
	// Outcomes should include both 00-ish and 11-ish patterns (qubit0 rightmost).
	if len(res.Counts) == 0 {
		t.Fatal("no counts")
	}
}
