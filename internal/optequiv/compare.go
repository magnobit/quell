// Copyright 2026 Magnobit, Inc. All rights reserved.

package optequiv

import (
	"fmt"
	"math/cmplx"
	"sort"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/simulate"
)

// stateFidelity returns |<ψ|φ>|² and the max |ψ_i − e^{iθ}φ_i| after
// aligning the global phase so the overlap is real and non-negative.
// Naive element-wise equality is intentionally not used.
func stateFidelity(psi, phi []complex128) (fidelity, maxAmpDelta float64) {
	if len(psi) == 0 || len(psi) != len(phi) {
		return 0, 1
	}
	var ov complex128
	for i := range psi {
		ov += cmplx.Conj(psi[i]) * phi[i]
	}
	fidelity = real(ov)*real(ov) + imag(ov)*imag(ov)
	if fidelity > 1 {
		fidelity = 1
	}
	abs := cmplx.Abs(ov)
	phase := complex(1, 0)
	if abs > 0 {
		phase = ov / complex(abs, 0)
	}
	for i := range psi {
		aligned := phi[i] * cmplx.Conj(phase)
		d := cmplx.Abs(psi[i] - aligned)
		if d > maxAmpDelta {
			maxAmpDelta = d
		}
	}
	return fidelity, maxAmpDelta
}

type measPair struct {
	Qubit int
	Cbit  int
}

func measurementPlan(p *ir.Program) []measPair {
	if p == nil {
		return nil
	}
	n := p.NumQubits
	if n < 1 {
		n = 1
	}
	var plan []measPair
	for _, op := range p.Ops {
		if op.Kind != ir.OpMEASURE {
			continue
		}
		qs := op.Qubits
		if len(qs) == 0 {
			qs = make([]int, n)
			for i := range qs {
				qs[i] = i
			}
		}
		for i, q := range qs {
			c := q
			if i < len(op.MeasTargets) {
				c = op.MeasTargets[i]
			}
			plan = append(plan, measPair{Qubit: q, Cbit: c})
		}
	}
	return plan
}

func measurementKey(p *ir.Program) string {
	plan := measurementPlan(p)
	s := ""
	for _, m := range plan {
		s += fmt.Sprintf("%d->%d;", m.Qubit, m.Cbit)
	}
	return s
}

func classicalWidth(plan []measPair) int {
	w := 0
	for _, m := range plan {
		if m.Cbit+1 > w {
			w = m.Cbit + 1
		}
	}
	return w
}

func classicalProbs(p *ir.Program) (map[string]float64, error) {
	sv, err := simulate.EvolveUnitary(p)
	if err != nil {
		return nil, fmt.Errorf("exact distribution requires a unitary prefix: %w", err)
	}
	plan := measurementPlan(p)
	if len(plan) == 0 {
		return nil, fmt.Errorf("exact distribution requires at least one measurement")
	}
	width := classicalWidth(plan)
	if width < 1 {
		width = 1
	}
	amps := sv.Amplitudes()
	nq := sv.N
	out := map[string]float64{}
	for i, a := range amps {
		pr := real(a)*real(a) + imag(a)*imag(a)
		if pr == 0 {
			continue
		}
		cbits := make([]int, width)
		for _, m := range plan {
			if m.Qubit < 0 || m.Qubit >= nq || m.Cbit < 0 || m.Cbit >= width {
				continue
			}
			cbits[m.Cbit] = (i >> m.Qubit) & 1
		}
		out[bitString(cbits)] += pr
	}
	return out, nil
}

func bitString(cbits []int) string {
	v := 0
	for i, b := range cbits {
		if b != 0 {
			v |= 1 << i
		}
	}
	return fmt.Sprintf("%0*b", len(cbits), v)
}

func distDistance(a, b map[string]float64) (tvd, maxAbs float64) {
	keys := map[string]struct{}{}
	for k, v := range a {
		if v > 0 {
			keys[k] = struct{}{}
		}
	}
	for k, v := range b {
		if v > 0 {
			keys[k] = struct{}{}
		}
	}
	var union []string
	for k := range keys {
		union = append(union, k)
	}
	sort.Strings(union)
	sum := 0.0
	for _, k := range union {
		d := a[k] - b[k]
		if d < 0 {
			d = -d
		}
		sum += d
		if d > maxAbs {
			maxAbs = d
		}
	}
	return 0.5 * sum, maxAbs
}

func debugSteps() []struct {
	name string
	fn   func(*ir.Program) (*ir.Program, []string)
} {
	names := optimizer.PassNames()
	out := make([]struct {
		name string
		fn   func(*ir.Program) (*ir.Program, []string)
	}, len(names))
	for i, n := range names {
		name := n
		out[i] = struct {
			name string
			fn   func(*ir.Program) (*ir.Program, []string)
		}{
			name: name,
			fn: func(p *ir.Program) (*ir.Program, []string) {
				return optimizer.ApplyPass(p, name)
			},
		}
	}
	return out
}

func naiveElementEqual(a, b []complex128, tol float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if cmplx.Abs(a[i]-b[i]) > tol {
			return false
		}
	}
	return true
}
