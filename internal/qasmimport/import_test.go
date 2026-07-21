// Copyright 2026 Magnobit, Inc. All rights reserved.

package qasmimport

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
)

func TestToQuell_UAndIF(t *testing.T) {
	src := `OPENQASM 3.0;
qubit[2] q;
bit[2] c;
u(pi/2, 0, pi) q[0];
crz(pi/4) q[0], q[1];
measure q[0] -> c[0];
if (c[0] == 1) x q[1];
`
	out, err := ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "U ") || !strings.Contains(out, "CRZ ") || !strings.Contains(out, "IF c[0]==1") {
		t.Fatalf("unexpected:\n%s", out)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
}
