// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"strings"
	"testing"
)

func TestHashIR_DeterministicForSameSource(t *testing.T) {
	src := "H 0\nCNOT 0 1\nMEASURE"
	a, err := HashIR(src, nil)
	if err != nil {
		t.Fatalf("HashIR: %v", err)
	}
	b, err := HashIR(src, nil)
	if err != nil {
		t.Fatalf("HashIR: %v", err)
	}
	if a != b {
		t.Fatalf("same source produced different IR hashes: %s vs %s", a, b)
	}
	if !strings.HasPrefix(a, "sha256:") {
		t.Fatalf("IR hash = %q, want sha256:<hex>", a)
	}
}

func TestHashIR_DifferentCircuitsDiffer(t *testing.T) {
	h, err := HashIR("H 0\nMEASURE", nil)
	if err != nil {
		t.Fatalf("HashIR H: %v", err)
	}
	x, err := HashIR("X 0\nMEASURE", nil)
	if err != nil {
		t.Fatalf("HashIR X: %v", err)
	}
	if h == x {
		t.Fatal("H and X circuits produced the same IR hash")
	}
}

func TestSnapshotExecution_BindsParametersAndRecordsPasses(t *testing.T) {
	src := "PARAM theta\nRX theta 0\nMEASURE"
	snap, err := SnapshotExecution(src, map[string]float64{"theta": 1.5708})
	if err != nil {
		t.Fatalf("SnapshotExecution: %v", err)
	}
	if snap.IRHash == "" {
		t.Fatal("expected a non-empty IR hash")
	}
	if snap.CompilerVersion == "" || snap.OptimizerVersion == "" || snap.IRVersion == "" {
		t.Fatalf("missing version snapshot: %+v", snap)
	}
	if len(snap.OptimizerPasses) == 0 {
		t.Fatal("expected optimizer passes to be snapshotted")
	}
	found := false
	for _, p := range snap.Parameters {
		if p.Name == "theta" && p.Bound && p.Value != nil && *p.Value == 1.5708 {
			found = true
		}
	}
	if !found {
		t.Fatalf("theta binding missing: %+v", snap.Parameters)
	}

	unbound, err := SnapshotExecution(src, nil)
	if err != nil {
		t.Fatalf("unbound SnapshotExecution: %v", err)
	}
	if unbound.IRHash == snap.IRHash {
		t.Fatal("bound and unbound IR must hash differently")
	}
	if snap.CompilerVersion == CompilerVersion || !strings.Contains(snap.CompilerVersion, "@") {
		t.Fatalf("CompilerVersion %q should pin schema to a build id, not the bare label", snap.CompilerVersion)
	}
	if snap.OptimizerVersion == OptimizerVersion || !strings.Contains(snap.OptimizerVersion, "@") {
		t.Fatalf("OptimizerVersion %q should pin schema to a build id, not the bare label", snap.OptimizerVersion)
	}
}

func TestSnapshotExecution_RecordsOriginalAndOptimizedHashes(t *testing.T) {
	src := "X 0\nX 0\nMEASURE"
	snap, err := SnapshotExecution(src, nil)
	if err != nil {
		t.Fatalf("SnapshotExecution: %v", err)
	}
	if snap.OriginalIRHash == "" || snap.OptimizedIRHash == "" {
		t.Fatal("expected original and optimized IR hashes")
	}
	if snap.OriginalIRHash != snap.IRHash {
		t.Fatal("IRHash must remain the pre-optimize canonical hash")
	}
	if snap.OriginalIRHash == snap.OptimizedIRHash {
		t.Fatal("X X MEASURE should optimize to a different IR")
	}
	if !strings.HasPrefix(snap.OriginalIRHash, "sha256:") || !strings.HasPrefix(snap.OptimizedIRHash, "sha256:") {
		t.Fatalf("hashes must use the P0 sha256: format: %s %s", snap.OriginalIRHash, snap.OptimizedIRHash)
	}
}

func TestBuildIdentity_PinsSchemaToModuleVersion(t *testing.T) {
	id := BuildIdentity()
	got := id.PinnedCompiler()
	if !strings.HasPrefix(got, CompilerVersion+"@") {
		t.Fatalf("PinnedCompiler() = %q, want %s@<build-id>", got, CompilerVersion)
	}
	if id.ModuleVersion != "" && id.ModuleVersion != "unknown" && !strings.Contains(got, id.ModuleVersion) {
		t.Fatalf("PinnedCompiler() = %q, missing module version %q", got, id.ModuleVersion)
	}
}
