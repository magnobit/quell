// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"
	"os"

	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/simulate"
)

// RunIntel executes via Intel Quantum SDK when available (QUELL_INTEL_SDK=1
// and a future bridge). Today it runs Quell's local statevector so Cloud
// plumbing and credential storage work — Backend notes the fallback.
// Set QUELL_INTEL_REQUIRE_SDK=1 to refuse the local fallback.
func RunIntel(cfg *config.IntelConfig, qasm3 string) (*RunResult, error) {
	if cfg == nil {
		cfg = &config.IntelConfig{}
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1024
	}

	quellSrc := ""
	if cfg.Extra != nil {
		quellSrc = cfg.Extra["quell_source"]
	}

	// Placeholder for a future Intel Quantum SDK / LLVM bridge.
	if os.Getenv("QUELL_INTEL_SDK") == "1" {
		if counts, err := tryIntelSDK(qasm3, shots); err == nil {
			return NewResult("intel", "intel-sdk", "Intel Quantum SDK", "intel-sdk", shots, counts), nil
		} else if os.Getenv("QUELL_INTEL_REQUIRE_SDK") == "1" {
			return nil, fmt.Errorf("intel: SDK required but unavailable: %w", err)
		}
	} else if os.Getenv("QUELL_INTEL_REQUIRE_SDK") == "1" {
		return nil, fmt.Errorf("intel: set QUELL_INTEL_SDK=1 and install the Intel Quantum SDK (local fallback disabled)")
	}

	if quellSrc == "" {
		return nil, fmt.Errorf("intel: no quell_source for local fallback")
	}
	res, err := simulate.Run(quellSrc, shots)
	if err != nil {
		return nil, fmt.Errorf("intel: local fallback: %w", err)
	}
	return NewFallback("intel", "local-statevector", "Intel / local-statevector-fallback", "intel-local-fallback", shots, res.Counts), nil
}

func tryIntelSDK(qasm3 string, shots int) (map[string]int, error) {
	_ = qasm3
	_ = shots
	return nil, fmt.Errorf("Intel Quantum SDK bridge not installed")
}
