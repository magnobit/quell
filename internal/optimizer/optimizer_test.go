// Copyright 2026 Magnobit, Inc. All rights reserved.

package optimizer_test

import (
	"math"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
)

func opsEqual(a, b []ir.Op) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind {
			return false
		}
		if len(a[i].Qubits) != len(b[i].Qubits) {
			return false
		}
		for j := range a[i].Qubits {
			if a[i].Qubits[j] != b[i].Qubits[j] {
				return false
			}
		}
		if len(a[i].Args) != len(b[i].Args) {
			return false
		}
		for j := range a[i].Args {
			if math.Abs(a[i].Args[j]-b[i].Args[j]) > 1e-9 {
				return false
			}
		}
	}
	return true
}

func TestOptimizeZeroAngleDrop(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0}},
			{Kind: ir.OpRX, Qubits: []int{0}, Args: []float64{2 * math.Pi}},
			{Kind: ir.OpH, Qubits: []int{0}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, notes := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
	if len(notes) == 0 {
		t.Fatal("expected optimizer notes for dropped zero-angle rotations, got none")
	}
}

func TestOptimizeSelfInverseCancellationXX(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, notes := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "qubit 0") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a cancellation note mentioning qubit 0, got %#v", notes)
	}
}

func TestOptimizeSelfInverseCancellationCNOT(t *testing.T) {
	p := &ir.Program{
		NumQubits: 2,
		Ops: []ir.Op{
			{Kind: ir.OpCNOT, Qubits: []int{0, 1}},
			{Kind: ir.OpCNOT, Qubits: []int{0, 1}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
}

func TestOptimizeSDGCancellation(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpH, Qubits: []int{0}},
			{Kind: ir.OpS, Qubits: []int{0}},
			{Kind: ir.OpSDG, Qubits: []int{0}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
}

func TestOptimizeCascadingCancellation(t *testing.T) {
	// X X X X should fully cancel (two cancelling pairs in sequence).
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
}

func TestOptimizeISWAPNeverCancels(t *testing.T) {
	p := &ir.Program{
		NumQubits: 2,
		Ops: []ir.Op{
			{Kind: ir.OpISWAP, Qubits: []int{0, 1}},
			{Kind: ir.OpISWAP, Qubits: []int{0, 1}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpISWAP, Qubits: []int{0, 1}},
		{Kind: ir.OpISWAP, Qubits: []int{0, 1}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("ISWAP ISWAP MEASURE should survive unchanged, got %#v", got.Ops)
	}
}

func TestOptimizeRotationFusion(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.5}},
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.25}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, notes := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.75}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
	if len(notes) == 0 {
		t.Fatal("expected a fusion note, got none")
	}
}

func TestOptimizeRotationFusionThenZeroAngle(t *testing.T) {
	// RZ(1.5) + RZ(-1.5) fuses to RZ(0), which the follow-up zero-angle
	// sweep must then drop entirely.
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpH, Qubits: []int{0}},
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{1.5}},
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{-1.5}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("Ops = %#v, want %#v", got.Ops, want)
	}
}

func TestOptimizeNoFusionAcrossDifferentKind(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpRX, Qubits: []int{0}, Args: []float64{0.5}},
			{Kind: ir.OpRY, Qubits: []int{0}, Args: []float64{0.5}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpRX, Qubits: []int{0}, Args: []float64{0.5}},
		{Kind: ir.OpRY, Qubits: []int{0}, Args: []float64{0.5}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("RX/RY should not fuse, got %#v", got.Ops)
	}
}

// Guard cases: nothing may cancel or fuse across a MEASURE, BARRIER, or
// RESET touching the same qubit.

func TestOptimizeNoCancelAcrossMeasure(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpMEASURE, Qubits: []int{0}},
			{Kind: ir.OpX, Qubits: []int{0}},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpX, Qubits: []int{0}},
		{Kind: ir.OpMEASURE, Qubits: []int{0}},
		{Kind: ir.OpX, Qubits: []int{0}},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("expected no cancellation across MEASURE, got %#v", got.Ops)
	}
}

func TestOptimizeNoCancelAcrossBarrier(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpH, Qubits: []int{0}},
			{Kind: ir.OpBARRIER, Qubits: []int{0}},
			{Kind: ir.OpH, Qubits: []int{0}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpBARRIER, Qubits: []int{0}},
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("expected no cancellation across BARRIER, got %#v", got.Ops)
	}
}

func TestOptimizeNoCancelAcrossReset(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpRESET, Qubits: []int{0}},
			{Kind: ir.OpX, Qubits: []int{0}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpX, Qubits: []int{0}},
		{Kind: ir.OpRESET, Qubits: []int{0}},
		{Kind: ir.OpX, Qubits: []int{0}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("expected no cancellation across RESET, got %#v", got.Ops)
	}
}

func TestOptimizeNoFusionAcrossBarrier(t *testing.T) {
	p := &ir.Program{
		NumQubits: 1,
		Ops: []ir.Op{
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.5}},
			{Kind: ir.OpBARRIER, Qubits: []int{0}},
			{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.25}},
			{Kind: ir.OpMEASURE},
		},
	}
	got, _ := optimizer.Optimize(p)

	want := []ir.Op{
		{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.5}},
		{Kind: ir.OpBARRIER, Qubits: []int{0}},
		{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0.25}},
		{Kind: ir.OpMEASURE},
	}
	if !opsEqual(got.Ops, want) {
		t.Fatalf("expected no fusion across BARRIER, got %#v", got.Ops)
	}
}
