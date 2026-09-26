// Copyright 2026 Magnobit, Inc. All rights reserved.

package targetfmt

import (
	"strings"
	"testing"
)

const bellQASM = "OPENQASM 3;\nqubit[2] q;\nbit[2] c;\n\nh q[0];\ncx q[0], q[1];\nc = measure q;\n"

func TestBellTranslations(t *testing.T) {
	qasm, err := QuantinuumOpenQASM(bellQASM)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`include "qelib1.inc";`, "h q[0];", "cx q[0], q[1];", "measure q -> c;"} {
		if !strings.Contains(qasm, want) {
			t.Fatalf("quantinuum missing %q in %s", want, qasm)
		}
	}

	quil, err := RigettiQuil(bellQASM)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DECLARE ro BIT[2]", "H 0", "CNOT 0 1", "MEASURE 0 ro[0]", "MEASURE 1 ro[1]"} {
		if !strings.Contains(quil, want) {
			t.Fatalf("rigetti missing %q in %s", want, quil)
		}
	}

	pulser, err := PasqalPulser(bellQASM)
	if err != nil {
		t.Fatal(err)
	}
	body := string(pulser)
	for _, want := range []string{`"sequence_builder"`, `"raman_local"`, `"rydberg_local"`, `"q0"`, `"q1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("pasqal missing %q in %s", want, body)
		}
	}

	job, err := Azure("quantinuum.sim.h2-1sc", bellQASM)
	if err != nil {
		t.Fatal(err)
	}
	if job.Provider != "quantinuum" || job.InputFormat != "honeywell.openqasm.v1" {
		t.Fatalf("azure payload = %+v", job)
	}
}

func TestUnknownGateRejected(t *testing.T) {
	_, err := RigettiQuil("OPENQASM 2.0;\nqreg q[2];\nccx q[0], q[1], q[0];\n")
	if err == nil || !strings.Contains(err.Error(), "not translated") {
		t.Fatalf("got %v", err)
	}
}

func TestAnglePi(t *testing.T) {
	quil, err := RigettiQuil("OPENQASM 2.0;\nqreg q[1];\nrx(pi/2) q[0];\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(quil, "RX(1.570796326795)") {
		t.Fatalf("quil = %s", quil)
	}
}
