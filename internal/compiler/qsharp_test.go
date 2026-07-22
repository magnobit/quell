package compiler_test

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/compile"
)

func TestCompileQSharp(t *testing.T) {
	src := "H 0\nCNOT 0 1\nMEASURE\n"
	out, err := compile.Compile(src, compile.QSharp)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"namespace QuellExport",
		"H(qs[0])",
		"CNOT(qs[0], qs[1])",
		"operation Run()",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
