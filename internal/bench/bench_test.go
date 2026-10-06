// Copyright 2026 Magnobit, Inc. All rights reserved.

package bench

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/estimate"
	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qasm"
	"github.com/magnobit/quell/qerr"
	"github.com/magnobit/quell/simulate"
)

func ghz(n int) string {
	var b strings.Builder
	b.WriteString("H 0\n")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "CNOT 0 %d\n", i)
	}
	b.WriteString("MEASURE\n")
	return b.String()
}

func corpus() map[string]string {
	var rot strings.Builder
	for q := 0; q < 8; q++ {
		fmt.Fprintf(&rot, "RX 0.1 %d\nRY 0.2 %d\nRZ 0.3 %d\n", q, q, q)
	}
	rot.WriteString("MEASURE\n")
	var cliff strings.Builder
	for q := 0; q < 8; q++ {
		fmt.Fprintf(&cliff, "H %d\nS %d\n", q, q)
	}
	cliff.WriteString("CNOT 0 1\nCNOT 2 3\nCNOT 4 5\nCNOT 6 7\nMEASURE\n")
	var qasmB strings.Builder
	qasmB.WriteString("OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[8];\ncreg c[8];\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&qasmB, "h q[%d];\n", i%8)
	}
	qasmB.WriteString("measure q -> c;\n")
	return map[string]string{
		"bell2":         "H 0\nCNOT 0 1\nMEASURE\n",
		"ghz3":          ghz(3),
		"ghz5":          ghz(5),
		"ghz10":         ghz(10),
		"ghz12":         ghz(12),
		"ghz16":         ghz(16),
		"rotation":      rot.String(),
		"clifford":      cliff.String(),
		"parameterized": "PARAM theta : angle\nRX theta 0\nCNOT 0 1\nMEASURE\n",
		"dynamic":       "H 0\nMEASURE 0\nIF c[0]==1 X 1\nMEASURE\n",
		"openqasm":      qasmB.String(),
		"example":       "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n",
	}
}

func BenchmarkParse(b *testing.B) {
	for name, src := range corpus() {
		if name == "openqasm" || name == "parameterized" {
			continue
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := parser.Parse(src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLower(b *testing.B) {
	src := ghz(10)
	circ, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ir.Lower(circ)
	}
}

func BenchmarkOptimizer(b *testing.B) {
	circ, err := parser.Parse(ghz(10))
	if err != nil {
		b.Fatal(err)
	}
	prog := ir.Lower(circ)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = optimizer.Optimize(prog)
	}
}

func BenchmarkCompileSharedVsRepeat(b *testing.B) {
	src := ghz(5)
	b.Run("shared", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := compile.CompileMany(src, compile.Targets, true); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("per-target", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, target := range compile.Targets {
				if _, err := compile.CompileWithWarnings(src, target, true); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

func BenchmarkCompile(b *testing.B) {
	src := ghz(5)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := compile.CompileMany(src, compile.Targets, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind(b *testing.B) {
	src := "PARAM theta : angle\nRX theta 0\nRY theta 1\nMEASURE\n"
	sets := []map[string]float64{{"theta": 0.1}, {"theta": 0.2}, {"theta": 0.3}}
	b.Run("reparse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, set := range sets {
				if _, err := estimate.BindSource(src, set); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("prepare-once", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := estimate.BindMany(src, sets); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkSimulate(b *testing.B) {
	src := ghz(10)
	b.Run("evolve", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := simulate.Run(src, 1); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("shots", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := simulate.Run(src, 32); err != nil {
				b.Fatal(err)
			}
		}
	})
	wide := ghz(16)
	b.Run("ghz16-evolve", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := simulate.Run(wide, 1); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkImport(b *testing.B) {
	src := corpus()["openqasm"]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := qasm.ToQuell(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCLIColdStart(b *testing.B) {
	bin := os.Getenv("QUELL_BENCH_BIN")
	if bin == "" {
		b.Skip("set QUELL_BENCH_BIN to a quell binary to measure process start")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin, "version")
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("%v: %s", err, out)
		}
	}
}

func hostLets(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "let v%d: int = %d\n", i, i)
	}
	b.WriteString("H 0\nMEASURE\n")
	return b.String()
}

func BenchmarkHostLets(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		src := hostLets(n)
		b.Run(fmt.Sprintf("parse-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := parser.Parse(src); err != nil {
					b.Fatal(err)
				}
			}
		})
		circ, err := parser.Parse(src)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("check-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if ds := check.Check(circ); len(ds) != 0 {
					b.Fatal(ds)
				}
			}
		})
		b.Run(fmt.Sprintf("eval-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			env := map[string]check.Value{}
			for i := 0; i < b.N; i++ {
				for _, h := range circ.Host {
					v, ds := check.Eval(h.Expr, env, h.Line, nil)
					if len(ds) != 0 {
						b.Fatal(ds)
					}
					env[h.Name] = v
				}
				env = map[string]check.Value{}
			}
		})
	}
	bad := "let x: int = missing\nH 0\nMEASURE\n"
	circ, err := parser.Parse(bad)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("diagnostic-json", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw, err := qerr.DiagnosticsJSON(check.Check(circ))
			if err != nil || len(raw) == 0 {
				b.Fatal(err)
			}
		}
	})
}

func hostFunctions(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "fn f%d(x: int) -> int {\n    return x + %d\n}\n", i, i)
	}
	b.WriteString("let n: int = f0(1)\nH 0\nMEASURE\n")
	return b.String()
}

