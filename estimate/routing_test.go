// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "testing"

func TestAssessRouting_BetterFitFewerSWAPs(t *testing.T) {
	required := [][2]int{{0, 1}, {1, 2}, {0, 2}}
	line := AssessRouting(required, [][2]int{{0, 1}, {1, 2}})
	tri := AssessRouting(required, [][2]int{{0, 1}, {1, 2}, {0, 2}})
	if line.Fit >= tri.Fit {
		t.Errorf("triangle fit %v should beat line fit %v", tri.Fit, line.Fit)
	}
	if tri.EstimatedSWAPs != 0 {
		t.Errorf("triangle SWAPs = %d, want 0", tri.EstimatedSWAPs)
	}
	if line.EstimatedSWAPs < 1 {
		t.Errorf("line SWAPs = %d, want ≥1 for the missing 0–2 edge", line.EstimatedSWAPs)
	}
}

func TestAssessRouting_UnknownCouplingHasNoFabricatedFit(t *testing.T) {
	required := [][2]int{{0, 1}}
	got := AssessRouting(required, nil)
	if got.DirectPairs != 0 || got.EstimatedSWAPs != 0 {
		t.Errorf("unknown coupling must not invent coverage or SWAPs: %+v", got)
	}
	if got.Fit != 0 {
		t.Errorf("Fit = %v, want 0 when coupling is unknown (caller treats as neutral)", got.Fit)
	}
}

func TestAllToAllEdges_CompleteGraph(t *testing.T) {
	edges := AllToAllEdges(3)
	if len(edges) != 3 {
		t.Fatalf("AllToAllEdges(3) = %v, want 3 pairs", edges)
	}
}
