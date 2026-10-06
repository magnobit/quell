// Copyright 2026 Magnobit, Inc. All rights reserved.

package bench

import (
	"fmt"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

// Profile benchmarks parse and lower once, then time the pure-Go simulator.
// They do not change the simulator.

func lower(b *testing.B, src string) *ir.Program {
	b.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	return ir.Lower(c)
}

func hadamards(n int) string {
	var b strings.Builder
	for q := 0; q < n; q++ {
		fmt.Fprintf(&b, "H %d\n", q)
	}
	b.WriteString("MEASURE\n")
	return b.String()
}

func cnots(n int) string {
	var b strings.Builder
	b.WriteString("H 0\n")
	for q := 1; q < n; q++ {
		fmt.Fprintf(&b, "CNOT 0 %d\n", q)
	}
	b.WriteString("MEASURE\n")
	return b.String()
}

func rotations(n int) string {
	var b strings.Builder
	for q := 0; q < n; q++ {
		fmt.Fprintf(&b, "RX 0.1 %d\nRY 0.2 %d\nRZ 0.3 %d\n", q, q, q)
	}
	b.WriteString("MEASURE\n")
	return b.String()
}

func BenchmarkProfileCopy(b *testing.B) {
	for _, n := range []int{12, 16, 20} {
		sv := simulate.New(n)
		b.Run(fmt.Sprintf("%dq", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				amps := sv.Amplitudes()
				if len(amps) != 1<<n {
					b.Fatal(len(amps))
				}
			}
		})
	}
}

func BenchmarkProfileGate1Q(b *testing.B) {
	for _, n := range []int{12, 16, 20} {
		p := lower(b, hadamards(n))
		b.Run(fmt.Sprintf("%dq", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := simulate.EvolveUnitary(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProfileControlled(b *testing.B) {
	for _, n := range []int{12, 16, 20} {
		p := lower(b, cnots(n))
		b.Run(fmt.Sprintf("ghz-%dq", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := simulate.EvolveUnitary(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProfileSample(b *testing.B) {
	for _, n := range []int{12, 16} {
		p := lower(b, hadamards(n))
		b.Run(fmt.Sprintf("%dq-1024shots", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := simulate.RunProgram(p, 1024); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProfileReset(b *testing.B) {
	src := "H 0\nRESET 0\nH 0\nMEASURE\n"
	p := lower(b, src)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := simulate.RunProgram(p, 256); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProfileNoise(b *testing.B) {
	src := "NOISE depolarizing 0.01\n" + hadamards(8)
	p := lower(b, src)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := simulate.RunProgram(p, 32); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProfileDynamic(b *testing.B) {
	src := "H 0\nMEASURE 0\nIF c[0]==1 {\n  X 1\n}\nMEASURE\n"
	p := lower(b, src)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := simulate.RunProgram(p, 64); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProfileRotation(b *testing.B) {
	for _, n := range []int{12, 16, 20} {
		p := lower(b, rotations(n))
		b.Run(fmt.Sprintf("%dq", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := simulate.EvolveUnitary(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProfileGHZShots(b *testing.B) {
	for _, n := range []int{12, 16, 20} {
		p := lower(b, cnots(n))
		shots := 32
		if n >= 20 {
			shots = 8
		}
		b.Run(fmt.Sprintf("%dq-%dshots", n, shots), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := simulate.RunProgram(p, shots); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
