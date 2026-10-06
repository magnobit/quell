// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qtest runs Quell programs on the local simulator.
// It does not call a paid provider.
package qtest

import (
	"fmt"

	"math"

	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/engine"
	"github.com/magnobit/quell/internal/host"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

// ExpectCompiles fails when Parse or Check fails.
func ExpectCompiles(src string) error {
	c, err := parser.Parse(src)
	if err != nil {
		return err
	}
	if err := check.Fail(c); err != nil {
		return err
	}
	return nil
}

// ExpectCounts fails when bitstring is less than minFrac of the shots.
func ExpectCounts(src, bits string, shots int, minFrac float64) error {
	res, err := simulate.Run(src, shots)
	if err != nil {
		return err
	}
	got := res.Counts[bits]
	if float64(got) < minFrac*float64(shots) {
		return fmt.Errorf("counts[%s]=%d, want at least %.0f%% of %d", bits, got, minFrac*100, shots)
	}
	return nil
}

// ExpectError fails when src compiles.
func ExpectError(src string) error {
	if err := ExpectCompiles(src); err == nil {
		return fmt.Errorf("expected a compile error")
	}
	return nil
}

// ExpectStdout evaluates call, which must be a host call such as say(),
// and compares captured print output.
func ExpectStdout(src, call, want string) error {
	c, err := parser.Parse(src)
	if err != nil {
		return err
	}
	if err := check.Fail(c); err != nil {
		return err
	}
	buf := &host.Buffer{}
	host.SetWriter(buf)
	defer host.SetWriter(nil)
	e, err := parser.ParseExpr(call, 1, 1)
	if err != nil {
		return err
	}
	if _, ds := check.EvalIn(c, e); len(ds) > 0 {
		return fmt.Errorf("%v", ds)
	}
	if buf.String() != want {
		return fmt.Errorf("stdout %q, want %q", buf.String(), want)
	}
	return nil
}

func evolve(src string) (*simulate.StateVector, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	if err := check.Fail(c); err != nil {
		return nil, err
	}
	return simulate.EvolveUnitary(ir.Lower(c))
}

// ExpectProbability compares the exact local statevector probability.
func ExpectProbability(src, bits string, want, tol float64) error {
	sv, err := evolve(src)
	if err != nil {
		return err
	}
	got, err := sv.Probability(bits)
	if err != nil {
		return err
	}
	if math.Abs(got-want) > tol {
		return fmt.Errorf("P(%s)=%g, want %g ± %g", bits, got, want, tol)
	}
	return nil
}

// ExpectState requires the named basis state to hold the amplitude, up to tolerance on each part.
func ExpectState(src, bits string, re, im, tol float64) error {
	sv, err := evolve(src)
	if err != nil {
		return err
	}
	amp, err := sv.Amplitude(bits)
	if err != nil {
		return err
	}
	if math.Abs(real(amp)-re) > tol || math.Abs(imag(amp)-im) > tol {
		return fmt.Errorf("amplitude(%s)=%v, want %g+%gi", bits, amp, re, im)
	}
	return nil
}

// ExpectFidelity compares two local statevectors. Neither circuit calls a provider.
func ExpectFidelity(src, other string, min float64) error {
	a, err := evolve(src)
	if err != nil {
		return err
	}
	b, err := evolve(other)
	if err != nil {
		return err
	}
	f, err := simulate.Fidelity(a, b)
	if err != nil {
		return err
	}
	if f+1e-12 < min {
		return fmt.Errorf("fidelity %g, want at least %g", f, min)
	}
	return nil
}

// ExpectTVD fails when the local distributions differ by more than max.
func ExpectTVD(src, other string, max float64) error {
	a, err := evolve(src)
	if err != nil {
		return err
	}
	b, err := evolve(other)
	if err != nil {
		return err
	}
	d, err := simulate.TVD(a, b)
	if err != nil {
		return err
	}
	if d > max {
		return fmt.Errorf("tvd %g, want at most %g", d, max)
	}
	return nil
}

// ExpectProviderCapability reads the local engine list. It does not call a provider.
func ExpectProviderCapability(name, status string) error {
	for _, info := range engine.Capabilities() {
		if info.Name == name {
			if info.Status != status {
				return fmt.Errorf("engine %s status %s, want %s (%s)", name, info.Status, status, info.Detail)
			}
			return nil
		}
	}
	return fmt.Errorf("engine %s is not listed", name)
}
