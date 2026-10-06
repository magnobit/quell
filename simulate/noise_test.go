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

func TestNoise_BitFlipFlipsDeterministicState(t *testing.T) {
	// X then MEASURE is always "1" when ideal. A bit-flip after the gate
	// turns some shots into "0"; probability 1 turns all of them.
	src := "X 0\nMEASURE\n"
	ideal, err := RunProgramOpts(mustLower(t, src), Options{Shots: 200, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ideal.Counts["1"] != 200 {
		t.Fatalf("ideal %v", ideal.Counts)
	}
	some, err := RunProgramOpts(mustLower(t, src), Options{Shots: 400, Seed: 1, Noise: NoiseModel{BitFlip: 0.3}})
	if err != nil {
		t.Fatal(err)
	}
	if some.Counts["0"] == 0 || some.Counts["1"] == 0 {
		t.Fatalf("bit_flip=0.3 should mix outcomes, got %v", some.Counts)
	}
	all, err := RunProgramOpts(mustLower(t, src), Options{Shots: 100, Seed: 1, Noise: NoiseModel{BitFlip: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if all.Counts["0"] != 100 {
		t.Fatalf("bit_flip=1 should always flip, got %v", all.Counts)
	}
}

func TestParseNoiseFlag_BitFlipAliasesAndRange(t *testing.T) {
	for _, s := range []string{"bit_flip=0.1", "bit-flip=0.1", "bitflip:0.1"} {
		n, err := ParseNoiseFlag(s)
		if err != nil || n.BitFlip != 0.1 {
			t.Fatalf("%s: %+v %v", s, n, err)
		}
	}
	if _, err := ParseNoiseFlag("bit_flip=1.5"); err == nil {
		t.Fatal("out-of-range bit_flip must fail")
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
