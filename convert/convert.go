// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package convert turns foreign quantum source (OpenQASM, and thin helpers)
// into Quell. Rich Qiskit/Cirq/Q# migrate lives in the platform AI package
// today; this package keeps CLI/local OpenQASM conversion comment-safe.
package convert

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/magnobit/quell/internal/qasmimport"
)

// Result is Quell output plus soft warnings.
type Result struct {
	Quell    string
	Language string
	Warnings []string
}

// DetectLanguage guesses the source language from content or filename.
func DetectLanguage(code, filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".qasm"), strings.HasSuffix(lower, ".qasm2"), strings.HasSuffix(lower, ".qasm3"):
		return "openqasm"
	case strings.HasSuffix(lower, ".qs"), strings.HasSuffix(lower, ".qsharp"):
		return "qsharp"
	case strings.HasSuffix(lower, ".py"):
		if strings.Contains(code, "cirq.") || strings.Contains(code, "import cirq") {
			return "cirq"
		}
		if strings.Contains(code, "braket") {
			return "braket"
		}
		return "qiskit"
	}
	c := strings.ToLower(code)
	if strings.Contains(c, "openqasm") || strings.Contains(c, "qreg ") || strings.Contains(c, "qubit[") {
		return "openqasm"
	}
	if strings.Contains(code, "operation ") || strings.Contains(code, "Microsoft.Quantum") {
		return "qsharp"
	}
	if strings.Contains(code, "cirq.") {
		return "cirq"
	}
	if strings.Contains(code, "braket") {
		return "braket"
	}
	if strings.Contains(code, "QuantumCircuit") || strings.Contains(code, "qc.") {
		return "qiskit"
	}
	return "unknown"
}

// FromOpenQASM converts OpenQASM 2/3 → Quell (comment-safe).
func FromOpenQASM(src string) (Result, error) {
	r, err := qasmimport.Convert(src)
	return Result{Quell: r.Quell, Language: "openqasm", Warnings: r.Warnings}, err
}

// FromFile converts a source file to Quell when a local converter exists.
// OpenQASM is fully local. Other languages return a clear error pointing at
// Labs Migrate / POST /api/v1/ai/convert (which has the full Qiskit/Cirq/Q#/Braket importers).
func FromFile(path string) (Result, error) {
	return Result{}, fmt.Errorf("use FromOpenQASM for .qasm, or platform /api/v1/ai/convert for %s", filepath.Ext(path))
}
