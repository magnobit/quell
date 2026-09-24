// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"github.com/magnobit/quell/optequiv"
)

// VerifyOptimization parses src, binds params, runs the conservative
// optimizer, and returns structured equivalence evidence. It does not
// compile to a backend and does not claim compiler-level equivalence.
func VerifyOptimization(src string, params map[string]float64) (optequiv.Evidence, error) {
	return optequiv.VerifySource(src, optequiv.Options{
		Params:           params,
		OptimizerVersion: BuildIdentity().PinnedOptimizer(),
	})
}
