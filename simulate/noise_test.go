// Copyright 2026 Magnobit, Inc. All rights reserved.

package simulate

import (
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func TestNoise_DepolarizingBreaksBell(t *testing.T) {
	src := "H 0\nCNOT 0 1\nMEASURE\n"
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	prog := ir.Lower(c)
	res, err := RunProgramOpts(prog, Options{
		Shots: 2000,
		Seed:  42,
		Noise: NoiseModel{Depolarizing: 0.15},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Ideal Bell only has 00/11; with strong depolarizing we expect leakage.
	leak := 0
	for k, v := range res.Counts {
		if k != "00" && k != "11" {
			leak += v
		}
	}
	if leak == 0 {
		t.Fatalf("expected some off-diagonal outcomes with depolarizing noise, got %v", res.Counts)
	}
}

func TestNoise_FromSourceDirective(t *testing.T) {
	src := "NOISE depolarizing 0.2\nX 0\nMEASURE\n"
	res, err := RunProgramOpts(mustLower(t, src), Options{Shots: 500, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	// Pure X → always |1⟩; noise should flip some shots toward |0⟩.
	if res.Counts["0"] == 0 {
		t.Fatalf("expected some |0> counts from noise, got %v", res.Counts)
	}
}

func TestNoise_ReadoutFlipsBits(t *testing.T) {
	src := "X 0\nMEASURE\n"
	res, err := RunProgramOpts(mustLower(t, src), Options{
		Shots: 500,
		Seed:  3,
		Noise: NoiseModel{ReadoutError: 0.2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["0"] == 0 {
		t.Fatalf("readout should flip some |1>→|0>, got %v", res.Counts)
	}
}

func TestNoise_PhaseDamping(t *testing.T) {
	src := "H 0\nMEASURE\n"
	_, err := RunProgramOpts(mustLower(t, src), Options{
		Shots: 200,
		Seed:  1,
		Noise: NoiseModel{PhaseDamping: 0.3},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustLower(t *testing.T, src string) *ir.Program {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return ir.Lower(c)
}
