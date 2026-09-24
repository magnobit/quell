// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"github.com/magnobit/quell/estimate"
	"github.com/magnobit/quell/provider"
)

// CatalogEntry describes one backend the `quell` CLI knows about, for
// offline `quell backends` / `backends inspect` / `backends compatible` use
// — no platform account or network access required. This intentionally
// mirrors qubitlabs-platform/internal/controlplane.Catalog()'s shape
// without importing it (they're separate Go modules, same reasoning
// cmd/quell/ai.go already documents for why callClaude is duplicated
// rather than shared: the CLI has to work standalone). Keep the two lists
// in sync by hand when a backend is added or its status changes.
type CatalogEntry struct {
	ID             string                  `json:"id"`
	Label          string                  `json:"label"`
	ExecutionModel estimate.ExecutionModel `json:"executionModel"`
	Status         string                  `json:"status"`           // catalog availability: live | stub | planned — not verification
	Qubits         int                     `json:"qubits,omitempty"` // 0 = unknown/not publicly fixed (e.g. simulators)
	Description    string                  `json:"description"`
	Readiness      provider.Readiness      `json:"readiness"`
	// SupportsParameterizedCircuits is QubitLabs bind-then-submit support
	// (nil = unknown). It is not native provider symbolic PARAM. Status
	// "live" does not imply this is true.
	SupportsParameterizedCircuits *bool `json:"supportsParameterizedCircuits,omitempty"`
}

// Catalog lists every backend the CLI can describe, in the same order as
// controlplane.Catalog() for easy eyeballing against it.
func Catalog() []CatalogEntry {
	entries := []CatalogEntry{
		{ID: "local", Label: "Local simulator", ExecutionModel: estimate.ExecSimulation, Status: "live", Description: "Quell CLI statevector / shot preview"},
		{ID: "ibm", Label: "IBM Quantum", ExecutionModel: estimate.ExecGate, Status: "live", Qubits: 127, Description: "Superconducting QPUs via Qiskit Runtime"},
		{ID: "aws", Label: "AWS Braket", ExecutionModel: estimate.ExecGate, Status: "live", Description: "Multi-vendor gate hardware via Braket"},
		{ID: "google", Label: "Google Quantum Engine", ExecutionModel: estimate.ExecGate, Status: "live", Description: "Google superconducting processors"},
		{ID: "rigetti", Label: "Rigetti QCS", ExecutionModel: estimate.ExecGate, Status: "live", Description: "Rigetti superconducting via QCS"},
		{ID: "ionq", Label: "IonQ Cloud", ExecutionModel: estimate.ExecGate, Status: "live", Qubits: 36, Description: "Trapped-ion hardware"},
		{ID: "azure", Label: "Azure Quantum", ExecutionModel: estimate.ExecGate, Status: "live", Description: "Microsoft marketplace targets (IonQ, Quantinuum, ...)"},
		{ID: "dwave", Label: "D-Wave", ExecutionModel: estimate.ExecAnnealing, Status: "live", Qubits: 5000, Description: "QUBO via Leap (Ocean) or local simulated annealing -- use `quell anneal run`, not gate-model Quell"},
		{ID: "nvidia", Label: "NVIDIA cuQuantum / CUDA-Q", ExecutionModel: estimate.ExecSimulation, Status: "live", Description: "CUDA-Q when installed; otherwise local statevector fallback"},
		{ID: "intel", Label: "Intel Quantum SDK", ExecutionModel: estimate.ExecSimulation, Status: "live", Description: "Local statevector today; Intel SDK bridge when QUELL_INTEL_SDK=1"},
		{ID: "quantinuum", Label: "Quantinuum", ExecutionModel: estimate.ExecGate, Status: "planned", Description: "Trapped-ion H-series -- often via Azure Quantum today"},
		{ID: "quera", Label: "QuEra Aquila", ExecutionModel: estimate.ExecGate, Status: "planned", Description: "Neutral-atom analog / digital -- often via AWS Braket"},
		{ID: "pasqal", Label: "Pasqal", ExecutionModel: estimate.ExecGate, Status: "planned", Description: "Neutral-atom processors"},
		{ID: "iqm", Label: "IQM Resonance", ExecutionModel: estimate.ExecGate, Status: "planned", Description: "Superconducting QPUs (Europe)"},
		{ID: "oqc", Label: "Oxford Quantum Circuits", ExecutionModel: estimate.ExecGate, Status: "planned", Description: "Superconducting -- often via AWS Braket"},
		{ID: "xanadu", Label: "Xanadu / PennyLane", ExecutionModel: estimate.ExecSimulation, Status: "planned", Description: "Photonic + differentiable simulators"},
	}
	for i := range entries {
		entries[i].Readiness = provider.For(entries[i].ID)
		entries[i].SupportsParameterizedCircuits = provider.ParameterizedSupport(entries[i].ID)
	}
	return entries
}

// Lookup returns the catalog entry with the given ID, if any.
func Lookup(id string) (CatalogEntry, bool) {
	for _, e := range Catalog() {
		if e.ID == id {
			return e, true
		}
	}
	return CatalogEntry{}, false
}
