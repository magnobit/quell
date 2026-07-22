package qasm

import "github.com/magnobit/quell/internal/qasmimport"

// Result is a Quell conversion with soft warnings for skipped/unsupported lines.
type Result = qasmimport.Result

// ToQuell converts a subset of OpenQASM 2/3 into Quell source.
func ToQuell(src string) (string, error) {
	return qasmimport.ToQuell(src)
}

// Convert is ToQuell plus structured warnings (unsupported / skipped lines).
func Convert(src string) (Result, error) {
	return qasmimport.Convert(src)
}
