// Copyright 2026 Magnobit, Inc. All rights reserved.

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

// CompileResult holds compiled output, any non-fatal semantic warnings, and
// any notes from the IR optimizer. Warnings describe issues that compile
// successfully but may produce unexpected results (e.g. no MEASURE
// instruction, circuit depth exceeding hardware limits). OptimizerNotes
// describe changes the optimizer made (e.g. dropped no-op gates, cancelled
// gate pairs, fused rotations) — always empty when optimization is disabled.
type CompileResult struct {
	Code           string
	Warnings       []string
	OptimizerNotes []string
	// NumQubits is the parsed circuit's qubit count — callers that go on to
	// submit the compiled output to real hardware (see the execute package)
	// need this for backends whose results API returns a packed bitstring
	// without the width, e.g. RunIBM/RunIonQ.
	NumQubits int
	// NumInstructions is the parsed circuit's gate/instruction count, for
	// callers (e.g. the CLI) that report it without needing direct access
	// to the parser.
	NumInstructions int
}

// Compile parses and compiles Quell source to the given target, with the
// conservative IR optimizer enabled by default. Returns the compiled source
// string or an error if the input is invalid. Semantic warnings and
// optimizer notes are discarded; use CompileWithWarnings to retrieve them.
func Compile(src string, target Target) (string, error) {
	r, err := CompileWithWarnings(src, target, true)
	if err != nil {
		return "", err
	}
	return r.Code, nil
}

// CompileWithWarnings parses and compiles Quell source, returning the
// compiled output, any non-fatal semantic warnings, and any optimizer notes.
// Warnings and notes are never empty strings; callers should surface them to
// users. Set optimize to false to skip the IR optimizer passes and get a
// direct, unoptimized translation of the parsed circuit.
func CompileWithWarnings(src string, target Target, optimize bool) (CompileResult, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return CompileResult{}, err
	}
	code, notes, err := compiler.Compile(c, target, optimize)
	if err != nil {
		return CompileResult{}, err
	}
	warnings := c.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	if notes == nil {
		notes = []string{}
	}
	return CompileResult{Code: code, Warnings: warnings, OptimizerNotes: notes, NumQubits: c.NumQubits, NumInstructions: len(c.Instructions)}, nil
}
