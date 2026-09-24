// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package provider is the authoritative QubitLabs verification record for
// each backend integration. Capability (can this device run a feature?)
// lives on scheduler_backends / telemetry. Readiness (how strongly have
// we verified the adapter path?) lives here and must never be inferred
// from "an adapter file exists" or from catalog Status live|stub|planned.
package provider

// Readiness levels are distinct facts. IMPLEMENTED is not LIVE_VERIFIED.
const (
	LevelPlanned         = "PLANNED"
	LevelImplemented     = "IMPLEMENTED"
	LevelContractTested  = "CONTRACT_TESTED"
	LevelSandboxVerified = "SANDBOX_VERIFIED"
	LevelLiveVerified    = "LIVE_VERIFIED"
	LevelExperimental    = "EXPERIMENTAL"
	LevelDegraded        = "DEGRADED"
)

// Readiness is how strongly QubitLabs has verified a provider path.
// VerifiedAt is set only for SANDBOX_VERIFIED / LIVE_VERIFIED evidence.
type Readiness struct {
	Level      string `json:"level"`
	VerifiedAt string `json:"verifiedAt,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// For returns the recorded readiness for a catalog id. Unknown ids are
// PLANNED — never LIVE_VERIFIED by default.
func For(id string) Readiness {
	if r, ok := registry[id]; ok {
		return r
	}
	return Readiness{
		Level:    LevelPlanned,
		Evidence: "no QubitLabs verification record",
		Notes:    "An unknown id is not an implemented provider.",
	}
}

// ParameterizedSupport is QubitLabs bind-then-submit support, not native
// provider symbolic-parameter support. true means Quell PARAM can be
// accepted, bound to concrete angles, and compiled to a fixed circuit
// those adapters already submit. It does not mean the provider receives
// or executes unbound/symbolic parameters. D-Wave annealing is known-false.
// Planned / unknown ids return nil (UNKNOWN). USER_CONFIGURED catalog
// knowledge, never PROVIDER_OBSERVED.
func ParameterizedSupport(id string) *bool {
	switch id {
	case "ibm", "ionq", "aws", "google", "rigetti", "azure", "nvidia", "intel", "local", "practice":
		return boolPtr(true)
	case "dwave":
		return boolPtr(false)
	default:
		return nil
	}
}

func boolPtr(v bool) *bool { return &v }

// httptest submit/poll/cancel coverage exists for these gate adapters.
// That is CONTRACT_TESTED, not sandbox or live verification.
var registry = map[string]Readiness{
	"local": {
		Level:    LevelContractTested,
		Evidence: "quell/adapter + quell/simulate unit tests",
		Notes:    "CLI/local statevector only — not hardware.",
	},
	"practice": {
		Level:    LevelImplemented,
		Evidence: "browser learning simulator",
		Notes:    "≤12 qubits; not a provider integration.",
	},
	"ibm":     gateContract("quell/internal/backends/ibm_test.go"),
	"ionq":    gateContract("quell/internal/backends/ionq_test.go"),
	"aws":     gateContract("quell/internal/backends/braket_test.go"),
	"google":  gateContract("quell/internal/backends/google_test.go"),
	"rigetti": gateContract("quell/internal/backends/rigetti_test.go"),
	"azure":   gateContract("quell/internal/backends/azure_test.go"),
	"dwave": {
		Level:    LevelImplemented,
		Evidence: "quell/internal/backends/dwave_test.go — gate-model reject + local SA fallback without Leap token",
		Notes:    "No Leap/Ocean httptest of request construction, response parsing, or job semantics. Local SA fallback is not provider contract verification.",
	},
	"nvidia": {
		Level:    LevelExperimental,
		Evidence: "quell/internal/backends/nvidia_test.go — CUDA-Q optional; local statevector fallback",
		Notes:    "Fallback success is not CUDA-Q or live-GPU verification.",
	},
	"intel": {
		Level:    LevelExperimental,
		Evidence: "quell/internal/backends/intel_test.go — SDK bridge is a placeholder; local statevector fallback",
		Notes:    "tryIntelSDK always errors today. Do not treat as native SDK success.",
	},
	"quantinuum": planned("Trapped-ion H-series — no direct adapter; often via Azure Quantum."),
	"quera":      planned("Neutral-atom — no direct adapter; often via AWS Braket."),
	"pasqal":     planned("Neutral-atom — no direct adapter."),
	"iqm":        planned("Superconducting — no direct adapter."),
	"oqc":        planned("Superconducting — no direct adapter; often via AWS Braket."),
	"xanadu":     planned("Photonic / PennyLane — no direct adapter."),
}

func gateContract(evidence string) Readiness {
	return Readiness{
		Level:    LevelContractTested,
		Evidence: evidence + " httptest submit/poll/cancel",
		Notes:    "No sandbox or live provider verification is recorded. Contract tests are not LIVE_VERIFIED.",
	}
}

func planned(notes string) Readiness {
	return Readiness{
		Level:    LevelPlanned,
		Evidence: "catalogued only — no adapter, no contract tests",
		Notes:    notes,
	}
}
