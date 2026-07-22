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

func TestToQuell_OpenQASM2Bell(t *testing.T) {
	src := `OPENQASM 2.0;
include "qelib1.inc";

qreg q[2];
creg c[2];

h q[0];
cx q[0], q[1];
measure q -> c;
`
	out, err := ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"H 0", "CNOT 0 1", "MEASURE"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
}

func TestToQuell_IfElseBrace(t *testing.T) {
	src := `OPENQASM 3.0;
qubit[2] q;
bit[2] c;
h q[0];
c[0] = measure q[0];
if (c[0] == 1) {
  x q[1];
  h q[1];
} else {
  z q[1];
}
`
	out, err := ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"IF c[0]==1", "X 1", "H 1", "IF c[0]==0", "Z 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
}

func TestToQuell_IfElseSameLine(t *testing.T) {
	src := `OPENQASM 3.0;
qubit[2] q;
bit[2] c;
h q[0];
c[0] = measure q[0];
if (c[0] == 1) { x q[1]; } else { z q[1]; }
`
	out, err := ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"IF c[0]==1 X 1", "IF c[0]==0 Z 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "unsupported") {
		t.Fatalf("unexpected unsupported:\n%s", out)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
}

func TestToQuell_RXX(t *testing.T) {
	src := `OPENQASM 3.0;
qubit[2] q;
rxx(pi/2) q[0], q[1];
measure q;
`
	out, err := ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CNOT 0 1") || !strings.Contains(out, "RZ ") {
		t.Fatalf("expected RXX decomp:\n%s", out)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
}

func TestToQuell_OpenQASM2RegisterIf(t *testing.T) {
	src := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[1];
h q[0];
measure q[0] -> c[0];
if(c==1) x q[1];
`
	out, err := ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "IF c[0]==1 X 1") {
		t.Fatalf("expected register if → IF c[0]==1:\n%s", out)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
}
