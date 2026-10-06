// Copyright 2026 Magnobit, Inc. All rights reserved.

package optimizer_test

import (
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
)

func TestBasisTDGLowersToT(t *testing.T) {
	c, err := parser.Parse("TDG 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	out, recs := optimizer.OptimizeForTarget(ir.Lower(c), optimizer.TargetProfile{
		Native:     []string{"T", "MEASURE"},
		MaxQubits:  8,
		Calibrated: "2026-10-01T00:00:00Z",
	})
	n := 0
	for _, op := range out.Ops {
		if op.Kind == ir.OpT {
			n++
		}
		if op.Kind == ir.OpTDG {
			t.Fatal("TDG survived native lowering")
		}
	}
	if n != 7 {
		t.Fatalf("T count %d", n)
	}
	if len(recs) < 6 {
		t.Fatalf("records %d", len(recs))
	}
	if recs[0].Name != "basis_decomposition" || recs[0].Snapshot == "" || recs[0].Version != "1" {
		t.Fatalf("%+v", recs[0])
	}
	ev := optequiv.Compare(ir.Lower(c), out, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("equivalence %s %s", ev.Status, ev.Reason)
	}
}

func TestCommuteThenCancelZ(t *testing.T) {
	c, err := parser.Parse("Z 0\nCNOT 0 1\nZ 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := optimizer.OptimizeForTarget(ir.Lower(c), optimizer.TargetProfile{})
	z := 0
	for _, op := range out.Ops {
		if op.Kind == ir.OpZ {
			z++
		}
	}
	if z != 0 {
		t.Fatalf("Z count %d, ops %+v", z, out.Ops)
	}
	ev := optequiv.Compare(ir.Lower(c), out, optequiv.Options{})
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("equivalence %s %s", ev.Status, ev.Reason)
	}
}

func TestDepthPullsDisjointGate(t *testing.T) {
	c, err := parser.Parse("H 0\nX 1\n")
	if err != nil {
		t.Fatal(err)
	}
	p := ir.Lower(c)
	if optimizer.DepthLayers(p) != 1 {
		t.Fatalf("depth %d", optimizer.DepthLayers(p))
	}
	late, err := parser.Parse("H 0\nCNOT 0 2\nX 1\n")
	if err != nil {
		t.Fatal(err)
	}
	raw := ir.Lower(late)
	moved, _ := optimizer.OptimizeForTarget(raw, optimizer.TargetProfile{})
	if optimizer.DepthLayers(moved) > optimizer.DepthLayers(raw) {
		t.Fatalf("depth grew %d -> %d", optimizer.DepthLayers(raw), optimizer.DepthLayers(moved))
	}
}

func TestSnapshotChangesWithCalibration(t *testing.T) {
	a := optimizer.SnapshotHash(optimizer.TargetProfile{Native: []string{"H", "CNOT"}, Calibrated: "2026-01-01"})
	b := optimizer.SnapshotHash(optimizer.TargetProfile{Native: []string{"CNOT", "H"}, Calibrated: "2026-01-01"})
	c := optimizer.SnapshotHash(optimizer.TargetProfile{Native: []string{"H", "CNOT"}, Calibrated: "2026-02-01"})
	if a != b {
		t.Fatal("native order changed the snapshot")
	}
	if a == c {
		t.Fatal("calibration timestamp did not change the snapshot")
	}
}

func TestRecordedTargetsDifferAndStayEquivalent(t *testing.T) {
	c, err := parser.Parse("TDG 0\nH 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	raw := ir.Lower(c)
	early, recEarly := optimizer.OptimizeForTarget(raw, optimizer.TargetProfile{
		Native: []string{"T", "H", "MEASURE"}, MaxQubits: 4, Dynamic: false, Calibrated: "2026-01-01T00:00:00Z",
		ErrorMetric: map[string]float64{"t": 0.001},
	})
	late, recLate := optimizer.OptimizeForTarget(raw, optimizer.TargetProfile{
		Native: []string{"TDG", "H", "MEASURE"}, MaxQubits: 4, Dynamic: false, Calibrated: "2026-06-01T00:00:00Z",
		ErrorMetric: map[string]float64{"tdg": 0.002},
	})
	if recEarly[0].Snapshot == recLate[0].Snapshot {
		t.Fatal("recorded snapshots hashed the same")
	}
	tCount := 0
	for _, op := range early.Ops {
		if op.Kind == ir.OpT {
			tCount++
		}
	}
	if tCount != 7 {
		t.Fatalf("early target T count %d", tCount)
	}
	kept := false
	for _, op := range late.Ops {
		if op.Kind == ir.OpTDG {
			kept = true
		}
	}
	if !kept {
		t.Fatal("later target dropped TDG")
	}
	for _, prog := range []*ir.Program{early, late} {
		ev := optequiv.Compare(raw, prog, optequiv.Options{})
		if ev.Status != optequiv.StatusEquivalent {
			t.Fatalf("equivalence %s %s", ev.Status, ev.Reason)
		}
	}
}
