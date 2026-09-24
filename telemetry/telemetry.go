// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package telemetry fetches live, best-effort backend characteristics
// (qubit count, native gates, error rates, queue wait) from providers that
// expose them beyond just running a job — today, IBM and IonQ. It's a
// top-level (not internal/) package specifically so it can be imported from
// outside the quell module: qubitlabs-platform's scheduler consumes it to
// keep its backend registry's capability/telemetry fields current, which an
// internal/ package could never allow (see quell/internal/backends, which
// this package deliberately doesn't import from, or get imported by).
package telemetry

import (
	"context"
	"time"
)

// BackendTelemetry is a best-effort snapshot of a live backend's current
// characteristics. Every field is a pointer/zero-value-means-unset type on
// purpose: a provider that doesn't expose a value (or a request that
// partially fails) must leave that field nil/zero rather than invent a
// number. Callers (qubitlabs-platform's scheduler) are responsible for
// marking whichever fields came back nil as "unknown" rather than treating
// them as e.g. zero error rate or zero queue wait.
type BackendTelemetry struct {
	Qubits           *int
	NativeGates      []string
	SingleQubitError *float64
	TwoQubitError    *float64
	ReadoutError     *float64
	QueueWaitSeconds *int
	// CouplingMap is the backend's real physical qubit connectivity — pairs
	// of qubit indices with a native two-qubit connection, exactly as the
	// provider reports it (not normalized/deduplicated the way
	// estimate.WorkloadRequirements.RequiredConnectivity is, since it's
	// meant to be compared against that as-is). nil means the provider
	// didn't report one, not "no qubits are connected."
	CouplingMap  [][2]int
	CalibratedAt time.Time
	Source       string // "provider_api" — set even on a partial/failed fetch, so callers can tell "we tried" from "never configured"

	// AverageFidelity is the provider's raw fidelity figure when they
	// publish one (IonQ average_fidelity). It is not an error rate.
	AverageFidelity *float64
	// TwoQubitErrorDerived is true when TwoQubitError was computed by
	// QubitLabs (e.g. 1-average_fidelity) rather than reported as a gate
	// error by the provider. TwoQubitErrorDerivation names the formula.
	TwoQubitErrorDerived    bool
	TwoQubitErrorDerivation string

	QubitsCal []QubitCalibration
	GatesCal  []GateCalibration

	// Partial is true when at least one telemetry endpoint contributed
	// and at least one failed. Valid fields are still populated.
	Partial bool

	// FailKind classifies a hard fetch failure for observability
	// (auth, rate_limit, timeout, malformed, network). Empty on success.
	FailKind string
}

// QubitCalibration is one qubit's independently optional measurements.
// Missing fields stay nil — a qubit may have readout without T1, or T1
// without an assignment matrix. Never synthesize a matrix from a scalar.
type QubitCalibration struct {
	Qubit        int
	ReadoutError *float64
	T1Seconds    *float64
	T2Seconds    *float64
	// P0Given1 / P1Given0 are the provider-supplied 2x2 assignment-matrix
	// off-diagonals (IBM: prob_meas0_prep1 / prob_meas1_prep0). Both must
	// be present and valid before HasAssignmentMatrix is true.
	P0Given1            *float64
	P1Given0            *float64
	HasAssignmentMatrix bool
}

// GateCalibration is one provider-reported gate error before any
// 1Q/2Q aggregation. Gate and qubit/edge scope are preserved.
type GateCalibration struct {
	Gate   string
	Qubits []int
	Error  *float64
}

// TelemetryProvider is implemented by backends that can report live
// characteristics beyond running a job — today, IBM and IonQ. It is
// deliberately separate from quell/adapter.BackendAdapter: most adapters
// (AWS, Google, Rigetti, Azure, D-Wave, NVIDIA, Intel) don't implement it
// yet, and that's fine — a missing TelemetryProvider is exactly as valid a
// source of "unknown" as an implemented one that couldn't fetch a field.
type TelemetryProvider interface {
	FetchTelemetry(ctx context.Context) (BackendTelemetry, error)
}
