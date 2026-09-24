// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"reflect"
	"testing"
)

func TestDeriveRequirements_BellPairBaseline(t *testing.T) {
	req, err := DeriveRequirements("H 0\nCNOT 0 1\nMEASURE\n", 1000)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if req.ExecutionModel != ExecGate {
		t.Errorf("ExecutionModel = %q, want %q", req.ExecutionModel, ExecGate)
	}
	if req.LogicalQubits != 2 {
		t.Errorf("LogicalQubits = %d, want 2", req.LogicalQubits)
	}
	if req.MidCircuitMeasurement {
		t.Errorf("a single terminal MEASURE should not count as mid-circuit")
	}
	if req.DynamicControl {
		t.Errorf("a straight-line circuit has no dynamic control")
	}
	if req.Shots != 1000 {
		t.Errorf("Shots = %d, want 1000 (passed through verbatim)", req.Shots)
	}
	wantOps := []string{"CNOT", "H", "MEASURE"}
	if !reflect.DeepEqual(req.RequiredOps, wantOps) {
		t.Errorf("RequiredOps = %v, want %v", req.RequiredOps, wantOps)
	}
}

func TestDeriveRequirements_DynamicControlDetectsIF(t *testing.T) {
	req, err := DeriveRequirements("H 0\nMEASURE 0\nIF c[0]==1 X 1\nMEASURE\n", 500)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if !req.DynamicControl {
		t.Errorf("an IF-conditioned gate should be detected as dynamic control")
	}
	found := false
	for _, op := range req.RequiredOps {
		if op == "IF" {
			found = true
		}
	}
	if !found {
		t.Errorf("RequiredOps = %v, want it to include IF", req.RequiredOps)
	}
}

func TestDeriveRequirements_MidCircuitMeasurementDetectsReuseAfterMeasure(t *testing.T) {
	// Qubit 0 is measured, then a gate touches qubit 0 again before the
	// final MEASURE — that's the defining trait of a mid-circuit
	// measurement, as opposed to measuring everything once at the end.
	req, err := DeriveRequirements("H 0\nMEASURE 0\nX 0\nMEASURE\n", 500)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if !req.MidCircuitMeasurement {
		t.Errorf("expected mid-circuit measurement to be detected when a qubit is reused after MEASURE")
	}
}

func TestDeriveRequirements_TerminalMeasureIsNotMidCircuit(t *testing.T) {
	req, err := DeriveRequirements("H 0\nH 1\nH 2\nMEASURE\n", 500)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if req.MidCircuitMeasurement {
		t.Errorf("a single measure-everything-at-the-end circuit should not be flagged as mid-circuit")
	}
}

func TestDeriveRequirements_InvalidSourceReturnsError(t *testing.T) {
	if _, err := DeriveRequirements("not valid quell {{{", 100); err == nil {
		t.Errorf("expected a parse error for invalid source")
	}
}

func TestDeriveRequirements_UsesParameterizedGates(t *testing.T) {
	req, err := DeriveRequirements("PARAM theta : angle\nRX theta 0\nMEASURE\n", 500)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if !req.UsesParameterizedGates {
		t.Errorf("a circuit using a named PARAM should report UsesParameterizedGates")
	}
}

func TestDeriveRequirements_FixedAngleIsNotParameterized(t *testing.T) {
	req, err := DeriveRequirements("RX 1.5708 0\nMEASURE\n", 500)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if req.UsesParameterizedGates {
		t.Errorf("a literal float angle should not count as a parameterized gate")
	}
}

func TestDeriveRequirements_RequiredConnectivity(t *testing.T) {
	// Bell pair on 0,1 plus a CNOT on 1,2 — expect exactly the two
	// normalized (low,high) pairs, deduplicated and sorted.
	req, err := DeriveRequirements("H 0\nCNOT 0 1\nCNOT 2 1\nMEASURE\n", 100)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	want := [][2]int{{0, 1}, {1, 2}}
	if !reflect.DeepEqual(req.RequiredConnectivity, want) {
		t.Errorf("RequiredConnectivity = %v, want %v", req.RequiredConnectivity, want)
	}
}

func TestDeriveRequirements_ThreeQubitGateNeedsAllPairs(t *testing.T) {
	req, err := DeriveRequirements("H 0\nH 1\nCCX 0 1 2\nMEASURE\n", 100)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	want := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	if !reflect.DeepEqual(req.RequiredConnectivity, want) {
		t.Errorf("RequiredConnectivity = %v, want all three pairs among CCX's qubits: %v", req.RequiredConnectivity, want)
	}
}

func TestDeriveRequirements_NoMultiQubitOpsHasNilConnectivity(t *testing.T) {
	req, err := DeriveRequirements("H 0\nMEASURE\n", 100)
	if err != nil {
		t.Fatalf("DeriveRequirements: %v", err)
	}
	if req.RequiredConnectivity != nil {
		t.Errorf("RequiredConnectivity = %v, want nil for a single-qubit-only circuit", req.RequiredConnectivity)
	}
}
