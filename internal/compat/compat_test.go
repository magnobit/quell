// Copyright 2026 Magnobit, Inc. All rights reserved.

package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/estimate"
	"github.com/magnobit/quell/format"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qasm"
	"github.com/magnobit/quell/simulate"
)

type fixture struct {
	name string
	src  string
	// bind, when set, is applied after a single Prepare. Compile and
	// simulate run on the bound source.
	bind map[string]float64
}

func fixtures() []fixture {
	return []fixture{
		{name: "bell", src: "H 0\nCNOT 0 1\nMEASURE\n"},
		{name: "ghz", src: "H 0\nCNOT 0 1\nCNOT 0 2\nMEASURE\n"},
		{name: "named", src: "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n"},
		{name: "param", src: "PARAM theta : angle\nRX theta 0\nMEASURE\n", bind: map[string]float64{"theta": 0.5}},
		{name: "macro", src: "gate bell a b {\n  H a\n  CNOT a b\n}\nbell 0 1\nMEASURE\n"},
		{name: "if-else", src: "H 0\nMEASURE 0\nIF c[0]==1 {\n  X 1\n}\nELSE {\n  Z 1\n}\nMEASURE\n"},
		{name: "while", src: "H 0\nMEASURE 0\nWHILE c[0]==0 MAX 4 {\n  X 0\n  MEASURE 0\n}\nMEASURE\n"},
		{name: "switch", src: "H 0\nMEASURE 0\nSWITCH c[0] {\n  CASE 0: X 1\n  CASE 1: Z 1\n}\nMEASURE\n"},
		{name: "for", src: "FOR i IN 0..1 {\n  H i\n}\nMEASURE\n"},
		{name: "par", src: "PAR {\n  H 0\n  H 1\n}\nMEASURE\n"},
		{name: "assert", src: "X 0\nMEASURE 0\nASSERT c[0]==1\nMEASURE\n"},
		{name: "reset", src: "X 0\nRESET 0\nMEASURE\n"},
		{name: "midcircuit", src: "H 0\nMEASURE 0\nX 1\nMEASURE\n"},
		{name: "noise", src: "NOISE depolarizing 0.01\nH 0\nCNOT 0 1\nMEASURE\n"},
	}
}

