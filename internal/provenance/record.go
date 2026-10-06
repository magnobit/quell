// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package provenance is a record of one execution. It does not schedule jobs.
package provenance

// Record links source, toolchain, and an optional lock hash.
// It is not a Verify report and not user println output.
type Record struct {
	SourceHash         string
	Language           string
	Compiler           string
	Optimizer          string
	IRHash             string
	LockHash           string
	Engine             string
	Provider           string
	Backend            string
	ProviderJob        string
	ParamsHash         string
	ObservableHash     string
	CapabilitySnapshot string
	EstimatedCost      *float64
	ActualCost         *float64
	ResultHash         string
	VerifyStatus       string
	VerifyReport       string
	ExperimentID       string
	SweepGroup         string
}

// EngineLocal is the only engine this record may use for a fallback run.
const EngineLocal = "local-statevector"
