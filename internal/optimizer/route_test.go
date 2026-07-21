package optimizer

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/topology"
)

func TestRouteAdjacentNoSwap(t *testing.T) {
	p := &ir.Program{NumQubits: 3, Ops: []ir.Op{
		{Kind: ir.OpCNOT, Qubits: []int{0, 1}},
	}}
	out, notes := OptimizeWithOptions(p, Options{Coupling: topology.Linear(3)})
	swaps := 0
	for _, op := range out.Ops {
		if op.Kind == ir.OpSWAP {
			swaps++
		}
	}
	if swaps != 0 {
		t.Fatalf("expected 0 swaps, got %d notes=%v", swaps, notes)
	}
}

func TestRouteDistantInsertsSwap(t *testing.T) {
	p := &ir.Program{NumQubits: 4, Ops: []ir.Op{
		{Kind: ir.OpCNOT, Qubits: []int{0, 3}},
	}}
	out, notes := OptimizeWithOptions(p, Options{Coupling: topology.Linear(4), NoiseAware: true})
	swaps := 0
	for _, op := range out.Ops {
		if op.Kind == ir.OpSWAP {
			swaps++
		}
	}
	if swaps < 1 {
		t.Fatalf("expected ≥1 SWAP for 0–3 on linear-4, got %d ops=%v notes=%v", swaps, out.Ops, notes)
	}
	joined := strings.Join(notes, " ")
	if !strings.Contains(joined, "routing") || !strings.Contains(joined, "noise-aware") {
		t.Fatalf("expected routing/noise notes, got %v", notes)
	}
}
