package estimate

import "testing"

func TestOptimizeSourceCancelsXX(t *testing.T) {
	src := "X 0\nX 0\nH 0\nMEASURE\n"
	r, err := OptimizeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if r.Optimizer.AfterGates >= r.Optimizer.BeforeGates {
		t.Fatalf("expected reduction: %+v", r.Optimizer)
	}
	if r.Optimized == "" {
		t.Fatal("empty optimized")
	}
}
