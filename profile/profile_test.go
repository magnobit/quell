// Copyright 2026 Magnobit, Inc. All rights reserved.

package profile

import (
	"strings"
	"testing"
)

func TestMeasureSeparatesLocal(t *testing.T) {
	rep := Measure("H 0\nCNOT 0 1\nMEASURE\n", 4)
	local := 0
	remote := 0
	for _, s := range rep.Stages {
		if s.Local {
			local++
			if s.Skipped {
				t.Fatalf("%s skipped", s.Name)
			}
		} else {
			remote++
			if !s.Skipped || s.Duration != 0 {
				t.Fatalf("remote %s %+v", s.Name, s)
			}
		}
	}
	if local != 7 || remote != 4 {
		t.Fatalf("local %d remote %d", local, remote)
	}
	if rep.Resources.Qubits != 2 || rep.Resources.TwoQubit < 1 || rep.Resources.SimMemory != 64 {
		t.Fatalf("%+v", rep.Resources)
	}
	rep.FillRemote("provider", 3)
	for _, s := range rep.Stages {
		if s.Name == "provider" && (s.Local || s.Skipped || s.Duration != 3) {
			t.Fatalf("%+v", s)
		}
		if s.Name == "parse" && !s.Local {
			t.Fatal("parse became remote")
		}
		text := rep.Text()
		if !strings.Contains(text, "Parse") || !strings.Contains(text, "Queue") || !strings.Contains(text, "not run") || !strings.Contains(text, "QPU runtime") {
			t.Fatalf("%s", text)
		}
	}
}
