// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"

	"github.com/magnobit/quell/internal/config"
)

// RunNVIDIA is the planned NVIDIA cuQuantum / CUDA-Q adapter.
// Gate Quell circuits will eventually compile to a CUDA-Q / cuQuantum
// simulation path for large-scale GPU simulation. Until that lands,
// this stub keeps nvidia in the backend catalog without pretending
// a submission works.
func RunNVIDIA(cfg *config.NVIDIAConfig, qasm3 string) (*RunResult, error) {
	_ = cfg
	_ = qasm3
	return nil, fmt.Errorf("nvidia: NVIDIA cuQuantum / CUDA-Q backend is planned — GPU simulation adapter not implemented yet; use backend: local or a live gate provider (ibm, aws, google, rigetti, ionq, azure)")
}
