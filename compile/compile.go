// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package compile is the public API for compiling Quell source code to
// third-party quantum SDK formats (Qiskit, OpenQASM 3, Cirq, Braket).
package compile

import (
	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/parser"
)

// Target is a supported compilation target.
type Target = compiler.Target

// Supported compile targets.
const (
	Qiskit   Target = compiler.TargetQiskit
	OpenQASM Target = compiler.TargetOpenQASM
	Cirq     Target = compiler.TargetCirq
	Braket   Target = compiler.TargetBraket
)

// Targets is the ordered list of all supported targets.
var Targets = []Target{Qiskit, OpenQASM, Cirq, Braket}

// CompileResult holds compiled output and any non-fatal semantic warnings.
// Warnings describe issues that compile successfully but may produce unexpected
// results (e.g. no MEASURE instruction, circuit depth exceeding hardware limits).
type CompileResult struct {
	Code     string
	Warnings []string
}

// Compile parses and compiles Quell source to the given target.
// Returns the compiled source string or an error if the input is invalid.
// Semantic warnings are discarded; use CompileWithWarnings to retrieve them.
func Compile(src string, target Target) (string, error) {
	r, err := CompileWithWarnings(src, target)
	if err != nil {
		return "", err
	}
	return r.Code, nil
}

// CompileWithWarnings parses and compiles Quell source, returning both the
// compiled output and any non-fatal semantic warnings alongside it.
// Warnings are never empty strings; callers should surface them to users.
func CompileWithWarnings(src string, target Target) (CompileResult, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return CompileResult{}, err
	}
	code, err := compiler.Compile(c, target)
	if err != nil {
		return CompileResult{}, err
	}
	warnings := c.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return CompileResult{Code: code, Warnings: warnings}, nil
}
