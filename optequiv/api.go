// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package optequiv is the public optimizer-equivalence API. The
// implementation lives in internal/optequiv; this wrapper exists so
// other modules (the platform API) can call it without importing
// Quell internals.
package optequiv

import (
	"github.com/magnobit/quell/internal/ir"
	intopt "github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/parser"
)

type Evidence = intopt.Evidence
type Options = intopt.Options
type StatisticalEvidence = intopt.StatisticalEvidence
type StatisticalCompare = intopt.StatisticalCompare

const (
	StatusEquivalent    = intopt.StatusEquivalent
	StatusNotEquivalent = intopt.StatusNotEquivalent
	StatusInconclusive  = intopt.StatusInconclusive
	StatusUnsupported   = intopt.StatusUnsupported

	TierExactStatevector  = intopt.TierExactStatevector
	TierExactDistribution = intopt.TierExactDistribution
	TierStatistical       = intopt.TierStatistical
	TierUnsupported       = intopt.TierUnsupported

	DefaultTolerance     = intopt.DefaultTolerance
	MaxExactQubits       = intopt.MaxExactQubits
	MaxStatisticalQubits = intopt.MaxStatisticalQubits
)

// VerifySource parses src, binds params from opt, runs the conservative
// optimizer, and returns structured equivalence evidence.
func VerifySource(src string, opt Options) (Evidence, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return Evidence{}, err
	}
	return intopt.VerifyOptimization(ir.Lower(c), opt), nil
}

// CompareSource compares two programs without re-optimizing the candidate.
func CompareSource(original, optimized string, opt Options) (Evidence, error) {
	a, err := parser.Parse(original)
	if err != nil {
		return Evidence{}, err
	}
	b, err := parser.Parse(optimized)
	if err != nil {
		return Evidence{}, err
	}
	return intopt.Compare(ir.Lower(a), ir.Lower(b), opt), nil
}
