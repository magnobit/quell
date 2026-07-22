// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package simulate is Quell's local state-vector quantum circuit simulator —
// the Go-native equivalent of the JS simulator QubitLabs' Playground runs in
// the browser (qubitlabs-platform/src/simulator), so `quell run` (no backend
// configured, or --backend local) and `quell simulate` can execute a circuit
// without any cloud credentials or network access at all.
//
// Like the Playground's simulator, the default path models a terminal
// measurement (gate application stops at the first MEASURE). When the
// circuit contains IF conditionals or active noise models, RunProgram
// switches to a per-shot trajectory path.
package simulate

import (
	"fmt"
	"math"
	"math/cmplx"
	"math/rand"
	"sort"
	"strings"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// maxQubits caps the simulator at a 2^24-entry state vector (16M complex128
// = 256MB just for amplitudes) — a practical memory limit, not a
// mathematical one. A real circuit needing more qubits than this belongs on
// real hardware or a dedicated HPC simulator, not a CLI's built-in one.
const maxQubits = 24

// StateVector is a dense state-vector simulator: the full 2^N complex
// amplitude vector, with the same single/multi-qubit gate matrix
// conventions used elsewhere in Quell (see internal/compiler's OpenQASM/
// Qiskit code generation — the U-gate matrix and rotation sign conventions
// here match those exactly, so results are consistent with what real
// hardware compiled from the same circuit would implement).
type StateVector struct {
	N   int
	dim int
	amp []complex128
}

// New returns a StateVector initialized to |0...0>. n is clamped to at least 1.
func New(n int) *StateVector {
	if n < 1 {
		n = 1
	}
	dim := 1 << n
	sv := &StateVector{N: n, dim: dim, amp: make([]complex128, dim)}
	sv.amp[0] = 1
	return sv
}

// apply1 applies the 2x2 unitary [[a,b],[c,d]] to qubit q across every
// amplitude pair that differ only in qubit q's bit.
func (sv *StateVector) apply1(q int, a, b, c, d complex128) {
	s := 1 << q
	for i := 0; i < sv.dim; i++ {
		if i&s != 0 {
			continue
		}
		j := i | s
		ai, aj := sv.amp[i], sv.amp[j]
		sv.amp[i] = a*ai + b*aj
		sv.amp[j] = c*ai + d*aj
	}
}

// applyControlled1 is apply1 restricted to amplitude pairs where the control
// qubit's bit is set — used for CRX/CRY/CRZ.
func (sv *StateVector) applyControlled1(ctrl, q int, a, b, c, d complex128) {
	cm := 1 << ctrl
	s := 1 << q
	for i := 0; i < sv.dim; i++ {
		if i&cm == 0 || i&s != 0 {
			continue
		}
		j := i | s
		ai, aj := sv.amp[i], sv.amp[j]
		sv.amp[i] = a*ai + b*aj
		sv.amp[j] = c*ai + d*aj
	}
}

func (sv *StateVector) H(q int) {
	v := complex(math.Sqrt2/2, 0)
	sv.apply1(q, v, v, v, -v)
}
func (sv *StateVector) X(q int)  { sv.apply1(q, 0, 1, 1, 0) }
func (sv *StateVector) Y(q int)  { sv.apply1(q, 0, complex(0, -1), complex(0, 1), 0) }
func (sv *StateVector) Z(q int)  { sv.apply1(q, 1, 0, 0, -1) }
func (sv *StateVector) S(q int)  { sv.apply1(q, 1, 0, 0, complex(0, 1)) }
func (sv *StateVector) SDG(q int) { sv.apply1(q, 1, 0, 0, complex(0, -1)) }
func (sv *StateVector) T(q int) {
	sv.apply1(q, 1, 0, 0, cmplx.Exp(complex(0, math.Pi/4)))
}
func (sv *StateVector) TDG(q int) {
	sv.apply1(q, 1, 0, 0, cmplx.Exp(complex(0, -math.Pi/4)))
}
func (sv *StateVector) SX(q int) {
	a, b := complex(0.5, 0.5), complex(0.5, -0.5)
	sv.apply1(q, a, b, b, a)
}
func (sv *StateVector) RX(q int, theta float64) {
	c, s := complex(math.Cos(theta/2), 0), complex(0, -math.Sin(theta/2))
	sv.apply1(q, c, s, s, c)
}
func (sv *StateVector) RY(q int, theta float64) {
	c, s := complex(math.Cos(theta/2), 0), complex(math.Sin(theta/2), 0)
	sv.apply1(q, c, -s, s, c)
}
func (sv *StateVector) RZ(q int, theta float64) {
	sv.apply1(q, cmplx.Exp(complex(0, -theta/2)), 0, 0, cmplx.Exp(complex(0, theta/2)))
}
func (sv *StateVector) P(q int, phi float64) {
	sv.apply1(q, 1, 0, 0, cmplx.Exp(complex(0, phi)))
}

// U applies the standard U(theta,phi,lambda) single-qubit gate:
// [[cos(θ/2), -e^{iλ}sin(θ/2)], [e^{iφ}sin(θ/2), e^{i(φ+λ)}cos(θ/2)]] — the
// same convention internal/compiler.go's toOpenQASM/toQiskit target.
func (sv *StateVector) U(q int, theta, phi, lambda float64) {
	ct, st := complex(math.Cos(theta/2), 0), complex(math.Sin(theta/2), 0)
	a := ct
	b := -cmplx.Exp(complex(0, lambda)) * st
	c := cmplx.Exp(complex(0, phi)) * st
	d := cmplx.Exp(complex(0, phi+lambda)) * ct
	sv.apply1(q, a, b, c, d)
}

func (sv *StateVector) CRX(ctrl, q int, theta float64) {
	c, s := complex(math.Cos(theta/2), 0), complex(0, -math.Sin(theta/2))
	sv.applyControlled1(ctrl, q, c, s, s, c)
}
func (sv *StateVector) CRY(ctrl, q int, theta float64) {
	c, s := complex(math.Cos(theta/2), 0), complex(math.Sin(theta/2), 0)
	sv.applyControlled1(ctrl, q, c, -s, s, c)
}
func (sv *StateVector) CRZ(ctrl, q int, theta float64) {
	sv.applyControlled1(ctrl, q, cmplx.Exp(complex(0, -theta/2)), 0, 0, cmplx.Exp(complex(0, theta/2)))
}

func (sv *StateVector) swap(i, j int) { sv.amp[i], sv.amp[j] = sv.amp[j], sv.amp[i] }

func (sv *StateVector) CNOT(ctrl, tgt int) {
	cm, tm := 1<<ctrl, 1<<tgt
	for i := 0; i < sv.dim; i++ {
		if i&cm == 0 {
			continue
		}
		if j := i ^ tm; j > i {
			sv.swap(i, j)
		}
	}
}

func (sv *StateVector) CZ(ctrl, tgt int) {
	cm, tm := 1<<ctrl, 1<<tgt
	for i := 0; i < sv.dim; i++ {
		if i&cm != 0 && i&tm != 0 {
			sv.amp[i] = -sv.amp[i]
		}
	}
}

func (sv *StateVector) SWAP(a, b int) {
	am, bm := 1<<a, 1<<b
	for i := 0; i < sv.dim; i++ {
		if i&am != 0 && i&bm == 0 {
			sv.swap(i, i^am^bm)
		}
	}
}

// ISWAP swaps |01> and |10> with an extra i phase: |01>→i|10>, |10>→i|01>.
func (sv *StateVector) ISWAP(a, b int) {
	am, bm := 1<<a, 1<<b
	for i := 0; i < sv.dim; i++ {
		if i&am != 0 && i&bm == 0 {
			j := i ^ am ^ bm
			ai, aj := sv.amp[i], sv.amp[j]
			sv.amp[i] = complex(0, 1) * aj
			sv.amp[j] = complex(0, 1) * ai
		}
	}
}

func (sv *StateVector) CCX(c1, c2, tgt int) {
	m1, m2, tm := 1<<c1, 1<<c2, 1<<tgt
	for i := 0; i < sv.dim; i++ {
		if i&m1 == 0 || i&m2 == 0 {
			continue
		}
		if j := i ^ tm; j > i {
			sv.swap(i, j)
		}
	}
}

func (sv *StateVector) CSWAP(ctrl, a, b int) {
	cm, am, bm := 1<<ctrl, 1<<a, 1<<b
	for i := 0; i < sv.dim; i++ {
		if i&cm == 0 {
			continue
		}
		if i&am != 0 && i&bm == 0 {
			sv.swap(i, i^am^bm)
		}
	}
}

// Reset projects qubit q to |0>: amplitude mass in the |1> subspace is moved
// onto the matching |0> index and zeroed out. Exact for a qubit unentangled
// with the rest of the register; for an entangled qubit this is a coherent
// projection, not true measurement-and-decoherence (which would require a
// density matrix) — a documented, deliberate simplification, same one the
// Playground's JS simulator makes by not supporting RESET at all.
func (sv *StateVector) Reset(q int) {
	s := 1 << q
	for i := 0; i < sv.dim; i++ {
		if i&s == 0 {
			continue
		}
		j := i &^ s
		sv.amp[j] += sv.amp[i]
		sv.amp[i] = 0
	}
}

// Probs returns the measurement probability of every computational basis
// state, len 2^N.
func (sv *StateVector) Probs() []float64 {
	p := make([]float64, sv.dim)
	for i, a := range sv.amp {
		p[i] = real(a)*real(a) + imag(a)*imag(a)
	}
	return p
}

// MeasureBit collapses qubit q onto a sampled 0/1 outcome and returns that bit.
// Used for mid-circuit MEASURE when IF conditionals follow.
func (sv *StateVector) MeasureBit(q int, rng *rand.Rand) int {
	s := 1 << q
	p1 := 0.0
	for i, a := range sv.amp {
		if i&s != 0 {
			p1 += real(a)*real(a) + imag(a)*imag(a)
		}
	}
	bit := 0
	if rng.Float64() < p1 {
		bit = 1
	}
	// Project and renormalize
	norm := 0.0
	for i := range sv.amp {
		if ((i>>q)&1) != bit {
			sv.amp[i] = 0
		} else {
			norm += real(sv.amp[i])*real(sv.amp[i]) + imag(sv.amp[i])*imag(sv.amp[i])
		}
	}
	if norm > 0 {
		scale := 1 / math.Sqrt(norm)
		for i := range sv.amp {
			sv.amp[i] *= complex(scale, 0)
		}
	}
	return bit
}

// Sample draws `shots` measurement outcomes from the current state,
// returning bit-string → count (e.g. "00" → 512), MSB-first with qubit 0 as
// the rightmost character — the same convention internal/backends uses for
// real hardware results (see backends.hexCountsToStr).
func (sv *StateVector) Sample(shots int, rng *rand.Rand) map[string]int {
	p := sv.Probs()
	counts := make(map[string]int)
	for s := 0; s < shots; s++ {
		r := rng.Float64()
		acc := 0.0
		for i, pi := range p {
			acc += pi
			if r < acc {
				counts[fmt.Sprintf("%0*b", sv.N, i)]++
				break
			}
		}
	}
	return counts
}

// Result is the outcome of simulating a circuit.
type Result struct {
	NumQubits int
	Shots     int
	Counts    map[string]int // bit-string → count, over Shots samples
	Probs     []float64      // probability of each computational basis state, len 2^NumQubits
}

// Options configures local simulation (shots, optional noise override).
type Options struct {
	Shots int
	// Noise overrides/merges with Program.Noise* when Active.
	Noise NoiseModel
	// Seed, when non-zero, seeds the RNG (useful for tests).
	Seed int64
}

// Run parses and simulates Quell source, sampling shots measurement
// outcomes from the final state. shots <= 0 defaults to 1000.
func Run(src string, shots int) (*Result, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	return RunProgramOpts(ir.Lower(c), Options{Shots: shots})
}

// RunFile parses and simulates the .quell file at path — resolving any
// "import" lines relative to its directory, or against an installed
// package (see parser.ParseFile) — the file-based counterpart to Run, for
// callers (like the CLI) that need import support rather than working from
// an in-memory source string.
func RunFile(path string, shots int) (*Result, error) {
	c, err := parser.ParseFile(path)
	if err != nil {
		return nil, err
	}
	return RunProgramOpts(ir.Lower(c), Options{Shots: shots})
}

// RunProgram simulates an already-lowered IR program — the entry point for
// callers (like the CLI's `run`/`simulate` commands) that already have a
// parsed *parser.Circuit and want to reuse the same ir.Lower step the
// compiler uses, rather than re-parsing from source.
func RunProgram(p *ir.Program, shots int) (*Result, error) {
	return RunProgramOpts(p, Options{Shots: shots})
}

// RunProgramOpts is RunProgram with explicit Options (noise, seed).
func RunProgramOpts(p *ir.Program, opt Options) (*Result, error) {
	shots := opt.Shots
	if shots <= 0 {
		shots = 1000
	}
	if ir.NeedsBind(p) {
		return nil, fmt.Errorf("simulate: circuit has unbound parameters %v — bind with --param name=value first", ir.UnboundParams(p))
	}
	n := p.NumQubits
	if n < 1 {
		n = 1
	}
	if n > maxQubits {
		return nil, fmt.Errorf("simulate: %d qubits exceeds the local simulator's limit of %d (a 2^%d-entry state vector would exhaust memory) — use a real backend instead", n, maxQubits, n)
	}

	noise := NoiseModel{
		Depolarizing:     p.NoiseDepolarizing,
		AmplitudeDamping: p.NoiseAmplitudeDamping,
		PhaseDamping:     p.NoisePhaseDamping,
		ReadoutError:     p.NoiseReadout,
	}
	noise = MergeNoise(noise, opt.Noise)
	if err := noise.Validate(); err != nil {
		return nil, err
	}

	hasIF := false
	for _, op := range p.Ops {
		switch op.Kind {
		case ir.OpIF, ir.OpWHILE, ir.OpSWITCH, ir.OpASSERT:
			hasIF = true
		case ir.OpMEASURE:
			if len(op.MeasTargets) > 0 {
				hasIF = true // remapped measure needs creg trajectory
			}
		}
		if hasIF {
			break
		}
	}
	// Stochastic gate noise / IF require per-shot trajectories.
	if hasIF || noise.GateNoise() {
		return runTrajectories(p, shots, n, noise, opt.Seed)
	}

	sv := New(n)
	for _, op := range p.Ops {
		if op.Kind == ir.OpMEASURE {
			break
		}
		if op.Kind == ir.OpPAR {
			for _, bop := range op.Then {
				if err := apply(sv, bop); err != nil {
					return nil, err
				}
			}
			continue
		}
		if op.Kind == ir.OpASSERT {
			continue // no classical bits yet in unitary path
		}
		if err := apply(sv, op); err != nil {
			return nil, err
		}
	}

	seed := opt.Seed
	if seed == 0 {
		seed = rand.Int63()
	}
	rng := rand.New(rand.NewSource(seed))
	counts := sv.Sample(shots, rng)
	counts = noise.CorruptCounts(counts, rng)
	return &Result{
		NumQubits: n,
		Shots:     shots,
		Counts:    counts,
		Probs:     sv.Probs(),
	}, nil
}

func runTrajectories(p *ir.Program, shots, n int, noise NoiseModel, seed int64) (*Result, error) {
	if seed == 0 {
		seed = rand.Int63()
	}
	rng := rand.New(rand.NewSource(seed))
	counts := make(map[string]int)
	for s := 0; s < shots; s++ {
		sv := New(n)
		creg := make([]int, n)
		for _, op := range p.Ops {
			switch op.Kind {
			case ir.OpMEASURE:
				if err := applyMeasure(sv, op, creg, n, rng); err != nil {
					return nil, err
				}
			case ir.OpIF:
				ok, err := evalCond(creg, op)
				if err != nil {
					return nil, err
				}
				runBody := func(body []ir.Op) error {
					for _, bop := range body {
						if err := apply(sv, bop); err != nil {
							return err
						}
						if bop.Kind != ir.OpRESET && bop.Kind != ir.OpBARRIER {
							noise.ApplyAfterGate(sv, bop.Qubits, rng)
						}
					}
					return nil
				}
				if ok {
					if len(op.Then) > 0 {
						if err := runBody(op.Then); err != nil {
							return nil, err
						}
					} else if op.Body != nil {
						if err := apply(sv, *op.Body); err != nil {
							return nil, err
						}
						noise.ApplyAfterGate(sv, op.Body.Qubits, rng)
					} else {
						return nil, fmt.Errorf("simulate: IF missing body")
					}
				} else if len(op.Else) > 0 {
					if err := runBody(op.Else); err != nil {
						return nil, err
					}
				}
			case ir.OpWHILE:
				if op.MaxIter < 1 {
					return nil, fmt.Errorf("simulate: WHILE requires MaxIter >= 1")
				}
				for iter := 0; iter < op.MaxIter; iter++ {
					ok, err := evalCond(creg, op)
					if err != nil {
						return nil, err
					}
					if !ok {
						break
					}
					for _, bop := range op.Then {
						if bop.Kind == ir.OpMEASURE {
							if err := applyMeasure(sv, bop, creg, n, rng); err != nil {
								return nil, err
							}
							continue
						}
						if err := apply(sv, bop); err != nil {
							return nil, err
						}
						if bop.Kind != ir.OpRESET && bop.Kind != ir.OpBARRIER {
							noise.ApplyAfterGate(sv, bop.Qubits, rng)
						}
					}
				}
			case ir.OpSWITCH:
				val, err := switchDisc(creg, op)
				if err != nil {
					return nil, err
				}
				var chosen []ir.Op
				var def []ir.Op
				matched := false
				for _, arm := range op.Cases {
					if arm.Default {
						def = arm.Body
						continue
					}
					if arm.Value == val {
						chosen = arm.Body
						matched = true
						break
					}
				}
				if !matched {
					chosen = def
				}
				for _, bop := range chosen {
					if err := apply(sv, bop); err != nil {
						return nil, err
					}
					if bop.Kind != ir.OpRESET && bop.Kind != ir.OpBARRIER {
						noise.ApplyAfterGate(sv, bop.Qubits, rng)
					}
				}
			case ir.OpPAR:
				for _, bop := range op.Then {
					if err := apply(sv, bop); err != nil {
						return nil, err
					}
					if bop.Kind != ir.OpRESET && bop.Kind != ir.OpBARRIER {
						noise.ApplyAfterGate(sv, bop.Qubits, rng)
					}
				}
			case ir.OpASSERT:
				ok, err := evalCond(creg, op)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("simulate: ASSERT failed (shot %d): %s", s, formatCond(op))
				}
			case ir.OpBARRIER:
				// no-op
			default:
				if err := apply(sv, op); err != nil {
					return nil, err
				}
				if op.Kind != ir.OpRESET {
					noise.ApplyAfterGate(sv, op.Qubits, rng)
				}
			}
		}
		// Final sample of full register (+ optional readout error)
		one := sv.Sample(1, rng)
		for k, v := range one {
			k = noise.ApplyReadout(k, rng)
			counts[k] += v
		}
	}
	return &Result{NumQubits: n, Shots: shots, Counts: counts}, nil
}

