// Copyright 2026 Magnobit, Inc. All rights reserved.

package optimizer

import (
	"testing"

	"github.com/magnobit/quell/internal/ir"
)

func TestProfileMatchesOptimize(t *testing.T) {
	p := &ir.Program{NumQubits: 1, Ops: []ir.Op{
		{Kind: ir.OpRZ, Qubits: []int{0}, Args: []float64{0}},
		{Kind: ir.OpH, Qubits: []int{0}},
		{Kind: ir.OpH, Qubits: []int{0}},
	}}
	opt, _ := Optimize(p)
	prof, stats := Profile(p)
	if string(ir.CanonicalBytes(opt)) != string(ir.CanonicalBytes(prof)) {
		t.Fatal("profile changed the optimized program")
	}
	if len(stats) != 4 || stats[0].Version == "" {
		t.Fatalf("%+v", stats)
	}
}