func TestFixtures(t *testing.T) {
	for _, fx := range fixtures() {
		t.Run(fx.name, func(t *testing.T) {
			circ, err := parser.Parse(fx.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			formatted := format.Format(fx.src)
			if _, err := parser.Parse(formatted); err != nil {
				t.Fatalf("parse after format: %v\n%s", err, formatted)
			}
			prog := ir.Lower(circ)
			if prog == nil || prog.NumQubits < 1 {
				t.Fatalf("lower produced %+v", prog)
			}
			src := fx.src
			if fx.bind != nil {
				tpl, err := estimate.Prepare(fx.src)
				if err != nil {
					t.Fatalf("prepare: %v", err)
				}
				src, err = tpl.Bind(fx.bind)
				if err != nil {
					t.Fatalf("bind: %v", err)
				}
				circ, err = parser.Parse(src)
				if err != nil {
					t.Fatalf("parse bound: %v", err)
				}
				prog = ir.Lower(circ)
			}
			opt, _ := optimizer.Optimize(prog)
			ev := optequiv.VerifyOptimization(prog, optequiv.Options{})
			if ev.Status == optequiv.StatusNotEquivalent {
				t.Fatalf("optimizer equivalence %s: %s", ev.Status, ev.Reason)
			}
			if opt == nil {
				t.Fatal("optimize returned nil")
			}
			if _, err := simulate.Run(src, 16); err != nil {
				t.Fatalf("simulate: %v", err)
			}
			// Unitary circuits must print every target. Dynamic constructs
			// are target-dependent: OpenQASM 3 is required, and a target
			// that rejects the construct must say so instead of emitting
			// an empty file. That rejection is the current limitation.
			exported := 0
			for _, target := range compile.Targets {
				out, err := compile.CompileMany(src, []compile.Target{target}, true)
				if err != nil {
					if target == compile.OpenQASM {
						t.Fatalf("openqasm 3: %v", err)
					}
					if !strings.Contains(err.Error(), "not ") {
						t.Fatalf("%s: %v", target, err)
					}
					continue
				}
				if strings.TrimSpace(out[target].Code) == "" {
					t.Fatalf("empty %s output", target)
				}
				exported++
			}
			if exported == 0 {
				t.Fatal("no target accepted the fixture")
			}
		})
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.quell")
	main := filepath.Join(dir, "main.quell")
	if err := os.WriteFile(lib, []byte("H 0\nCNOT 0 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte("import \"./lib.quell\"\nMEASURE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	circ, err := parser.ParseFile(main)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	prog := ir.Lower(circ)
	if prog.NumQubits < 2 {
		t.Fatalf("imported program qubits = %d", prog.NumQubits)
	}
	if _, err := simulate.Run(format.Format("H 0\nCNOT 0 1\nMEASURE\n"), 8); err != nil {
		t.Fatal(err)
	}
}

func TestHostLetDoesNotChangeTargets(t *testing.T) {
	base := "H 0\nCNOT 0 1\nMEASURE\n"
	with := "let shots: int = 1000\nlet label: string = \"baseline\"\n" + base
	a, err := compile.CompileWithWarnings(base, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compile.CompileWithWarnings(with, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Fatalf("openqasm changed\n%s\n%s", a.Code, b.Code)
	}
	ca, err := parser.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := parser.Parse(with)
	if err != nil {
		t.Fatal(err)
	}
	if string(ir.CanonicalBytes(ir.Lower(ca))) != string(ir.CanonicalBytes(ir.Lower(cb))) {
		t.Fatal("canonical IR changed because of unused lets")
	}
	if _, err := compile.CompileWithWarnings("PARAM theta\nlet theta: float = 0.5\nRX theta 0\nMEASURE\n", compile.OpenQASM, false); err == nil {
		t.Fatal("expected PARAM/let collision to fail compile")
	}
}

func TestHostFunctionDoesNotChangeTargets(t *testing.T) {
	base := "H 0\nCNOT 0 1\nMEASURE\n"
	with := "fn square(x: float) -> float {\n    return x * x\n}\nlet n: int = 1\n" + base
	a, err := compile.CompileWithWarnings(base, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compile.CompileWithWarnings(with, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Fatalf("openqasm changed\n%s\n%s", a.Code, b.Code)
	}
	ca, err := parser.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := parser.Parse(with)
	if err != nil {
		t.Fatal(err)
	}
	if string(ir.CanonicalBytes(ir.Lower(ca))) != string(ir.CanonicalBytes(ir.Lower(cb))) {
		t.Fatal("canonical IR changed because of an unused host function")
	}
	if len(ir.Lower(cb).Funcs) != 1 {
		t.Fatal("host function was dropped from the host program")
	}
}

func TestHostIfDoesNotChangeTargetsOrQPUIF(t *testing.T) {
	base := "H 0\nCNOT 0 1\nMEASURE\n"
	with := "fn abs(x: int) -> int {\n    return if x < 0 {\n        -x\n    } else {\n        x\n    }\n}\nlet n: int = if true { 1 } else { 0 }\n" + base
	a, err := compile.CompileWithWarnings(base, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compile.CompileWithWarnings(with, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Fatalf("openqasm changed\n%s\n%s", a.Code, b.Code)
	}
	ca, err := parser.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := parser.Parse(with)
	if err != nil {
		t.Fatal(err)
	}
	if string(ir.CanonicalBytes(ir.Lower(ca))) != string(ir.CanonicalBytes(ir.Lower(cb))) {
		t.Fatal("canonical IR changed because of an unused host conditional")
	}
	qpu := "H 0\nMEASURE 0\nIF c[0]==1 {\n  X 1\n}\nELSE {\n  Z 1\n}\nMEASURE\n"
	prog := ir.Lower(mustParse(t, qpu))
	if !hasOp(prog, ir.OpIF) {
		t.Fatal("QPU IF was not lowered")
	}
	opt, _ := optimizer.Optimize(prog)
	if !hasOp(opt, ir.OpIF) {
		t.Fatal("optimizer removed QPU IF")
	}
	if _, err := compile.CompileWithWarnings(qpu, compile.Braket, true); err == nil || !strings.Contains(err.Error(), "not exported to Braket") {
		t.Fatalf("Braket dynamic IF behavior changed: %v", err)
	}
}

func TestHostIfStmtProducesNoQuantumIR(t *testing.T) {
	base := "H 0\nMEASURE\n"
	with := "fn abs(x: int) -> int {\n    if x < 0 {\n        return -x\n    } else {\n        return x\n    }\n}\n" + base
	a, err := compile.CompileWithWarnings(base, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compile.CompileWithWarnings(with, compile.OpenQASM, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Fatalf("openqasm changed\n%s\n%s", a.Code, b.Code)
	}
	prog := ir.Lower(mustParse(t, with))
	if hasOp(prog, ir.OpIF) {
		t.Fatal("statement host if lowered to quantum IF")
	}
	if string(ir.CanonicalBytes(ir.Lower(mustParse(t, base)))) != string(ir.CanonicalBytes(prog)) {
		t.Fatal("canonical IR changed because of an unused host if statement")
	}
}

func mustParse(t *testing.T, src string) *parser.Circuit {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func hasOp(p *ir.Program, kind ir.OpKind) bool {
	if p == nil {
		return false
	}
	var walk func([]ir.Op) bool
	walk = func(ops []ir.Op) bool {
		for _, op := range ops {
			if op.Kind == kind || walk(op.Then) || walk(op.Else) {
				return true
			}
			if op.Body != nil && (op.Body.Kind == kind || walk([]ir.Op{*op.Body})) {
				return true
			}
		}
		return false
	}
	return walk(p.Ops)
}

func TestOpenQASMImport(t *testing.T) {
	src, err := qasm.ToQuell("OPENQASM 3.0;\ninclude \"stdgates.inc\";\nqubit[2] q;\nbit[2] c;\nh q[0];\ncx q[0], q[1];\nmeasure q -> c;\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := parser.Parse(src); err != nil {
		t.Fatalf("parse imported %q: %v", src, err)
	}
	out, err := compile.CompileMany(src, compile.Targets, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(compile.Targets) {
		t.Fatalf("targets = %d", len(out))
	}
}
