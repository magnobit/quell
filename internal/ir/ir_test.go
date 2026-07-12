// Copyright 2026 Magnobit, Inc. All rights reserved.

package ir_test

import (
	"reflect"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func TestLowerBellPair(t *testing.T) {
	c := &parser.Circuit{
		NumQubits: 2,
		Instructions: []parser.Instruction{
			{Gate: "H", Qubits: []int{0}, Line: 1},
			{Gate: "CNOT", Qubits: []int{0, 1}, Line: 2},
			{Gate: "MEASURE", Line: 3},
		},
	}

	got := ir.Lower(c)

	want := &ir.Program{
		NumQubits: 2,
		Ops: []ir.Op{
			{Kind: ir.OpH, Qubits: []int{0}},
			{Kind: ir.OpCNOT, Qubits: []int{0, 1}},
			{Kind: ir.OpMEASURE},
		},
	}

	if got.NumQubits != want.NumQubits {
		t.Fatalf("NumQubits = %d, want %d", got.NumQubits, want.NumQubits)
	}
	if !reflect.DeepEqual(got.Ops, want.Ops) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want.Ops)
	}
}

func TestLowerRotationsAndAngles(t *testing.T) {
	c := &parser.Circuit{
		NumQubits: 1,
		Instructions: []parser.Instruction{
			{Gate: "RX", Qubits: []int{0}, Args: []float64{1.5707963267948966}, Line: 1},
			{Gate: "RZ", Qubits: []int{0}, Args: []float64{3.141592653589793}, Line: 2},
			{Gate: "MEASURE", Line: 3},
		},
	}

	got := ir.Lower(c)

	want := []ir.Op{
		{Kind: ir.OpRX, Qubits: []int{0}, Args: []float64{1.5707963267948966}},
		{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{3.141592653589793}},
		{Kind: ir.OpMEASURE},
	}

	if !reflect.DeepEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
	if got.NumQubits != 1 {
		t.Fatalf("NumQubits = %d, want 1", got.NumQubits)
	}
}

func TestLowerNumQubitsFloor(t *testing.T) {
	c := &parser.Circuit{
		NumQubits: 0,
		Instructions: []parser.Instruction{
			{Gate: "MEASURE"},
		},
	}
	got := ir.Lower(c)
	if got.NumQubits != 1 {
		t.Fatalf("NumQubits = %d, want 1 (floored)", got.NumQubits)
	}
}

func TestLowerThreeQubitGate(t *testing.T) {
	c := &parser.Circuit{
		NumQubits: 3,
		Instructions: []parser.Instruction{
			{Gate: "H", Qubits: []int{0}, Line: 1},
			{Gate: "CCX", Qubits: []int{0, 1, 2}, Line: 2},
			{Gate: "MEASURE", Qubits: []int{0, 1, 2}, Line: 3},
		},
	}

	got := ir.Lower(c)

	want := []ir.Op{
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpCCX, Qubits: []int{0, 1, 2}},
		{Kind: ir.OpMEASURE, Qubits: []int{0, 1, 2}},
	}

	if !reflect.DeepEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
}
