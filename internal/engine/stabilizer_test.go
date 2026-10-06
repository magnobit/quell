// Copyright 2026 Magnobit, Inc. All rights reserved.

package engine

import "testing"

func TestStabilizerBell(t *testing.T) {
	counts, err := RunStabilizer("H 0\nCNOT 0 1\nMEASURE\n", 200, 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts["01"] != 0 || counts["10"] != 0 {
		t.Fatalf("non-bell outcomes %v", counts)
	}
	if counts["00"]+counts["11"] != 200 {
		t.Fatalf("%v", counts)
	}
}

func TestStabilizerRejectsT(t *testing.T) {
	_, err := RunStabilizer("T 0\nMEASURE\n", 1, 1)
	if err == nil {
		t.Fatal("T is not Clifford")
	}
}

func TestStabilizerGHZ(t *testing.T) {
	counts, err := RunStabilizer("H 0\nCNOT 0 1\nCNOT 1 2\nMEASURE\n", 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts["000"]+counts["111"] != 100 {
		t.Fatalf("%v", counts)
	}
}

func TestStabilizerTwentyQubits(t *testing.T) {
	src := ""
	for i := 0; i < 20; i++ {
		src += "H " + itoa(i) + "\n"
	}
	src += "MEASURE\n"
	counts, err := RunStabilizer(src, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) == 0 {
		t.Fatal("empty counts")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestGroundState(t *testing.T) {
	counts, err := RunStabilizer("MEASURE\n", 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts["0"] != 20 {
		t.Fatalf("%v", counts)
	}
}
