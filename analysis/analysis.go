// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package analysis is read-only inspection of a Quell circuit on the pure-Go
// statevector: final state, exact observable expectation, gradients, and a
// derivative-free minimiser. It adds no syntax, no IR opcode, and no new
// execution path. Every number comes from simulate.EvolveUnitary, so the
// 24-qubit ceiling and the unitary-prefix rule still apply.
package analysis

import (
	"fmt"
	"math"
	"math/cmplx"
	"sort"

	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

// Backend and strategy labels carried on every result so a number is never
// mistaken for a provider run.
const (
	Backend = "local-statevector"
)

// Model is a parsed, checked circuit ready for analysis.
type Model struct {
	circ *parser.Circuit
	prog *ir.Program
}

// Load parses and checks Quell source.
func Load(src string) (*Model, error) {
	circ, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	if err := check.Fail(circ); err != nil {
		return nil, err
	}
	return &Model{circ: circ, prog: ir.Lower(circ)}, nil
}

// Program returns the lowered, unbound IR program.
func (m *Model) Program() *ir.Program { return m.prog }

// Params lists the unbound PARAM names in first-use order.
func (m *Model) Params() []string { return ir.UnboundParams(m.prog) }

// Observables lists declared observable names.
func (m *Model) Observables() []string {
	out := make([]string, 0, len(m.circ.Observables))
	for _, o := range m.circ.Observables {
		out = append(out, o.Name)
	}
	return out
}

// PrefixOnly reports whether the circuit has operations after its first
// MEASURE (other than more MEASURE or BARRIER). State and Expectation stop at
// the first MEASURE, so for such a circuit they describe only the gates
// before it. Callers must say so rather than present it as the final state.
func (m *Model) PrefixOnly() bool {
	seen := false
	for _, op := range m.prog.Ops {
		if op.Kind == ir.OpMEASURE {
			seen = true
			continue
		}
		if seen && op.Kind != ir.OpBARRIER {
			return true
		}
	}
	return false
}

func (m *Model) bound(params map[string]float64) (*ir.Program, error) {
	if !ir.NeedsBind(m.prog) {
		return m.prog, nil
	}
	return ir.Bind(m.prog, params)
}

// State evolves the unitary prefix with params bound.
func (m *Model) State(params map[string]float64) (*simulate.StateVector, error) {
	p, err := m.bound(params)
	if err != nil {
		return nil, err
	}
	return simulate.EvolveUnitary(p)
}

// Amplitude is one basis state of the final statevector.
type Amplitude struct {
	Bits string  `json:"bits"` // MSB-first, qubit 0 rightmost
	Re   float64 `json:"re"`
	Im   float64 `json:"im"`
	Prob float64 `json:"prob"`
}

// TopAmplitudes returns up to top basis states by probability, highest
// first, dropping states below minProb. top <= 0 returns all that remain.
func TopAmplitudes(sv *simulate.StateVector, top int, minProb float64) []Amplitude {
	if sv == nil {
		return nil
	}
	amps := sv.Amplitudes()
	out := make([]Amplitude, 0, len(amps))
	for i, a := range amps {
		p := real(a)*real(a) + imag(a)*imag(a)
		if p < minProb || p == 0 {
			continue
		}
		out = append(out, Amplitude{
			Bits: fmt.Sprintf("%0*b", sv.N, i),
			Re:   real(a),
			Im:   imag(a),
			Prob: p,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Prob != out[j].Prob {
			return out[i].Prob > out[j].Prob
		}
		return out[i].Bits < out[j].Bits
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

// Phase is the argument of the amplitude in radians.
func (a Amplitude) Phase() float64 { return cmplx.Phase(complex(a.Re, a.Im)) }

// Expectation is the exact <psi|O|psi> for a named observable.
func (m *Model) Expectation(name string, params map[string]float64) (float64, error) {
	sv, err := m.State(params)
	if err != nil {
		return 0, err
	}
	return simulate.ExpectationFromState(sv, m.circ, name)
}

// gradientStep is the central-difference step. Statevector arithmetic is
// double precision, so truncation error is O(h^2) ~ 1e-10.
const gradientStep = 1e-5

// Gradient is the central-difference derivative of the observable with
// respect to each named parameter, evaluated at params. wrt empty means
// every unbound PARAM. This is a finite difference, not the parameter-shift
// rule: a parameter used by several gates, or by a controlled rotation,
// breaks the two-point shift identity, and a finite difference does not.
func (m *Model) Gradient(name string, params map[string]float64, wrt []string) (map[string]float64, error) {
	if len(wrt) == 0 {
		wrt = m.Params()
	}
	known := map[string]bool{}
	for _, p := range m.Params() {
		known[p] = true
	}
	grad := make(map[string]float64, len(wrt))
	for _, n := range wrt {
		if !known[n] {
			return nil, fmt.Errorf("gradient: %q is not a PARAM of this circuit", n)
		}
		plus := cloneParams(params)
		minus := cloneParams(params)
		plus[n] = params[n] + gradientStep
		minus[n] = params[n] - gradientStep
		a, err := m.Expectation(name, plus)
		if err != nil {
			return nil, err
		}
		b, err := m.Expectation(name, minus)
		if err != nil {
			return nil, err
		}
		grad[n] = (a - b) / (2 * gradientStep)
	}
	return grad, nil
}

// MinOptions tunes Minimize. Zero values pick the defaults.
type MinOptions struct {
	MaxIter int     // default 400
	Tol     float64 // default 1e-9 on the spread of simplex values
	Step    float64 // initial simplex edge, default 0.5 radians
}

// MinResult is the outcome of a Minimize run. Converged is false when the
// iteration cap was reached first; Value is still the best point seen.
type MinResult struct {
	Params      map[string]float64 `json:"params"`
	Value       float64            `json:"value"`
	Evaluations int                `json:"evaluations"`
	Iterations  int                `json:"iterations"`
	Converged   bool               `json:"converged"`
	Strategy    string             `json:"strategy"`
	Backend     string             `json:"backend"`
}

// Minimize drives the observable expectation down with Nelder-Mead over the
// circuit's PARAMs, starting at start (missing parameters start at 0.1, not
// 0, because many ansatz circuits have a stationary point at the origin).
// It is a classical outer loop around exact local expectation, which is how
// a variational run is structured. It does not claim a provider ran any step.
func (m *Model) Minimize(name string, start map[string]float64, opt MinOptions) (*MinResult, error) {
	names := m.Params()
	if len(names) == 0 {
		return nil, fmt.Errorf("minimize: circuit has no PARAM to optimise")
	}
	if opt.MaxIter <= 0 {
		opt.MaxIter = 400
	}
	if opt.Tol <= 0 {
		opt.Tol = 1e-9
	}
	if opt.Step <= 0 {
		opt.Step = 0.5
	}
	x0 := make([]float64, len(names))
	for i, n := range names {
		if v, ok := start[n]; ok {
			x0[i] = v
		} else {
			x0[i] = 0.1
		}
	}
	evals := 0
	var firstErr error
	f := func(x []float64) float64 {
		evals++
		p := make(map[string]float64, len(names))
		for i, n := range names {
			p[n] = x[i]
		}
		v, err := m.Expectation(name, p)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return math.Inf(1)
		}
		return v
	}
	x, fx, iters, conv := nelderMead(f, x0, opt)
	if firstErr != nil {
		return nil, firstErr
	}
	out := make(map[string]float64, len(names))
	for i, n := range names {
		out[n] = x[i]
	}
	return &MinResult{
		Params: out, Value: fx, Evaluations: evals, Iterations: iters,
		Converged: conv, Strategy: "nelder-mead", Backend: Backend,
	}, nil
}

func nelderMead(f func([]float64) float64, x0 []float64, opt MinOptions) ([]float64, float64, int, bool) {
	n := len(x0)
	type vertex struct {
		x []float64
		v float64
	}
	sim := make([]vertex, n+1)
	sim[0] = vertex{append([]float64(nil), x0...), f(x0)}
	for i := 0; i < n; i++ {
		x := append([]float64(nil), x0...)
		x[i] += opt.Step
		sim[i+1] = vertex{x, f(x)}
	}
	const alpha, gamma, rho, sigma = 1.0, 2.0, 0.5, 0.5
	order := func() {
		sort.SliceStable(sim, func(i, j int) bool { return sim[i].v < sim[j].v })
	}
	point := func(c []float64, w []float64, t float64) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = c[i] + t*(w[i]-c[i])
		}
		return out
	}
	iter := 0
	for ; iter < opt.MaxIter; iter++ {
		order()
		if math.Abs(sim[n].v-sim[0].v) < opt.Tol {
			return sim[0].x, sim[0].v, iter, true
		}
		c := make([]float64, n)
		for i := 0; i < n; i++ {
			for j := range c {
				c[j] += sim[i].x[j] / float64(n)
			}
		}
		worst := sim[n]
		xr := point(c, worst.x, -alpha)
		fr := f(xr)
		switch {
		case fr < sim[0].v:
			xe := point(c, worst.x, -gamma)
			if fe := f(xe); fe < fr {
				sim[n] = vertex{xe, fe}
			} else {
				sim[n] = vertex{xr, fr}
			}
		case fr < sim[n-1].v:
			sim[n] = vertex{xr, fr}
		default:
			var xc []float64
			if fr < worst.v {
				xc = point(c, worst.x, -rho)
			} else {
				xc = point(c, worst.x, rho)
			}
			if fc := f(xc); fc < math.Min(fr, worst.v) {
				sim[n] = vertex{xc, fc}
			} else {
				for i := 1; i <= n; i++ {
					sim[i].x = point(sim[0].x, sim[i].x, sigma)
					sim[i].v = f(sim[i].x)
				}
			}
		}
	}
	order()
	return sim[0].x, sim[0].v, iter, false
}

func cloneParams(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
