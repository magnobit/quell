// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/qerr"
)

func TestParseHostIfExpr(t *testing.T) {
	src := "let sign: int = if true {\n    -1\n} else {\n    1\n}\nH 0\nMEASURE\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Host) != 1 {
		t.Fatalf("host: %+v", c.Host)
	}
	ife, ok := c.Host[0].Expr.(*IfExpr)
	if !ok || ife.Else == nil {
		t.Fatalf("expr: %#v", c.Host[0].Expr)
	}
	if len(c.Instructions) != 2 {
		t.Fatalf("gates changed: %+v", c.Instructions)
	}
}

func TestParseHostIfOneLineAndReturn(t *testing.T) {
	src := "fn abs(x: int) -> int {\n    return if x < 0 { -x } else { x }\n}\nH 0\nMEASURE\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Functions[0].Canonical()
	want := "fn abs(x: int) -> int {\n    return if x < 0 {\n        -x\n    } else {\n        x\n    }\n}"
	if got != want {
		t.Fatalf("canonical:\n%s", got)
	}
}

func TestParseHostIfBranchLocals(t *testing.T) {
	src := strings.Join([]string{
		"fn f(x: int) -> int {",
		"    return if x > 0 {",
		"        let y: int = x + 1",
		"        y",
		"    } else {",
		"        let y: int = 0",
		"        y",
		"    }",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	ife := c.Functions[0].Body[0].Expr.(*IfExpr)
	if len(ife.Then.Lets) != 1 || ife.Then.Lets[0].Name != "y" || len(ife.Else.Lets) != 1 {
		t.Fatalf("%+v / %+v", ife.Then, ife.Else)
	}
}

func TestParseHostIfRejects(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"missing else", "let x: int = if true { 1 }\nH 0\nMEASURE\n", qerr.CodeIfElse},
		{"empty branch", "let x: int = if true { } else { 1 }\nH 0\nMEASURE\n", qerr.CodeIfBlock},
		{"two values", "let x: int = if true {\n1\n2\n} else {\n0\n}\nH 0\nMEASURE\n", qerr.CodeIfBlock},
		{"statement", "if true { 1 } else { 0 }\nH 0\nMEASURE\n", qerr.CodeSyntax},
		{"return in branch", "fn f(x: bool) -> int {\n    return if x {\n        return 1\n    } else {\n        0\n    }\n}\nH 0\nMEASURE\n", qerr.CodeIfBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src)
			d, ok := qerr.AsDiagnostic(err)
			if !ok || d.Code != tc.code {
				t.Fatalf("err=%v diag=%+v", err, d)
			}
		})
	}
}

func TestUppercaseIFUnchanged(t *testing.T) {
	src := "H 0\nMEASURE 0\nIF c[0]==1 {\n  X 1\n}\nELSE {\n  Z 1\n}\nMEASURE\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ins := range c.Instructions {
		if ins.Gate == "IF" {
			found = true
		}
	}
	if !found {
		t.Fatalf("QPU IF missing: %+v", c.Instructions)
	}
	if _, err := Parse("If c[0]==1 X 1\nMEASURE\n"); err != nil {
		t.Fatal(err)
	}
}
