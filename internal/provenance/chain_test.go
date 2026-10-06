// Copyright 2026 Magnobit, Inc. All rights reserved.

package provenance

import "testing"

func TestBuildLocalChain(t *testing.T) {
	rec, err := Build("H 0\nCNOT 0 1\nMEASURE\n", 8)
	if err != nil {
		t.Fatal(err)
	}
	if rec.SourceHash == "" || rec.IRHash == "" || rec.ResultHash == "" || rec.VerifyStatus == "" || rec.VerifyReport == "" {
		t.Fatalf("%+v", rec)
	}
	if rec.EstimatedCost != nil || rec.ActualCost != nil {
		t.Fatal("unknown cost was stored")
	}
	if rec.Engine != EngineLocal || rec.Language == "" {
		t.Fatalf("%+v", rec)
	}
	linked := Link(rec, "job-9", "exp-9", "local", "local-statevector", nil, nil)
	if linked.ProviderJob != "job-9" || linked.ExperimentID != "exp-9" {
		t.Fatalf("%+v", linked)
	}
}
