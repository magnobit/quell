// Copyright 2026 Magnobit, Inc. All rights reserved.

package targetfmt

import (
	"encoding/json"
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
	for _, want := range []string{
		`"sequence_builder"`, `"rydberg_local"`, `"q0"`, `"q1"`,
		`"name":"DigitalAnalogDevice"`, `"measurement":"ground-rydberg"`, `"protocol":"min-delay"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("pasqal missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `"digital":"raman_local"`) {
		t.Fatalf("entangling pasqal sequence must not use the Raman channel: %s", body)
	}
	if strings.Contains(body, `"protocol":"const"`) {
		t.Fatalf("pasqal still uses invalid protocol const")
	}

	job, err := Azure("quantinuum.sim.h2-1sc", bellQASM)
	if err != nil {
		t.Fatal(err)
	}
	if job.Provider != "quantinuum" || job.InputFormat != "honeywell.openqasm.v1" {
		t.Fatalf("azure payload = %+v", job)
	}
}

func TestPasqalPulser_Schema(t *testing.T) {
	body, err := PasqalPulser(bellQASM)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	seq, _ := env["sequence_builder"].(map[string]any)
	if seq == nil {
		t.Fatalf("envelope = %s", body)
	}
	for _, key := range []string{"version", "name", "register", "channels", "variables", "operations", "measurement", "device"} {
		if _, ok := seq[key]; !ok {
			t.Fatalf("missing %s in %s", key, body)
		}
	}
	device, _ := seq["device"].(map[string]any)
	if device["name"] != "DigitalAnalogDevice" || seq["measurement"] != "ground-rydberg" {
		t.Fatalf("device=%v measurement=%v", seq["device"], seq["measurement"])
	}
	channels, _ := seq["channels"].(map[string]any)
	if _, ok := channels["digital"]; ok || channels["rydberg"] != "rydberg_local" {
		t.Fatalf("entangling channels = %v", channels)
	}
	ops, _ := seq["operations"].([]any)
	if len(ops) == 0 {
		t.Fatal("no operations")
	}
	for _, raw := range ops {
		op, _ := raw.(map[string]any)
		if op["op"] == "pulse" && op["protocol"] != "min-delay" {
			t.Fatalf("pulse protocol = %v", op["protocol"])
		}
		if op["op"] == "target" {
			switch op["target"].(type) {
			case float64, int:
			default:
				t.Fatalf("target must be an index, got %T %v", op["target"], op["target"])
			}
		}
	}
}

func TestPasqalPulser_SingleQubitOmitsRydberg(t *testing.T) {
	body, err := PasqalPulser("OPENQASM 3;\nqubit[1] q;\nbit[1] c;\nh q[0];\nc = measure q;\n")
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	seq, _ := env["sequence_builder"].(map[string]any)
	channels, _ := seq["channels"].(map[string]any)
	if _, ok := channels["rydberg"]; ok || len(channels) != 1 || channels["digital"] != "raman_local" {
		t.Fatalf("channels = %v", channels)
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