func applyMeasure(sv *StateVector, op ir.Op, creg []int, n int, rng *rand.Rand) error {
	qs := op.Qubits
	if len(qs) == 0 {
		qs = make([]int, n)
		for i := range qs {
			qs[i] = i
		}
	}
	for i, q := range qs {
		if q < 0 || q >= n {
			return fmt.Errorf("simulate: MEASURE qubit %d out of range", q)
		}
		bit := sv.MeasureBit(q, rng)
		dst := q
		if i < len(op.MeasTargets) {
			dst = op.MeasTargets[i]
		}
		if dst < 0 || dst >= len(creg) {
			return fmt.Errorf("simulate: MEASURE target c[%d] out of range", dst)
		}
		creg[dst] = bit
	}
	return nil
}

func evalCond(creg []int, op ir.Op) (bool, error) {
	if op.CondRightBit >= 0 {
		if op.CondCbit < 0 || op.CondCbit >= len(creg) {
			return false, fmt.Errorf("simulate: condition c[%d] out of range", op.CondCbit)
		}
		if op.CondRightBit >= len(creg) {
			return false, fmt.Errorf("simulate: condition c[%d] out of range", op.CondRightBit)
		}
		return creg[op.CondCbit] == creg[op.CondRightBit], nil
	}
	if op.CondCbit < 0 {
		// int(c) == CondEq — little-endian bits
		v := 0
		for i := 0; i < len(creg); i++ {
			if creg[i] != 0 {
				v |= 1 << i
			}
		}
		return v == op.CondEq, nil
	}
	if op.CondCbit >= len(creg) {
		return false, fmt.Errorf("simulate: condition c[%d] out of range", op.CondCbit)
	}
	return creg[op.CondCbit] == op.CondEq, nil
}

