// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func TestHostIfStmtValues(t *testing.T) {
	src := strings.Join([]string{
		"fn abs(x: int) -> int {",
		"    if x < 0 {",
		"        return -x",
		"    } else {",
		"        return x",
		"    }",
		"}",
		"fn category(x: int) -> int {",
		"    if x < 0 {",
		"        return -1",
		"    } else {",
		"        if x == 0 {",
		"            return 0",
		"        } else {",
		"            return 1",
		"        }",
		"    }",
		"}",
		"fn pick(flag: bool) -> int {",
		"    if flag {",
		"        return 1",
		"    } else {",
		"        return 1 / 0",
		"    }",
		"}",
		"fn both(x: int) -> int {",
		"    let s: int = if x < 0 { -1 } else { 1 }",
		"    if s < 0 {",
		"        return -x",
		"    } else {",
		"        return x",
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
	if callInt(t, c, "abs(-3)") != 3 || callInt(t, c, "abs(4)") != 4 {
		t.Fatal("abs")
	}
	if callInt(t, c, "category(-4)") != -1 || callInt(t, c, "category(0)") != 0 || callInt(t, c, "category(2)") != 1 {
		t.Fatal("category")
	}
	if callInt(t, c, "pick(true)") != 1 {
		t.Fatal("pick")
	}
	if callInt(t, c, "both(-3)") != 3 {
		t.Fatal("both")
	}
}

func TestHostIfStmtLocals(t *testing.T) {
	src := strings.Join([]string{
		"fn f(y: int) -> int {",
		"    if y < 0 {",
		"        let y: int = 4",
		"        return y",
		"    } else {",
		"        let y: int = y + 1",
		"        return y",
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
	if callInt(t, c, "f(-1)") != 4 || callInt(t, c, "f(2)") != 3 {
		t.Fatal("locals")
	}
}

func TestHostIfStmtFallthroughReturn(t *testing.T) {
	src := strings.Join([]string{
		"fn sign(x: int) -> int {",
		"    if x < 0 {",
		"        return -1",
		"    }",
		"    return 1",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c := parseOK(t, src)
	if ds := Check(c); len(ds) != 0 {
		t.Fatal(ds)
	}
	if callInt(t, c, "sign(-2)") != -1 || callInt(t, c, "sign(2)") != 1 {
		t.Fatal("sign")
	}
}

func TestHostIfStmtDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"non-bool", "fn f(x: int) -> int {\n    if x {\n        return x\n    } else {\n        return 0\n    }\n}\nH 0\nMEASURE\n", qerr.CodeIfCond},
		{"missing path", "fn sign(x: int) -> int {\n    if x < 0 {\n        return -1\n    }\n}\nH 0\nMEASURE\n", qerr.CodeIfPaths},
		{"nested missing path", "fn category(x: int) -> int {\n    if x < 0 {\n        return -1\n    } else {\n        if x == 0 {\n            return 0\n        }\n    }\n}\nH 0\nMEASURE\n", qerr.CodeIfPaths},
		{"other branch local", "fn f(x: int) -> int {\n    if x < 0 {\n        let y: int = 1\n        return y\n    } else {\n        return y\n    }\n}\nH 0\nMEASURE\n", qerr.CodeUnknownIdent},
		{"param shadow", "PARAM theta\nfn f(x: int) -> int {\n    if x < 0 {\n        let theta: int = 1\n        return theta\n    } else {\n        return x\n    }\n}\nH 0\nMEASURE\n", qerr.CodeParamClash},
		{"wrong return type", "fn f(x: bool) -> int {\n    if x {\n        return 1\n    } else {\n        return 1.0\n    }\n}\nH 0\nMEASURE\n", qerr.CodeReturnType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := parseOK(t, tc.src)
			ds := Check(c)
			if !hasCode(ds, tc.code) {
				t.Fatalf("%+v", ds)
			}
			raw, err := qerr.DiagnosticsJSON(ds)
			if err != nil || !strings.Contains(string(raw), tc.code) || !strings.Contains(string(raw), "docsURL") {
				t.Fatalf("json: %s %v", raw, err)
			}
		})
	}
}

func TestHostIfStmtUnreachableIsWarning(t *testing.T) {
	src := "fn f() -> int {\n    return 1\n    let x: int = 2\n}\nH 0\nMEASURE\n"
	c := parseOK(t, src)
	ds := Check(c)
	if !hasCode(ds, qerr.CodeUnreachable) {
		t.Fatalf("%+v", ds)
	}
	for _, d := range ds {
		if d.Code == qerr.CodeUnreachable && d.Severity != qerr.SeverityWarning {
			t.Fatalf("severity: %+v", d)
		}
		if d.Severity == qerr.SeverityError || d.Severity == "" {
			t.Fatalf("error leaked: %+v", d)
		}
	}
	if err := Fail(c); err != nil {
		t.Fatal(err)
	}
}

func TestHostIfStmtConstTiming(t *testing.T) {
	unknown := "fn f(x: int) -> int {\n    if x < 0 {\n        return 1 / 0\n    } else {\n        return x\n    }\n}\nH 0\nMEASURE\n"
	c := parseOK(t, unknown)
	if ds := Check(c); hasCode(ds, qerr.CodeDivZero) {
		t.Fatalf("untaken constant check: %+v", ds)
	}
	known := "fn bad() -> int {\n    if true {\n        return 1 / 0\n    } else {\n        return 1\n    }\n}\nH 0\nMEASURE\n"
	c = parseOK(t, known)
	if ds := Check(c); !hasCode(ds, qerr.CodeDivZero) {
		t.Fatalf("taken constant check: %+v", ds)
	}
}

func callInt(t *testing.T, c *parser.Circuit, expr string) int64 {
	t.Helper()
	e, err := parser.ParseExpr(expr, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	return v.Int
}

func hasCode(ds []qerr.Diagnostic, code string) bool {
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}
