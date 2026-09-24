// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magnobit/quell/estimate"
)

func TestExecutionModelCompatible(t *testing.T) {
	cases := []struct {
		required, backend estimate.ExecutionModel
		want              bool
	}{
		{estimate.ExecGate, estimate.ExecGate, true},
		{estimate.ExecGate, estimate.ExecSimulation, true},
		{estimate.ExecGate, estimate.ExecAnnealing, false},
		{estimate.ExecAnnealing, estimate.ExecAnnealing, true},
		{estimate.ExecAnnealing, estimate.ExecGate, false},
		{estimate.ExecSimulation, estimate.ExecSimulation, true},
		{estimate.ExecSimulation, estimate.ExecGate, false},
	}
	for _, c := range cases {
		if got := executionModelCompatible(c.required, c.backend); got != c.want {
			t.Errorf("executionModelCompatible(%s, %s) = %v, want %v", c.required, c.backend, got, c.want)
		}
	}
}

func TestCompatibilityResults_DWaveRejectedForGateModelWorkload(t *testing.T) {
	req := estimate.WorkloadRequirements{ExecutionModel: estimate.ExecGate, LogicalQubits: 2}
	results := compatibilityResults(req)

	var dwave, ibm *backendCompatibility
	for i := range results {
		switch results[i].ID {
		case "dwave":
			dwave = &results[i]
		case "ibm":
			ibm = &results[i]
		}
	}
	if dwave == nil || ibm == nil {
		t.Fatalf("expected both dwave and ibm in the catalog results, got %+v", results)
	}
	if dwave.Compatible {
		t.Errorf("D-Wave (annealing) should never be compatible with a gate-model workload, even with plenty of qubits")
	}
	if dwave.ExecutionModel != "FAIL" {
		t.Errorf("D-Wave's execution-model check = %q, want FAIL", dwave.ExecutionModel)
	}
	if !ibm.Compatible {
		t.Errorf("IBM (gate, 127 qubits) should be compatible with a 2-qubit gate-model workload")
	}
}

func TestCompatibilityResults_UnknownQubitCountNeverFailsAlone(t *testing.T) {
	// aws/google/rigetti/etc. have no fixed qubit count in the local
	// catalog (Qubits == 0, meaning "unknown"). An unknown dimension must
	// never fail the compatibility verdict by itself.
	req := estimate.WorkloadRequirements{ExecutionModel: estimate.ExecGate, LogicalQubits: 50}
	results := compatibilityResults(req)
	for _, r := range results {
		if r.Qubits == "?" && r.ExecutionModel == "PASS" && !r.Compatible {
			t.Errorf("backend %s: unknown qubit count should not cause a FAIL, got %+v", r.ID, r)
		}
	}
}

func TestCompatibilityResults_NonParameterizedUnknownIsNotRejected(t *testing.T) {
	req := estimate.WorkloadRequirements{ExecutionModel: estimate.ExecGate, LogicalQubits: 2}
	results := compatibilityResults(req)
	for _, r := range results {
		if r.ID == "quantinuum" && !r.Compatible {
			t.Errorf("non-PARAM + UNKNOWN parameterized flag must not reject, got %+v", r)
		}
		if r.ID == "ibm" && !r.Compatible {
			t.Errorf("non-PARAM IBM must stay compatible, got %+v", r)
		}
	}
}

func TestCompatibilityResults_DynamicUnknownIsNotSupported(t *testing.T) {
	req := estimate.WorkloadRequirements{ExecutionModel: estimate.ExecGate, LogicalQubits: 2, DynamicControl: true}
	results := compatibilityResults(req)
	for _, r := range results {
		if r.Compatible {
			t.Errorf("%s: required dynamic + UNKNOWN must be incompatible, got %+v", r.ID, r)
		}
		if r.Dynamic != "?" {
			t.Errorf("%s dynamic check = %q, want ?", r.ID, r.Dynamic)
		}
	}
}

func TestCompatibilityResults_MidCircuitUnknownIsNotSupported(t *testing.T) {
	req := estimate.WorkloadRequirements{ExecutionModel: estimate.ExecGate, LogicalQubits: 2, MidCircuitMeasurement: true}
	results := compatibilityResults(req)
	for _, r := range results {
		if r.Compatible {
			t.Errorf("%s: required mid-circuit + UNKNOWN must be incompatible, got %+v", r.ID, r)
		}
		if r.MidCircuit != "?" {
			t.Errorf("%s mid-circuit check = %q, want ?", r.ID, r.MidCircuit)
		}
	}
}

func TestCompatibilityResults_ParameterizedUnknownIsNotSupported(t *testing.T) {
	req := estimate.WorkloadRequirements{ExecutionModel: estimate.ExecGate, LogicalQubits: 2, UsesParameterizedGates: true}
	results := compatibilityResults(req)
	var dwave, ibm, planned *backendCompatibility
	for i := range results {
		switch results[i].ID {
		case "dwave":
			dwave = &results[i]
		case "ibm":
			ibm = &results[i]
		case "quantinuum":
			planned = &results[i]
		}
	}
	if ibm == nil || dwave == nil || planned == nil {
		t.Fatal("expected ibm, dwave, quantinuum")
	}
	if !ibm.Compatible || ibm.Parameterized != "PASS" {
		t.Errorf("ibm = %+v, want compatible PARAM PASS", ibm)
	}
	if dwave.Compatible || dwave.Parameterized == "PASS" {
		t.Errorf("dwave = %+v, want incompatible for parameterized gate circuits", dwave)
	}
	if planned.Compatible || planned.Parameterized != "?" {
		t.Errorf("quantinuum = %+v, unknown PARAM must not count as supported", planned)
	}
}

func TestBackendsCompatibleCmd_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bell.quell")
	if err := os.WriteFile(file, []byte("H 0\nCNOT 0 1\nMEASURE\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cmd := newBackendsCompatibleCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{file})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// printCompatibilityTable writes to stdout directly (fmt.Printf), not
	// cmd.OutOrStdout — this test only exercises that the command runs
	// without error end-to-end (parses the file, derives requirements,
	// checks the catalog); the actual PASS/FAIL logic is covered by the
	// compatibilityResults unit tests above.
}

func TestBackendsInspectCmd_UnknownIDErrors(t *testing.T) {
	cmd := newBackendsInspectCmd()
	cmd.SetArgs([]string{"not-a-real-backend"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected an error for an unknown backend id")
	}
	if !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("error = %q, want it to mention 'unknown backend'", err.Error())
	}
}