func switchDisc(creg []int, op ir.Op) (int, error) {
	if op.CondCbit < 0 {
		v := 0
		for i := 0; i < len(creg); i++ {
			if creg[i] != 0 {
				v |= 1 << i
			}
		}
		return v, nil
	}
	if op.CondCbit >= len(creg) {
		return 0, fmt.Errorf("simulate: SWITCH c[%d] out of range", op.CondCbit)
	}
	return creg[op.CondCbit], nil
}

func formatCond(op ir.Op) string {
	if op.CondRightBit >= 0 {
		return fmt.Sprintf("c[%d]==c[%d]", op.CondCbit, op.CondRightBit)
	}
	if op.CondCbit < 0 {
		return fmt.Sprintf("c==%d", op.CondEq)
	}
	return fmt.Sprintf("c[%d]==%d", op.CondCbit, op.CondEq)
}

func apply(sv *StateVector, op ir.Op) error {
	q, a := op.Qubits, op.Args
	need := func(nq, na int) error {
		if len(q) < nq {
			return fmt.Errorf("simulate: gate %s needs %d qubit(s), got %d", op.Kind, nq, len(q))
		}
		if len(a) < na {
			return fmt.Errorf("simulate: gate %s needs %d angle arg(s), got %d", op.Kind, na, len(a))
		}
		return nil
	}

	switch op.Kind {
	case ir.OpH:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.H(q[0])
	case ir.OpX:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.X(q[0])
	case ir.OpY:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.Y(q[0])
	case ir.OpZ:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.Z(q[0])
	case ir.OpS:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.S(q[0])
	case ir.OpSDG:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.SDG(q[0])
	case ir.OpT:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.T(q[0])
	case ir.OpTDG:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.TDG(q[0])
	case ir.OpSX:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.SX(q[0])
	case ir.OpRX:
		if err := need(1, 1); err != nil {
			return err
		}
		sv.RX(q[0], a[0])
	case ir.OpRY:
		if err := need(1, 1); err != nil {
			return err
		}
		sv.RY(q[0], a[0])
	case ir.OpRZ:
		if err := need(1, 1); err != nil {
			return err
		}
		sv.RZ(q[0], a[0])
	case ir.OpP:
		if err := need(1, 1); err != nil {
			return err
		}
		sv.P(q[0], a[0])
	case ir.OpU:
		if err := need(1, 3); err != nil {
			return err
		}
		sv.U(q[0], a[0], a[1], a[2])
	case ir.OpCNOT:
		if err := need(2, 0); err != nil {
			return err
		}
		sv.CNOT(q[0], q[1])
	case ir.OpCZ:
		if err := need(2, 0); err != nil {
			return err
		}
		sv.CZ(q[0], q[1])
	case ir.OpSWAP:
		if err := need(2, 0); err != nil {
			return err
		}
		sv.SWAP(q[0], q[1])
	case ir.OpISWAP:
		if err := need(2, 0); err != nil {
			return err
		}
		sv.ISWAP(q[0], q[1])
	case ir.OpCRX:
		if err := need(2, 1); err != nil {
			return err
		}
		sv.CRX(q[0], q[1], a[0])
	case ir.OpCRY:
		if err := need(2, 1); err != nil {
			return err
		}
		sv.CRY(q[0], q[1], a[0])
	case ir.OpCRZ:
		if err := need(2, 1); err != nil {
			return err
		}
		sv.CRZ(q[0], q[1], a[0])
	case ir.OpCCX:
		if err := need(3, 0); err != nil {
			return err
		}
		sv.CCX(q[0], q[1], q[2])
	case ir.OpCSWAP:
		if err := need(3, 0); err != nil {
			return err
		}
		sv.CSWAP(q[0], q[1], q[2])
	case ir.OpRESET:
		if err := need(1, 0); err != nil {
			return err
		}
		sv.Reset(q[0])
	case ir.OpBARRIER:
		// no-op for simulation — only affects optimizer/scheduling.
	default:
		return fmt.Errorf("simulate: unsupported operation %q", op.Kind)
	}
	return nil
}

// Print writes a terminal histogram to stdout, in the same visual format
// internal/backends.RunResult.Print uses for real-hardware results, so
// `quell run --backend local` and `quell run --backend ibm` (etc.) output
// look consistent.
func (r *Result) Print() {
	fmt.Printf("\nResults — local simulator  (%d shots, %d qubits)\n\n", r.Shots, r.NumQubits)

	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(r.Counts))
	for k, v := range r.Counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})

	maxCount := 0
	if len(pairs) > 0 {
		maxCount = pairs[0].v
	}
	for _, p := range pairs {
		barLen := 0
		if maxCount > 0 {
			barLen = p.v * 30 / maxCount
		}
		bar := strings.Repeat("█", barLen)
		pct := float64(p.v) / float64(r.Shots) * 100
		fmt.Printf("  |%s⟩  %6.2f%%  %s\n", p.k, pct, bar)
	}
	fmt.Println()
}
