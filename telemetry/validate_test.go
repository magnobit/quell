// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import "testing"

func TestNormalizeError_PercentVsProbability(t *testing.T) {
	if v := normalizeError(2, "percent"); v == nil || *v != 0.02 {
		t.Errorf("2 percent = %v, want 0.02", v)
	}
	if v := normalizeError(0.02, ""); v == nil || *v != 0.02 {
		t.Errorf("probability 0.02 = %v", v)
	}
	if v := normalizeError(0, ""); v == nil || *v != 0 {
		t.Errorf("explicit zero must be known 0, got %v", v)
	}
	if v := normalizeError(2, ""); v != nil {
		t.Errorf("value 2 without unit must not be guessed as percent, got %v", v)
	}
	if v := normalizeError(-0.1, ""); v != nil {
		t.Errorf("negative probability must be rejected, got %v", v)
	}
}

func TestValidAssignmentPair_RequiresBothDirectionsAndInvertibleMatrix(t *testing.T) {
	if !ValidAssignmentPair(0.03, 0.01) {
		t.Fatal("valid IBM-style pair should pass")
	}
	if ValidAssignmentPair(0.03, 0.97) {
		t.Fatal("singular / unnormalized pair must be rejected")
	}
	if ValidAssignmentPair(1, 0) || ValidAssignmentPair(0, 1) {
		t.Fatal("off-diagonal of 1 is not a valid assignment probability")
	}
	if ValidAssignmentPair(0.5, 0.5) {
		t.Fatal("det=0 pair must be rejected")
	}
}

func TestNormalizeTimeToSeconds(t *testing.T) {
	if v := normalizeTimeToSeconds(100, "us"); v == nil || *v < 99.9e-6 || *v > 100.1e-6 {
		t.Errorf("100us = %v, want 1e-4 s", v)
	}
	if v := normalizeTimeToSeconds(2, "ms"); v == nil || *v != 0.002 {
		t.Errorf("2ms = %v, want 0.002", v)
	}
	if v := normalizeTimeToSeconds(1.5, "s"); v == nil || *v != 1.5 {
		t.Errorf("1.5s = %v", v)
	}
	if v := normalizeTimeToSeconds(-1, "us"); v != nil {
		t.Errorf("negative T1 must be rejected, got %v", v)
	}
	if v := normalizeTimeToSeconds(10, "parsecs"); v != nil {
		t.Errorf("unknown unit must be rejected, got %v", v)
	}
}
