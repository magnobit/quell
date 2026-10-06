// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func TestFunctionsPositive(t *testing.T) {
	src := strings.Join([]string{
		"fn square(x: float) -> float {",
		"    return x * x",
		"}",
		"fn add(a: int, b: int) -> int {",
		"    let s: int = a + b",
		"    return s",
		"}",
		"fn useAdd() -> int {",
		"    return add(2, 3)",
		"}",
		"let n: int = useAdd()",
		"let w: float = square(2.0)",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr("useAdd()", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) != 0 || v.Int != 5 {
		t.Fatalf("%+v %v", v, ds)
	}
	names, sigDiags := Signatures(c)
	if len(sigDiags) != 0 || len(names) != 3 {
		t.Fatalf("%v %v", names, sigDiags)
	}
}

func TestForwardAndNested(t *testing.T) {
	src := "fn a() -> int {\n    return b()\n}\nfn b() -> int {\n    return inc(1)\n}\nfn inc(x: int) -> int {\n    return x + 1\n}\nlet n: int = a()\nH 0\nMEASURE\n"
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	if c.Host[0].Expr == nil {
		t.Fatal("missing let")
	}
	v, ds := EvalIn(c, c.Host[0].Expr)
	if len(ds) != 0 || v.Int != 2 {
		t.Fatalf("%+v %v", v, ds)
	}
}

func TestFunctionDiagnostics(t *testing.T) {
	cases := []struct {
		src, code string
	}{
		{"fn f() -> int {\n    return 1\n}\nfn f() -> int {\n    return 2\n}\nH 0\nMEASURE\n", qerr.CodeDuplicateFunc},
		{"gate bell a {\nH a\n}\nfn bell() -> int {\n    return 1\n}\nbell 0\nMEASURE\n", qerr.CodeFuncGate},
		{"fn add(a: int, b: int) -> int {\n    return a + b\n}\nlet n: int = add(1)\nH 0\nMEASURE\n", qerr.CodeArgCount},
		{"fn add(a: int, b: int) -> int {\n    return a + b\n}\nlet n: int = add(1, 1.0)\nH 0\nMEASURE\n", qerr.CodeArgType},
		{"fn f() -> int {\n}\nH 0\nMEASURE\n", qerr.CodeMissingReturn},
		{"fn f() -> int {\n    return 1.0\n}\nH 0\nMEASURE\n", qerr.CodeReturnType},
		{"fn f(n: int) -> int {\n    return f(n)\n}\nH 0\nMEASURE\n", qerr.CodeRecursion},
		{"fn a() -> int {\n    return b()\n}\nfn b() -> int {\n    return a()\n}\nH 0\nMEASURE\n", qerr.CodeRecursion},
		{"fn f(x: int, x: int) -> int {\n    return x\n}\nH 0\nMEASURE\n", qerr.CodeDupParam},
		{"PARAM theta\nfn f(theta: float) -> float {\n    return theta\n}\nRX theta 0\nMEASURE\n", qerr.CodeParamClash},
		{"fn f() -> int {\n    return theta\n}\nPARAM theta\nRX theta 0\nMEASURE\n", qerr.CodeUnknownIdent},
		{"fn missing() -> int {\n    return noSuch()\n}\nH 0\nMEASURE\n", qerr.CodeUnknownFunc},
		{"fn f(x: int) -> int {\n    x = 2\n    return x\n}\nH 0\nMEASURE\n", qerr.CodeImmutable},
	}
	for _, tc := range cases {
		c, err := parser.Parse(tc.src)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.src, err)
		}
		ds := Check(c)
		if !strings.Contains(codes(ds), tc.code) {
			t.Fatalf("%s: got %s want %s (%v)", tc.src, codes(ds), tc.code, ds)
		}
		d := ds[0]
		if d.Line < 1 || d.Column < 1 || d.EndColumn < d.Column || d.Message == "" || d.SuggestedFix == "" {
			t.Fatalf("incomplete diagnostic %+v", d)
		}
		raw, err := d.JSON()
		if err != nil || !strings.Contains(string(raw), tc.code) {
			t.Fatalf("json %s: %s", tc.code, raw)
		}
	}
}

func TestFunctionShadowAndScope(t *testing.T) {
	src := "let base: int = 10\nfn f(x: int) -> int {\n    let y: int = x + base\n    return y\n}\nlet n: int = f(1)\nH 0\nMEASURE\n"
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	v, ds := EvalIn(c, c.Host[1].Expr)
	if len(ds) != 0 || v.Int != 11 {
		t.Fatalf("%+v %v", v, ds)
	}
	later := "fn f() -> int {\n    return base\n}\nlet base: int = 1\nlet n: int = f()\nH 0\nMEASURE\n"
	c = parseOK(t, later)
	ds = Check(c)
	if !strings.Contains(codes(ds), qerr.CodeUnknownIdent) {
		t.Fatal(ds)
	}
}
