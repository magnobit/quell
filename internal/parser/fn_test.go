// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magnobit/quell/qerr"
)

func TestParseFunction(t *testing.T) {
	src := "fn square(x: float) -> float {\n    return x * x\n}\nfn add(a: int, b: int) -> int {\n    return a + b\n}\nlet n: int = add(2, 3)\nH 0\nMEASURE\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Functions) != 2 || c.Functions[0].Name != "square" || c.Functions[1].Ret != "int" {
		t.Fatalf("%+v", c.Functions)
	}
	if len(c.Params) != 0 {
		t.Fatalf("function leaked into PARAM: %v", c.Params)
	}
	if len(c.Instructions) != 2 {
		t.Fatalf("gates: %+v", c.Instructions)
	}
	got := c.Functions[1].Canonical()
	if got != "fn add(a: int, b: int) -> int {\n    return a + b\n}" {
		t.Fatalf("canonical:\n%s", got)
	}
}

func TestParseExprCall(t *testing.T) {
	e, err := ParseExpr("add(1, 2+3)", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if e.Format(0) != "add(1, 2 + 3)" {
		t.Fatal(e.Format(0))
	}
}

func TestParseFunctionRejects(t *testing.T) {
	cases := []string{
		"fn f(x: int) -> int {\nH 0\n}\nH 0\nMEASURE\n",
		"fn f(x: int) -> int {\nfn g() -> int { return 1 }\n}\nH 0\nMEASURE\n",
		"return 1\nH 0\nMEASURE\n",
		"quantum fn f() -> int { return 1 }\nH 0\nMEASURE\n",
		"IF c[0]==1 {\nfn f() -> int { return 1 }\n}\nH 0\nMEASURE\n",
	}
	for _, src := range cases {
		_, err := Parse(src)
		if err == nil {
			t.Fatalf("expected error for %s", src)
		}
		d, ok := qerr.AsDiagnostic(err)
		if !ok || d.Code != qerr.CodeSyntax {
			t.Fatalf("%s: %v", src, err)
		}
	}
}

func TestImportSeesFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.quell"), []byte("fn add(a: int, b: int) -> int {\n    return a + b\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := "import \"./lib.quell\"\nlet n: int = add(2, 3)\nH 0\nMEASURE\n"
	if err := os.WriteFile(filepath.Join(dir, "main.quell"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := ParseFile(filepath.Join(dir, "main.quell"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Functions) != 1 || c.Functions[0].Name != "add" {
		t.Fatalf("imported function not visible: %+v", c.Functions)
	}
	if !strings.Contains(c.Host[0].Expr.Format(0), "add") {
		t.Fatal(c.Host[0].Expr.Format(0))
	}
}
