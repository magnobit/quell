// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func parseOK(t *testing.T, src string) *parser.Circuit {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func codes(ds []qerr.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.Code)
		b.WriteByte(' ')
	}
	return b.String()
}

func TestCheck_ValidLets(t *testing.T) {
	c := parseOK(t, "let shots: int = 1000\nlet theta: float = 1.5708\nlet enabled: bool = true\nlet label: string = \"baseline\"\nlet n: int = shots + 1\nH 0\nMEASURE\n")
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	v, ds := Eval(c.Host[4].Expr, map[string]Value{"shots": {Type: parser.TypeInt, Int: 1000}}, 1, nil)
	if len(ds) != 0 || v.Int != 1001 {
		t.Fatalf("%+v %v", v, ds)
	}
}

func TestCheck_OperatorsAndConversions(t *testing.T) {
	ok := []string{
		"let x: int = 1 + 2 * 3 - 4 / 2 % 2\nH 0\nMEASURE\n",
		"let x: float = 1.5 * 2.0\nH 0\nMEASURE\n",
		"let x: bool = !false && true || false\nH 0\nMEASURE\n",
		"let x: bool = 1 < 2 && 3.0 >= 3.0\nH 0\nMEASURE\n",
		"let x: int = int(1.9)\nH 0\nMEASURE\n",
		"let x: float = float(2)\nH 0\nMEASURE\n",
		"let x: string = string(true)\nH 0\nMEASURE\n",
		"let x: bool = bool(0)\nH 0\nMEASURE\n",
		"let x: int = int(\"12\")\nH 0\nMEASURE\n",
	}
	for _, src := range ok {
		if ds := Check(parseOK(t, src)); len(ds) != 0 {
			t.Fatalf("%s: %v", src, ds)
		}
	}
	bad := []struct{ src, code string }{
		{"let x: int = 1 + 1.0\nH 0\nMEASURE\n", qerr.CodeBadOperator},
		{"let x: int = true\nH 0\nMEASURE\n", qerr.CodeTypeMismatch},
		{"let x: int = 1\nlet x: int = 2\nH 0\nMEASURE\n", qerr.CodeDuplicate},
		{"PARAM theta\nlet theta: float = 0.5\nRX theta 0\nMEASURE\n", qerr.CodeParamClash},
		{"let y: int = missing\nH 0\nMEASURE\n", qerr.CodeUnknownIdent},
		{"let x: int = int(\"no\")\nH 0\nMEASURE\n", qerr.CodeBadConvert},
		{"let x: bool = bool(\"x\")\nH 0\nMEASURE\n", qerr.CodeBadConvert},
		{"let x: int = 1\nx = 2\nH 0\nMEASURE\n", qerr.CodeImmutable},
		{"let x: float = 1.0 % 2.0\nH 0\nMEASURE\n", qerr.CodeBadOperator},
		{"let x: int = 1 / 0\nH 0\nMEASURE\n", qerr.CodeDivZero},
		{"let theta: float = 0.5\nRX theta 0\nMEASURE\n", qerr.CodeParamClash},
	}
	for _, tc := range bad {
		c, err := parser.Parse(tc.src)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.src, err)
		}
		ds := Check(c)
		if !strings.Contains(codes(ds), tc.code) {
			t.Fatalf("%s: got %s want %s (%v)", tc.src, codes(ds), tc.code, ds)
		}
		d := ds[0]
		raw, err := d.JSON()
		if err != nil || !strings.Contains(string(raw), d.Code) || d.Line < 1 || d.Message == "" || d.SuggestedFix == "" {
			t.Fatalf("json/span %+v %s", d, raw)
		}
	}
}

func TestCheck_JSONArray(t *testing.T) {
	c := parseOK(t, "let x: int = no\nH 0\nMEASURE\n")
	raw, err := qerr.DiagnosticsJSON(Check(c))
	if err != nil || !strings.Contains(string(raw), "QL2001") {
		t.Fatalf("%s %v", raw, err)
	}
}
