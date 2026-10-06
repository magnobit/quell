// Copyright 2026 Magnobit, Inc. All rights reserved.

package simulate

import (
	"fmt"
	"math"
	"math/cmplx"

	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func init() {
	check.ExpectationHook = ExpectationOf
}

// ExpectationOf evaluates a named observable on the unitary prefix of c.
// The pure-Go statevector is the local strategy. Provider-native
// expectation is recorded separately when a backend supplies it.
func ExpectationOf(c *parser.Circuit, name string) (float64, error) {
	if c == nil {
		return 0, fmt.Errorf("expectation: nil circuit")
	}
	var obs *parser.ObservableDecl
	for i := range c.Observables {
		if c.Observables[i].Name == name {
			obs = &c.Observables[i]
			break
		}
	}
	if obs == nil {
		return 0, fmt.Errorf("expectation: unknown observable %q", name)
	}
	sv, err := EvolveUnitary(ir.Lower(c))
	if err != nil {
		return 0, err
	}
	return expectObservable(sv, obs), nil
}

func expectObservable(sv *StateVector, obs *parser.ObservableDecl) float64 {
	if sv == nil || obs == nil {
		return 0
	}
	sum := 0.0
	for _, term := range obs.Terms {
		sum += term.Coeff * expectTerm(sv, term.Ops)
	}
	return sum
}

func expectTerm(sv *StateVector, ops []parser.PauliOp) float64 {
	worked := &StateVector{N: sv.N, dim: sv.dim, amp: append([]complex128(nil), sv.amp...)}
	for _, op := range ops {
		switch op.Axis {
		case "X":
			worked.X(op.Qubit)
		case "Y":
			worked.Y(op.Qubit)
		case "Z":
			worked.Z(op.Qubit)
		}
	}
	dot := complex(0, 0)
	for i := range sv.amp {
		dot += cmplx.Conj(sv.amp[i]) * worked.amp[i]
	}
	return real(dot)
}

// ExpectationFromState evaluates a named observable on an already evolved
// state. Callers that bind PARAM values themselves use this instead of
// ExpectationOf, which lowers the circuit unbound.
func ExpectationFromState(sv *StateVector, c *parser.Circuit, name string) (float64, error) {
	if sv == nil || c == nil {
		return 0, fmt.Errorf("expectation: nil state or circuit")
	}
	for i := range c.Observables {
		if c.Observables[i].Name == name {
			return expectObservable(sv, &c.Observables[i]), nil
		}
	}
	return 0, fmt.Errorf("expectation: unknown observable %q", name)
}

// ExpectationResult is the local-simulator result. Variance is set when
// shots are used; the exact statevector path leaves Variance nil.
type ExpectationResult struct {
	Value    float64
	Shots    int
	Variance *float64
	Backend  string
	Strategy string
}

// ExactExpectation is the statevector strategy. It does not sample.
func ExactExpectation(c *parser.Circuit, name string) (ExpectationResult, error) {
	v, err := ExpectationOf(c, name)
	if err != nil {
		return ExpectationResult{}, err
	}
	if math.IsNaN(v) {
		return ExpectationResult{}, fmt.Errorf("expectation: not a number")
	}
	return ExpectationResult{
		Value:    v,
		Backend:  "local-statevector",
		Strategy: "statevector",
	}, nil
}
