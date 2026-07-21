// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "math"

// CostModelVersion labels the educational pricing table used for estimates.
// These are not invoices — rates are approximate public list figures for UX.
const CostModelVersion = "2026.1-educational"

// GateBackends are the four providers Phase 1 compares side-by-side.
var GateBackends = []string{"ibm", "ionq", "google", "rigetti"}

// ProviderModel holds rough cost / queue / noise parameters for one backend.
type ProviderModel struct {
	Backend           string
	TaskFeeUSD        float64 // fixed per-job fee
	PerShotUSD        float64
	PerTwoQubitUSD    float64 // IonQ-style gate pricing when used
	BaseQueueSec      float64
	RuntimePerShotSec float64
	RuntimeBaseSec    float64
	// Teaching noise for fidelity proxy (local sim).
	Depolarizing float64
	Readout      float64
	NoiseNote    string
}

var models = map[string]ProviderModel{
	"ibm": {
		Backend: "ibm", TaskFeeUSD: 0, PerShotUSD: 0.0, PerTwoQubitUSD: 0,
		// IBM often billed by runtime on premium; free-tier estimate uses a
		// small effective per-shot proxy so cost ranking still works.
		BaseQueueSec: 90, RuntimePerShotSec: 0.0008, RuntimeBaseSec: 2,
		Depolarizing: 0.008, Readout: 0.02,
		NoiseNote: "Teaching noise ≈ superconducting gate+readout error",
	},
	"ionq": {
		Backend: "ionq", TaskFeeUSD: 0.30, PerShotUSD: 0.01, PerTwoQubitUSD: 0,
		BaseQueueSec: 120, RuntimePerShotSec: 0.002, RuntimeBaseSec: 3,
		Depolarizing: 0.005, Readout: 0.01,
		NoiseNote: "Teaching noise ≈ trapped-ion (lower 2Q error, slower)",
	},
	"google": {
		Backend: "google", TaskFeeUSD: 0.25, PerShotUSD: 0.004, PerTwoQubitUSD: 0,
		BaseQueueSec: 75, RuntimePerShotSec: 0.001, RuntimeBaseSec: 2.5,
		Depolarizing: 0.01, Readout: 0.015,
		NoiseNote: "Teaching noise ≈ superconducting Sycamore-class proxy",
	},
	"rigetti": {
		Backend: "rigetti", TaskFeeUSD: 0.30, PerShotUSD: 0.0035, PerTwoQubitUSD: 0,
		BaseQueueSec: 45, RuntimePerShotSec: 0.0012, RuntimeBaseSec: 2,
		Depolarizing: 0.012, Readout: 0.025,
		NoiseNote: "Teaching noise ≈ superconducting Aspen-class proxy",
	},
}

// ModelFor returns the cost/noise model for a backend id, or a zero model.
func ModelFor(backend string) (ProviderModel, bool) {
	m, ok := models[backend]
	return m, ok
}

// EstimateCostUSD returns an approximate USD cost for shots and circuit stats.
func (m ProviderModel) EstimateCostUSD(shots int, stats CircuitStats) float64 {
	if shots < 1 {
		shots = 1
	}
	c := m.TaskFeeUSD + float64(shots)*m.PerShotUSD + float64(stats.TwoQubitGates)*m.PerTwoQubitUSD
	// IBM free-tier style: derive a small runtime-based estimate so ranking works.
	if m.Backend == "ibm" && c < 1e-9 {
		rt := m.EstimateRuntimeSec(shots)
		c = rt * 0.02 // ~$0.02/s educational proxy (not IBM list price)
	}
	return math.Round(c*10000) / 10000
}

// EstimateRuntimeSec approximates wall-clock execution (not including queue).
func (m ProviderModel) EstimateRuntimeSec(shots int) float64 {
	if shots < 1 {
		shots = 1
	}
	rt := m.RuntimeBaseSec + float64(shots)*m.RuntimePerShotSec
	return math.Round(rt*100) / 100
}

// EstimateQueueSec returns a rough expected queue wait.
func (m ProviderModel) EstimateQueueSec() float64 {
	return m.BaseQueueSec
}
