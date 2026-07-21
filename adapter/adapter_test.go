// Copyright 2026 Magnobit, Inc. All rights reserved.

package adapter_test

import (
	"sort"
	"testing"

	"github.com/magnobit/quell/adapter"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func TestRegistryHasBuiltins(t *testing.T) {
	names := adapter.Names()
	sort.Strings(names)
	want := []string{"aws", "azure", "google", "ibm", "intel", "ionq", "local", "nvidia", "rigetti"}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing adapter %q in %v", w, names)
		}
	}
}

func TestSimulatorAdapterBell(t *testing.T) {
	c, err := parser.Parse("H 0\nCNOT 0 1\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	prog := ir.Lower(c)
	res, err := adapter.Run("local", &adapter.Job{Program: prog, Shots: 500})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["00"]+res.Counts["11"] != 500 {
		t.Fatalf("counts=%v", res.Counts)
	}
	if res.Requested != "local" || res.FellBack {
		t.Fatalf("meta=%+v", res)
	}
}

func TestRegisterPlugin(t *testing.T) {
	type stub struct{}
	err := adapter.RegisterPlugin(stubAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := adapter.Lookup("stub-test-provider")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "stub-test-provider" {
		t.Fatal(a.Name())
	}
}

type stubAdapter struct{}

func (stubAdapter) Name() string { return "stub-test-provider" }
func (stubAdapter) Run(job *adapter.Job) (*adapter.Result, error) {
	return &adapter.Result{Backend: "stub", Counts: map[string]int{"0": 1}}, nil
}

func TestLoadSharedPluginWindowsMessage(t *testing.T) {
	err := adapter.LoadSharedPlugin("plugins/ibm.so")
	if err == nil {
		t.Fatal("expected error")
	}
}
