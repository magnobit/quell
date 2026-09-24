// Copyright 2026 Magnobit, Inc. All rights reserved.

package provider

import "testing"

func TestFor_DoesNotTreatImplementedAsLiveVerified(t *testing.T) {
	for _, id := range []string{"ibm", "ionq", "aws", "google", "rigetti", "azure", "dwave", "nvidia", "intel", "local"} {
		r := For(id)
		if r.Level == LevelLiveVerified || r.Level == LevelSandboxVerified {
			t.Errorf("%s readiness = %s — httptest/fallback is not sandbox or live verification", id, r.Level)
		}
		if r.Level == "" {
			t.Errorf("%s readiness level is empty", id)
		}
		if r.Evidence == "" {
			t.Errorf("%s readiness has no evidence", id)
		}
	}
}

func TestFor_PlannedProvidersArePlanned(t *testing.T) {
	for _, id := range []string{"quantinuum", "quera", "pasqal", "iqm", "oqc", "xanadu", "unknown-vendor"} {
		if got := For(id).Level; got != LevelPlanned {
			t.Errorf("%s readiness = %s, want PLANNED", id, got)
		}
	}
}

func TestFor_FallbackProvidersAreNotEquivalentToGateContracts(t *testing.T) {
	if For("nvidia").Level != LevelExperimental {
		t.Errorf("nvidia = %s, want EXPERIMENTAL", For("nvidia").Level)
	}
	if For("intel").Level != LevelExperimental {
		t.Errorf("intel = %s, want EXPERIMENTAL", For("intel").Level)
	}
	if For("ibm").Level != LevelContractTested {
		t.Errorf("ibm = %s, want CONTRACT_TESTED", For("ibm").Level)
	}
	if For("dwave").Level != LevelImplemented {
		t.Errorf("dwave = %s, want IMPLEMENTED (local SA fallback is not a Leap contract)", For("dwave").Level)
	}
}

func TestParameterizedSupport_UnknownIsNil(t *testing.T) {
	if ParameterizedSupport("quantinuum") != nil {
		t.Error("planned provider must leave parameterized support UNKNOWN")
	}
	if ParameterizedSupport("dwave") == nil || *ParameterizedSupport("dwave") {
		t.Error("D-Wave must be known-false for parameterized gate circuits")
	}
	if ParameterizedSupport("ibm") == nil || !*ParameterizedSupport("ibm") {
		t.Error("IBM bind-then-submit path supports parameterized Quell")
	}
}
