// Copyright 2026 Magnobit, Inc. All rights reserved.

package engine

import "testing"

func TestLocalEngine(t *testing.T) {
	res, err := Run("auto", "H 0\nMEASURE\n", 16)
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != LocalStatevector {
		t.Fatal(res.Engine)
	}
	if len(res.Counts) == 0 {
		t.Fatal("no counts")
	}
}

func TestCUDAQDoesNotPretend(t *testing.T) {
	res, err := Run(CUDAQ, "H 0\nMEASURE\n", 8)
	if err != nil {
		for _, info := range Capabilities() {
			if info.Name == CUDAQ && info.Status == StatusAvailable {
				t.Fatal("an engine that failed the run is not AVAILABLE")
			}
		}
		return
	}
	if res.Engine != CUDAQ {
		t.Fatalf("engine %s", res.Engine)
	}
}

func TestExternalEnginesMatchBellSupport(t *testing.T) {
	src := "H 0\nCNOT 0 1\nMEASURE\n"
	for _, name := range []string{TensorNetwork, DensityMatrix} {
		t.Run(name, func(t *testing.T) {
			res, err := Run(name, src, 200)
			if err != nil {
				t.Fatal(err)
			}
			if res.Engine != name {
				t.Fatalf("engine %s", res.Engine)
			}
			for bits := range res.Counts {
				if bits != "00" && bits != "11" {
					t.Fatalf("%s produced %s", name, bits)
				}
			}
			if res.Counts["00"] == 0 || res.Counts["11"] == 0 {
				t.Fatalf("%s counts %v", name, res.Counts)
			}
		})
	}
}
