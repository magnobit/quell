package parser_test

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

func TestParseBlockIfElse(t *testing.T) {
	src := `H 0
MEASURE 0
IF c[0]==1 {
  X 1
  H 1
}
ELSE {
  Z 1
}
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inst := range c.Instructions {
		if inst.Gate == "IF" && len(inst.ThenBody) == 2 && len(inst.ElseBody) == 1 {
			found = true
			if inst.ThenBody[0].Gate != "X" || inst.ElseBody[0].Gate != "Z" {
				t.Fatalf("unexpected body: %+v / %+v", inst.ThenBody, inst.ElseBody)
			}
		}
	}
	if !found {
		t.Fatalf("block IF not found: %+v", c.Instructions)
	}
	res, err := simulate.RunProgram(ir.Lower(c), 200)
	if err != nil {
		t.Fatal(err)
	}
	if res.Shots != 200 {
		t.Fatalf("shots=%d", res.Shots)
	}
}

func TestParseForUnroll(t *testing.T) {
	src := `FOR i IN 0..2 {
  H i
}
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	hs := 0
	for _, inst := range c.Instructions {
		if inst.Gate == "H" {
			hs++
		}
	}
	if hs != 3 {
		t.Fatalf("expected 3 H from FOR, got %d in %+v", hs, c.Instructions)
	}
}

func TestParseWhile(t *testing.T) {
	src := `H 0
MEASURE 0
WHILE c[0]==0 MAX 4 {
  X 0
  MEASURE 0
}
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inst := range c.Instructions {
		if inst.Gate == "WHILE" && inst.MaxIter == 4 && len(inst.ThenBody) >= 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("WHILE not found: %+v", c.Instructions)
	}
	if _, err := simulate.RunProgram(ir.Lower(c), 50); err != nil {
		t.Fatal(err)
	}
}

func TestLineIfStillWorks(t *testing.T) {
	src := "H 0\nMEASURE 0\nIF c[0]==1 X 1\nMEASURE\n"
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, inst := range c.Instructions {
		if inst.Gate == "IF" && inst.Body != nil && inst.Body.Gate == "X" {
			ok = true
			if inst.CondRightBit != -1 {
				t.Fatalf("CondRightBit=%d want -1", inst.CondRightBit)
			}
		}
	}
	if !ok {
		t.Fatal("line IF missing")
	}
}

func TestRichConditions(t *testing.T) {
	src := `H 0
H 1
MEASURE 0 -> c[0]
MEASURE 1 -> c[1]
IF c[0]==c[1] X 2
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inst := range c.Instructions {
		if inst.Gate == "IF" && inst.CondRightBit == 1 && inst.CondCbit == 0 {
			found = true
		}
		if inst.Gate == "MEASURE" && len(inst.MeasTargets) > 0 {
			if inst.MeasTargets[0] != inst.Qubits[0] && inst.Qubits[0] == 0 && inst.MeasTargets[0] != 0 {
				// ok — remapped
			}
		}
	}
	if !found {
		t.Fatalf("bit-vs-bit IF not found: %+v", c.Instructions)
	}
	if _, err := simulate.RunProgram(ir.Lower(c), 80); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchAndParAndAssert(t *testing.T) {
	src := `H 0
MEASURE 0
SWITCH c[0] {
CASE 0: X 1
CASE 1: H 1
DEFAULT: Z 1
}
PAR {
  H 2
  X 3
}
X 4
MEASURE 4
ASSERT c[4]==1
MEASURE
`
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	var hasSwitch, hasPar, hasAssert bool
	for _, inst := range c.Instructions {
		switch inst.Gate {
		case "SWITCH":
			hasSwitch = len(inst.Cases) >= 2
		case "PAR":
			hasPar = len(inst.ThenBody) == 2
		case "ASSERT":
			hasAssert = true
		}
	}
	if !hasSwitch || !hasPar || !hasAssert {
		t.Fatalf("missing constructs switch=%v par=%v assert=%v in %+v", hasSwitch, hasPar, hasAssert, c.Instructions)
	}
	if _, err := simulate.RunProgram(ir.Lower(c), 40); err != nil {
		t.Fatal(err)
	}
}

func TestParamTyped(t *testing.T) {
	src := "PARAM theta : angle\nRX theta 0\nMEASURE\n"
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Params) != 1 || c.Params[0] != "theta" {
		t.Fatalf("params=%v", c.Params)
	}
}

func TestParOverlapRejected(t *testing.T) {
	_, err := parser.Parse("PAR {\nH 0\nCNOT 0 1\n}\nMEASURE\n")
	if err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("expected overlapping error, got %v", err)
	}
}
