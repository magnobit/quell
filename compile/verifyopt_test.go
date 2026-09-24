// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"testing"

	"github.com/magnobit/quell/optequiv"
)

func TestVerifyOptimization_XXCancellation(t *testing.T) {
	ev, err := VerifyOptimization("X 0\nX 0\nMEASURE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Status != optequiv.StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
	if ev.OriginalIRHash == "" || ev.OptimizedIRHash == "" || ev.OriginalIRHash == ev.OptimizedIRHash {
		t.Fatalf("hashes: %s / %s", ev.OriginalIRHash, ev.OptimizedIRHash)
	}
	if ev.OptimizerVersion == "" || len(ev.Passes) == 0 {
		t.Fatal("expected pinned optimizer version and pass list")
	}
}

func TestVerifyOptimization_ParseError(t *testing.T) {
	if _, err := VerifyOptimization("not a circuit ???", nil); err == nil {
		t.Fatal("expected parse error")
	}
}
