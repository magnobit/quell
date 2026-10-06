// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "testing"

func TestSweepCancelKeepsPartial(t *testing.T) {
	src := "PARAM theta\nRX theta 0\nMEASURE\n"
	points := []map[string]float64{{"theta": 0.1}, {"theta": 0.2}, {"theta": 0.3}}
	cancel := make(chan struct{})
	close(cancel)
	got, err := Run(src, "zip", "exp-1", points, cancel)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].Cancelled || got[0].Bound != "" || got[0].Group != "exp-1" || got[0].Provenance == "" {
		t.Fatalf("%+v", got[0])
	}
}

func TestSweepBindsOnePoint(t *testing.T) {
	src := "PARAM theta\nRX theta 0\nMEASURE\n"
	got, err := Run(src, "grid", "exp-2", []map[string]float64{{"theta": 0.5}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Err != "" || got[0].Bound == "" || got[0].Cancelled {
		t.Fatalf("%+v", got[0])
	}
}

func TestParameterShiftUsesBinder(t *testing.T) {
	src := "PARAM theta\nRX theta 0\nMEASURE\n"
	grad, err := ParameterShift(map[string]float64{"theta": 0.2}, "theta", 0.1, func(p map[string]float64) (float64, error) {
		bound, err := BindMany(src, []map[string]float64{p})
		if err != nil {
			return 0, err
		}
		if len(bound) != 1 || bound[0] == "" {
			t.Fatalf("%v", bound)
		}
		return p["theta"], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if grad < 0.99 || grad > 1.01 {
		t.Fatalf("grad %v", grad)
	}
}
