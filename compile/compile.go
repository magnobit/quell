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

// Compile parses Quell source code and compiles it to the given target.
// Returns the compiled source string or an error if the input is invalid.
func Compile(src string, target Target) (string, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	return compiler.Compile(c, target)
}
