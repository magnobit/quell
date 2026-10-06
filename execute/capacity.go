// Copyright 2026 Magnobit, Inc. All rights reserved.

package execute

import "github.com/magnobit/quell/internal/backends"

// SimulatorCapacity is the qubit ceiling of the engine a local-fallback
// backend will actually run. Known is false when that engine cannot
// report a limit.
type SimulatorCapacity = backends.SimulatorCapacity

// DiscoverSimulatorCapacity reports that ceiling for nvidia or intel.
// Other backend ids stay unknown.
func DiscoverSimulatorCapacity(backend string) SimulatorCapacity {
	return backends.DiscoverSimulatorCapacity(backend)
}
