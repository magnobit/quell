// Copyright 2026 Magnobit, Inc. All rights reserved.

package optequiv

import "testing"

func TestVerifySource_XX(t *testing.T) {
	ev, err := VerifySource("X 0\nX 0\nMEASURE", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Status != StatusEquivalent {
		t.Fatalf("status=%s reason=%s", ev.Status, ev.Reason)
	}
}