func BenchmarkHostFunction(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		src := hostFunctions(n)
		b.Run(fmt.Sprintf("parse-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := parser.Parse(src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	src := hostFunctions(100)
	circ, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("signatures-100", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			names, ds := check.Signatures(circ)
			if len(ds) != 0 || len(names) != 100 {
				b.Fatal(ds)
			}
		}
	})
	b.Run("check-100", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if ds := check.Check(circ); len(ds) != 0 {
				b.Fatal(ds)
			}
		}
	})
	shallow, err := parser.Parse("fn add(a: int, b: int) -> int {\n    return a + b\n}\nH 0\nMEASURE\n")
	if err != nil {
		b.Fatal(err)
	}
	call, err := parser.ParseExpr("add(2, 3)", 1, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("shallow-call", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, ds := check.EvalIn(shallow, call)
			if len(ds) != 0 || v.Int != 5 {
				b.Fatal(ds)
			}
		}
	})
	nested, err := parser.Parse("fn inc(x: int) -> int {\n    return x + 1\n}\nfn twice(x: int) -> int {\n    return inc(inc(x))\n}\nH 0\nMEASURE\n")
	if err != nil {
		b.Fatal(err)
	}
	nestCall, err := parser.ParseExpr("twice(3)", 1, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("nested-call", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, ds := check.EvalIn(nested, nestCall)
			if len(ds) != 0 || v.Int != 5 {
				b.Fatal(ds)
			}
		}
	})
	bad, err := parser.Parse("fn f(n: int) -> int {\n    return f(n)\n}\nH 0\nMEASURE\n")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("diagnostic-json", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw, err := qerr.DiagnosticsJSON(check.Check(bad))
			if err != nil || len(raw) == 0 {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkHostIf(b *testing.B) {
	nested := hostIfNested(8)
	b.Run("parse-nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := parser.Parse(nested); err != nil {
				b.Fatal(err)
			}
		}
	})
	circ, err := parser.Parse(nested)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("check-nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if ds := check.Check(circ); len(ds) != 0 {
				b.Fatal(ds)
			}
		}
	})
	selected, err := parser.Parse("let x: int = if true { 1 } else { 1 / 0 }\nH 0\nMEASURE\n")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("eval-selected", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, ds := check.EvalIn(selected, selected.Host[0].Expr)
			if len(ds) != 0 || v.Int != 1 {
				b.Fatal(ds)
			}
		}
	})
	call, err := parser.ParseExpr("nest(3)", 1, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("eval-nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, ds := check.EvalIn(circ, call)
			if len(ds) != 0 || v.Int != 3 {
				b.Fatal(v, ds)
			}
		}
	})
	badIf, err := parser.Parse("let x: int = if 1 { 1 } else { 2 }\nH 0\nMEASURE\n")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("diagnostic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw, err := qerr.DiagnosticsJSON(check.Check(badIf))
			if err != nil || !strings.Contains(string(raw), "QL2301") {
				b.Fatal(err, string(raw))
			}
		}
	})
}

func BenchmarkHostIfStmt(b *testing.B) {
	abs := "fn abs(x: int) -> int {\n    if x < 0 {\n        return -x\n    } else {\n        return x\n    }\n}\nH 0\nMEASURE\n"
	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := parser.Parse(abs); err != nil {
				b.Fatal(err)
			}
		}
	})
	circ, err := parser.Parse(abs)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("check-return-paths", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if ds := check.Check(circ); len(ds) != 0 {
				b.Fatal(ds)
			}
		}
	})
	nestedSrc := "fn category(x: int) -> int {\n    if x < 0 {\n        return -1\n    } else {\n        if x == 0 {\n            return 0\n        } else {\n            return 1\n        }\n    }\n}\nH 0\nMEASURE\n"
	nested, err := parser.Parse(nestedSrc)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("check-nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if ds := check.Check(nested); len(ds) != 0 {
				b.Fatal(ds)
			}
		}
	})
	call, err := parser.ParseExpr("abs(-3)", 1, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("eval-selected", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, ds := check.EvalIn(circ, call)
			if len(ds) != 0 || v.Int != 3 {
				b.Fatal(v, ds)
			}
		}
	})
	nestedCall, err := parser.ParseExpr("category(2)", 1, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("eval-nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, ds := check.EvalIn(nested, nestedCall)
			if len(ds) != 0 || v.Int != 1 {
				b.Fatal(v, ds)
			}
		}
	})
	bad, err := parser.Parse("fn sign(x: int) -> int {\n    if x < 0 {\n        return -1\n    }\n}\nH 0\nMEASURE\n")
	if err != nil {
		b.Fatal(err)
	}
	b.Run("diagnostic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw, err := qerr.DiagnosticsJSON(check.Check(bad))
			if err != nil || !strings.Contains(string(raw), "QL2305") {
				b.Fatal(err, string(raw))
			}
		}
	})
}

func hostIfNested(depth int) string {
	var b strings.Builder
	b.WriteString("fn nest(x: int) -> int {\n    return ")
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&b, "if x > %d {\n", i)
	}
	b.WriteString("x\n")
	for i := depth - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "} else {\n    %d\n}\n", i)
	}
	b.WriteString("}\nH 0\nMEASURE\n")
	return b.String()
}
