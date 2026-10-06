// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func TestHostArrays(t *testing.T) {
	src := strings.Join([]string{
		"fn total(xs: int[]) -> int {",
		"    var s: int = 0",
		"    for i in 0..len(xs) - 1 {",
		"        s = s + xs[i]",
		"    }",
		"    return s",
		"}",
		"fn bump() -> int {",
		"    var xs: int[] = [1, 2, 3]",
		"    xs[0] = 5",
		"    return xs[0]",
		"}",
		"fn same() -> bool {",
		"    let a: int[] = [1, 2]",
		"    let b: int[] = [1, 2]",
		"    return a == b",
		"}",
		"let n: int = total([1, 2, 3])",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	if callInt(t, c, "total([1, 2, 3])") != 6 {
		t.Fatal(callInt(t, c, "total([1, 2, 3])"))
	}
	if callInt(t, c, "bump()") != 5 {
		t.Fatal("bump")
	}
	e, err := parser.ParseExpr("same()", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) != 0 || !v.Bool {
		t.Fatal(v, ds)
	}
}

func TestHostArrayDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"const index", "fn f() -> int {\n    let xs: int[] = [1, 2]\n    return xs[4]\n}\nH 0\nMEASURE\n", qerr.CodeIndex},
		{"immutable element", "fn f() -> int {\n    let xs: int[] = [1]\n    xs[0] = 2\n    return xs[0]\n}\nH 0\nMEASURE\n", qerr.CodeImmutable},
		{"element type", "fn f() -> int {\n    let xs: int[] = [1, true]\n    return xs[0]\n}\nH 0\nMEASURE\n", qerr.CodeArrayType},
		{"len type", "fn f() -> int {\n    return len(1)\n}\nH 0\nMEASURE\n", qerr.CodeArrayType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := parseOK(t, tc.src)
			if !hasCode(Check(c), tc.code) {
				t.Fatal(Check(c))
			}
		})
	}
}

func TestRuntimeIndex(t *testing.T) {
	src := "fn f(i: int) -> int {\n    let xs: int[] = [1, 2]\n    return xs[i]\n}\nH 0\nMEASURE\n"
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr("f(9)", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, ds := EvalIn(c, e)
	if !hasCode(ds, qerr.CodeIndex) {
		t.Fatal(ds)
	}
}
