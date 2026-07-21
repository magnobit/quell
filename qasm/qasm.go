// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qasm is the public OpenQASM → Quell import API (thin subset).
package qasm

import "github.com/magnobit/quell/internal/qasmimport"

// ToQuell converts a subset of OpenQASM 3 into Quell source.
func ToQuell(src string) (string, error) {
	return qasmimport.ToQuell(src)
}
