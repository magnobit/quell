// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func TestHostIfValues(t *testing.T) {
	src := strings.Join([]string{
		"let y: int = 1",
		"fn abs(x: int) -> int {",
		"    return if x < 0 {",
		"        -x",
		"    } else {",
		"        x",
		"    }",
		"}",
		"fn f(x: int) -> int {",
		"    return if x > 0 {",
		"        let y: int = x + 1",
		"        y",
		"    } else {",
		"        y",
		"    }",
		"}",
		"let lazy: int = if true { 1 } else { 1 / 0 }",
		"let sign: int = if false { -1 } else { 1 }",
		"let flag: bool = if true { true } else { false }",
		"let word: string = if false { \"no\" } else { \"yes\" }",
		"let nested: int = if true {",
		"    if false { 1 } else { 2 }",
		"} else {",
		"    0",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	abs, err := parser.ParseExpr("abs(-3)", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, abs)
	if len(ds) != 0 || v.Int != 3 {
		t.Fatalf("abs: %v %v", v, ds)
	}
	f2, err := parser.ParseExpr("f(2)", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds = EvalIn(c, f2)
	if len(ds) != 0 || v.Int != 3 {
		t.Fatalf("f(2): %v %v", v, ds)
	}
	f0, err := parser.ParseExpr("f(0)", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds = EvalIn(c, f0)
	if len(ds) != 0 || v.Int != 1 {
		t.Fatalf("f(0): %v %v", v, ds)
	}
	if c.Host[1].Name != "lazy" {
		t.Fatalf("host order: %+v", c.Host)
	}
}

func TestHostIfDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"non-bool", "let x: int = if 1 { 1 } else { 2 }\nH 0\nMEASURE\n", qerr.CodeIfCond},
		{"mismatch", "let x: int = if true { 1 } else { 1.0 }\nH 0\nMEASURE\n", qerr.CodeIfType},
		{"duplicate", "fn f(x: int) -> int {\n    return if true {\n        let y: int = 1\n        let y: int = 2\n        y\n    } else {\n        0\n    }\n}\nH 0\nMEASURE\n", qerr.CodeDuplicate},
		{"param shadow", "PARAM theta\nfn f(x: int) -> int {\n    return if true {\n        let theta: int = 1\n        theta\n    } else {\n        0\n    }\n}\nH 0\nMEASURE\n", qerr.CodeParamClash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := parseOK(t, tc.src)
			ds := Check(c)
			if len(ds) == 0 || ds[0].Code != tc.code {
				t.Fatalf("%+v", ds)
			}
			raw, err := qerr.DiagnosticsJSON(ds)
			if err != nil || !strings.Contains(string(raw), tc.code) || !strings.Contains(string(raw), "docsURL") {
				t.Fatalf("json: %s %v", raw, err)
			}
		})
	}
}

func TestHostIfShadowsParameter(t *testing.T) {
	src := strings.Join([]string{
		"fn f(y: int) -> int {",
		"    return if true {",
		"        let y: int = 4",
		"        y",
		"    } else {",
		"        0",
		"    }",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr("f(1)", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) != 0 || v.Int != 4 {
		t.Fatalf("shadow: %v %v", v, ds)
	}
}
