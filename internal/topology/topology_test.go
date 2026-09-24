// Copyright 2026 Magnobit, Inc. All rights reserved.

package topology

import "testing"

func TestFromEdges_ConvertsProviderCouplingMap(t *testing.T) {
	edges := [][2]int{{0, 1}, {1, 2}, {2, 0}}
	cm := FromEdges("ibm", edges)
	if cm == nil {
		t.Fatal("FromEdges returned nil for a real coupling map")
	}
	if cm.Name != "ibm" {
		t.Errorf("Name = %q, want ibm (live backend name, not a preset)", cm.Name)
	}
	if !cm.Connected(0, 1) || !cm.Connected(1, 2) || !cm.Connected(0, 2) {
		t.Errorf("expected all three triangle edges, adj=%v", cm.adj)
	}
}

func TestFromEdges_UnknownStaysUnknown(t *testing.T) {
	if FromEdges("ibm", nil) != nil {
		t.Error("nil edges must stay unknown, not a fabricated complete graph")
	}
	if FromEdges("ibm", [][2]int{}) != nil {
		t.Error("empty edges must stay unknown")
	}
}

func TestPreset_IsNotLiveProviderName(t *testing.T) {
	p, err := Preset("fully-connected-5")
	if err != nil || p == nil {
		t.Fatalf("Preset(fully-connected-5): %v %#v", err, p)
	}
	if p.Name != "fully-connected-5" {
		t.Errorf("preset Name = %q", p.Name)
	}
	live := AllToAll("ionq-all-to-all", 5)
	if live.Name == p.Name {
		t.Fatal("architectural all-to-all must not reuse the teaching preset name")
	}
	if _, err := Preset("ionq-all-to-all"); err == nil {
		t.Fatal("ionq-all-to-all must not be a static Preset")
	}
}

func TestAllToAll_EveryPairConnected(t *testing.T) {
	cm := AllToAll("ionq-all-to-all", 4)
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			if !cm.Connected(i, j) {
				t.Errorf("expected %d–%d connected on all-to-all", i, j)
			}
		}
	}
}
