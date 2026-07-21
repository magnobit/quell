// Copyright 2026 Magnobit, Inc. All rights reserved.

package simulate

import (
	"fmt"
	"math"
	"math/rand"
)

// NoiseModel is a stochastic (trajectory) noise channel applied during local
// simulation. Ideal for teaching decoherence / readout error — not a full
// density-matrix simulator.
type NoiseModel struct {
	// Depolarizing: after each gated qubit, with probability P apply X, Y, or Z
	// each with equal probability (standard single-qubit depolarizing channel).
	Depolarizing float64
	// AmplitudeDamping: T1-like relaxation toward |0⟩ (stochastic unravelling).
	AmplitudeDamping float64
	// PhaseDamping: T2-like dephasing — with probability P apply Z on the qubit.
	PhaseDamping float64
	// ReadoutError: after sampling a bitstring, flip each bit independently
	// with this probability (classical SPAM / readout error).
	ReadoutError float64
}

// Active reports whether any channel is enabled.
func (n NoiseModel) Active() bool {
	return n.GateNoise() || n.ReadoutError > 0
}

// GateNoise is true when a channel must run during gate application.
func (n NoiseModel) GateNoise() bool {
	return n.Depolarizing > 0 || n.AmplitudeDamping > 0 || n.PhaseDamping > 0
}

// Validate returns an error if parameters are out of [0,1].
func (n NoiseModel) Validate() error {
	check := func(name string, v float64) error {
		if v < 0 || v > 1 {
			return fmt.Errorf("noise: %s=%g must be in [0,1]", name, v)
		}
		return nil
	}
	if err := check("depolarizing", n.Depolarizing); err != nil {
		return err
	}
	if err := check("amplitude_damping", n.AmplitudeDamping); err != nil {
		return err
	}
	if err := check("phase_damping", n.PhaseDamping); err != nil {
		return err
	}
	return check("readout", n.ReadoutError)
}

// ApplyAfterGate applies configured gate channels to each touched qubit.
func (n NoiseModel) ApplyAfterGate(sv *StateVector, qubits []int, rng *rand.Rand) {
	if !n.GateNoise() || rng == nil {
		return
	}
	seen := map[int]bool{}
	for _, q := range qubits {
		if q < 0 || q >= sv.N || seen[q] {
			continue
		}
		seen[q] = true
		if n.Depolarizing > 0 {
			applyDepolarizing(sv, q, n.Depolarizing, rng)
		}
		if n.AmplitudeDamping > 0 {
			applyAmplitudeDamping(sv, q, n.AmplitudeDamping, rng)
		}
		if n.PhaseDamping > 0 {
			applyPhaseDamping(sv, q, n.PhaseDamping, rng)
		}
	}
}

// ApplyReadout flips bits in a single-shot bitstring with probability ReadoutError.
func (n NoiseModel) ApplyReadout(bitstring string, rng *rand.Rand) string {
	if n.ReadoutError <= 0 || rng == nil || bitstring == "" {
		return bitstring
	}
	b := []byte(bitstring)
	for i := range b {
		if b[i] != '0' && b[i] != '1' {
			continue
		}
		if rng.Float64() < n.ReadoutError {
			if b[i] == '0' {
				b[i] = '1'
			} else {
				b[i] = '0'
			}
		}
	}
	return string(b)
}

// CorruptCounts applies readout error shot-by-shot to an ideal counts map.
func (n NoiseModel) CorruptCounts(counts map[string]int, rng *rand.Rand) map[string]int {
	if n.ReadoutError <= 0 || rng == nil {
		return counts
	}
	out := make(map[string]int)
	for bit, c := range counts {
		for i := 0; i < c; i++ {
			k := n.ApplyReadout(bit, rng)
			out[k]++
		}
	}
	return out
}

func applyDepolarizing(sv *StateVector, q int, p float64, rng *rand.Rand) {
	if rng.Float64() >= p {
		return
	}
	switch rng.Intn(3) {
	case 0:
		sv.X(q)
	case 1:
		sv.Y(q)
	default:
		sv.Z(q)
	}
}

func applyPhaseDamping(sv *StateVector, q int, p float64, rng *rand.Rand) {
	// Stochastic Z: teaches T2 dephasing without a full density matrix.
	if rng.Float64() < p {
		sv.Z(q)
	}
}

// applyAmplitudeDamping uses a simple stochastic unravelling:
// with probability γ * P(|1⟩), apply the jump operator that maps the |1⟩
// subspace toward |0⟩ (RESET-like on that qubit). Otherwise apply the
// no-jump Kraus E0 ≈ diag(1, sqrt(1-γ)) as a damping of |1⟩ amplitudes.
func applyAmplitudeDamping(sv *StateVector, q int, gamma float64, rng *rand.Rand) {
	s := 1 << q
	p1 := 0.0
	for i, a := range sv.amp {
		if i&s != 0 {
			p1 += real(a)*real(a) + imag(a)*imag(a)
		}
	}
	if p1 <= 0 {
		return
	}
	if rng.Float64() < gamma*p1 {
		sv.Reset(q)
		return
	}
	scale := math.Sqrt(1 - gamma)
	norm := 0.0
	for i := range sv.amp {
		if i&s != 0 {
			sv.amp[i] *= complex(scale, 0)
		}
		a := sv.amp[i]
		norm += real(a)*real(a) + imag(a)*imag(a)
	}
	if norm > 0 {
		inv := 1 / math.Sqrt(norm)
		for i := range sv.amp {
			sv.amp[i] *= complex(inv, 0)
		}
	}
}

// ParseNoiseFlag parses CLI forms: "depolarizing=0.01", "readout=0.02", etc.
func ParseNoiseFlag(s string) (NoiseModel, error) {
	var n NoiseModel
	eq := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '=' || s[i] == ':' {
			eq = i
			break
		}
	}
	if eq <= 0 {
		return n, fmt.Errorf("noise: expected name=value, got %q", s)
	}
	name, val := s[:eq], s[eq+1:]
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return n, fmt.Errorf("noise: bad value %q", val)
	}
	switch name {
	case "depolarizing", "depolarising", "dep":
		n.Depolarizing = f
	case "amplitude_damping", "amplitude-damping", "amp", "t1":
		n.AmplitudeDamping = f
	case "phase_damping", "phase-damping", "dephasing", "t2":
		n.PhaseDamping = f
	case "readout", "readout_error", "spam":
		n.ReadoutError = f
	default:
		return n, fmt.Errorf("noise: unknown model %q (depolarizing|amplitude_damping|phase_damping|readout)", name)
	}
	return n, n.Validate()
}

// MergeNoise combines two models (later non-zero fields win when set).
func MergeNoise(a, b NoiseModel) NoiseModel {
	out := a
	if b.Depolarizing > 0 {
		out.Depolarizing = b.Depolarizing
	}
	if b.AmplitudeDamping > 0 {
		out.AmplitudeDamping = b.AmplitudeDamping
	}
	if b.PhaseDamping > 0 {
		out.PhaseDamping = b.PhaseDamping
	}
	if b.ReadoutError > 0 {
		out.ReadoutError = b.ReadoutError
	}
	return out
}
