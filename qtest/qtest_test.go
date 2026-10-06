// Copyright 2026 Magnobit, Inc. All rights reserved.

package qtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssertions(t *testing.T) {
	if err := ExpectCompiles("H 0\nMEASURE\n"); err != nil {
		t.Fatal(err)
	}
	if err := ExpectError("fn f() -> int { return }\nH 0\nMEASURE\n"); err != nil {
		t.Fatal(err)
	}
	if err := ExpectCounts("H 0\nCNOT 0 1\nMEASURE\n", "00", 400, 0.15); err != nil {
		t.Fatal(err)
	}
	src := "fn say() -> int {\n    println(\"ok\")\n    return 1\n}\nH 0\nMEASURE\n"
	if err := ExpectStdout(src, "say()", "ok\n"); err != nil {
		t.Fatal(err)
	}
	if err := ExpectProbability("H 0\nMEASURE\n", "0", 0.5, 1e-9); err != nil {
		t.Fatal(err)
	}
	if err := ExpectState("X 0\nMEASURE\n", "1", 1, 0, 1e-9); err != nil {
		t.Fatal(err)
	}
	if err := ExpectFidelity("H 0\nCNOT 0 1\nMEASURE\n", "H 0\nCNOT 0 1\nMEASURE\n", 0.999); err != nil {
		t.Fatal(err)
	}
	if err := ExpectTVD("H 0\nMEASURE\n", "H 0\nMEASURE\n", 1e-9); err != nil {
		t.Fatal(err)
	}
	if err := ExpectProviderCapability("local-statevector", "AVAILABLE"); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverSummary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "h.quell"), []byte("H 0\nMEASURE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "h.expect"), []byte("probability 0 0.5 0.0001\ncapability local-statevector AVAILABLE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := RunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Passed != 1 || sum.Failed != 0 || sum.Seed != 1 {
		t.Fatalf("%+v", sum)
	}
	filtered, err := RunDirOptions(dir, RunOptions{Filter: "absent", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Passed != 0 || filtered.Failed != 0 || filtered.Seed != 7 {
		t.Fatalf("%+v", filtered)
	}
	bad := filepath.Join(dir, "bad.expect")
	if err := os.WriteFile(filepath.Join(dir, "bad.quell"), []byte("H 0\nMEASURE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("probability 0 0.1 0.0001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := RunDirOptions(dir, RunOptions{Filter: "bad"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Failed != 1 || len(got.Failures) != 1 || got.Failures[0].Line != 1 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.JUnit(), "failure") || !strings.Contains(got.Text(), "expected") {
		t.Fatal(got.Text())
	}
}
