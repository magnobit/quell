// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/qerr"
)

func TestParseExpr_Table(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"1", "1"},
		{"1.5", "1.5"},
		{"true", "true"},
		{`"baseline"`, `"baseline"`},
		{"1+2*3", "1 + 2 * 3"},
		{"(1+2)*3", "(1 + 2) * 3"},
		{"1-2-3", "1 - 2 - 3"},
		{"-1", "-1"},
		{"!true", "!true"},
		{"1<2", "1 < 2"},
		{"1<=2", "1 <= 2"},
		{"1==1", "1 == 1"},
		{"1!=2", "1 != 2"},
		{"true&&false||true", "true && false || true"},
		{"int(1.5)", "int(1.5)"},
		{"float(1)", "float(1)"},
		{"string(1)", "string(1)"},
		{"bool(0)", "bool(0)"},
		{"1%2", "1 % 2"},
		{`"a\"b"`, `"a\"b"`},
	}
	for _, tc := range cases {
		e, err := ParseExpr(tc.src, 1, 1)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		got := FormatExpr(e)
		if got != tc.want {
			t.Fatalf("%s formatted %q, want %q", tc.src, got, tc.want)
		}
		again, err := ParseExpr(got, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if FormatExpr(again) != got {
			t.Fatalf("format not idempotent: %q -> %q", got, FormatExpr(again))
		}
	}
}

func TestParseExpr_Malformed(t *testing.T) {
	for _, src := range []string{"", "(", "1+", `"unterminated`, "1 & 2", "@"} {
		_, err := ParseExpr(src, 3, 2)
		if err == nil {
			t.Fatalf("%q: expected error", src)
		}
		d, ok := qerr.AsDiagnostic(err)
		if !ok || d.Code != qerr.CodeSyntax && d.Code != qerr.CodeExprDepth {
			t.Fatalf("%q: diagnostic %#v", src, d)
		}
		if d.Line != 3 || d.Message == "" {
			t.Fatalf("%q: span/message %#v", src, d)
		}
	}
}

func TestParse_LetAndKeepGates(t *testing.T) {
	src := "let shots: int = 1000\nlet theta: float = 1.5708\nlet enabled: bool = true\nlet label: string = \"baseline\"\nH 0\nMEASURE\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Host) != 4 {
		t.Fatalf("host decls %d", len(c.Host))
	}
	if c.Host[0].Name != "shots" || c.Host[0].Type != TypeInt {
		t.Fatalf("first local %+v", c.Host[0])
	}
	if len(c.Instructions) != 2 || c.Instructions[0].Gate != "H" {
		t.Fatalf("gates changed: %+v", c.Instructions)
	}
	if len(c.Params) != 0 {
		t.Fatalf("let leaked into PARAM names: %v", c.Params)
	}
}

func TestParse_LetInsideBlockRejected(t *testing.T) {
	_, err := Parse("IF c[0]==1 {\n  let x: int = 1\n}\nMEASURE\n")
	if err == nil || !strings.Contains(err.Error(), "program scope") {
		t.Fatal(err)
	}
}

func TestParse_ExistingBellUnchanged(t *testing.T) {
	c, err := Parse("H 0\nCNOT 0 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Host) != 0 || len(c.Instructions) != 3 {
		t.Fatalf("%+v", c)
	}
}
