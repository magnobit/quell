// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/host"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func TestHostLoopAndVar(t *testing.T) {
	src := strings.Join([]string{
		"fn sum(n: int) -> int {",
		"    var total: int = 0",
		"    for i in 1..n {",
		"        total = total + i",
		"    }",
		"    return total",
		"}",
		"fn early(n: int) -> int {",
		"    var seen: int = 0",
		"    for i in 0..n {",
		"        if i == 2 {",
		"            break",
		"        } else {",
		"            seen = seen + 1",
		"        }",
		"    }",
		"    return seen",
		"}",
		"fn always() -> int {",
		"    while true {",
		"        return 7",
		"    }",
		"}",
		"fn say() -> string {",
		"    println(\"shots\", 3)",
		"    return format(\"{:.1f}\", 1.5)",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	if callInt(t, c, "sum(3)") != 6 {
		t.Fatal(callInt(t, c, "sum(3)"))
	}
	if callInt(t, c, "early(5)") != 2 {
		t.Fatal(callInt(t, c, "early(5)"))
	}
	if callInt(t, c, "always()") != 7 {
		t.Fatal("always")
	}
	buf := &host.Buffer{}
	host.SetWriter(buf)
	defer host.SetWriter(nil)
	v := callString(t, c, "say()")
	if v != "1.5" {
		t.Fatal(v)
	}
	if buf.String() != "shots 3\n" {
		t.Fatalf("output %q", buf.String())
	}
	src2 := "fn say(energy: float, shots: int) -> int {\n    println(\"Energy = {energy:.4f}\")\n    println(\"Shots = {shots}\")\n    return shots\n}\nH 0\nMEASURE\n"
	c2 := parseOK(t, src2)
	if ds := Check(c2); len(ds) != 0 {
		t.Fatal(ds)
	}
	buf.Reset()
	if callInt(t, c2, "say(1.5, 3)") != 3 {
		t.Fatal("say")
	}
	if buf.String() != "Energy = 1.5000\nShots = 3\n" {
		t.Fatalf("interp %q", buf.String())
	}
}

func TestHostLoopDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"maybe skip", "fn f(n: int) -> int {\n    while n < 0 {\n        return 1\n    }\n}\nH 0\nMEASURE\n", qerr.CodeIfPaths},
		{"break outside", "fn f() -> int {\n    break\n    return 1\n}\nH 0\nMEASURE\n", qerr.CodeBreak},
		{"immutable", "fn f() -> int {\n    let n: int = 1\n    n = 2\n    return n\n}\nH 0\nMEASURE\n", qerr.CodeImmutable},
		{"bad format", "fn f() -> string {\n    return format(\"{:d}\", 1.5)\n}\nH 0\nMEASURE\n", qerr.CodeFormatType},
		{"bad interp", "fn f() -> int {\n    println(\"{:d}\")\n    return 1\n}\nH 0\nMEASURE\n", qerr.CodeInterp},
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

func callString(t *testing.T, c *parser.Circuit, expr string) string {
	t.Helper()
	e, err := parser.ParseExpr(expr, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	return v.Str
}
