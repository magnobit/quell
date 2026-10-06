// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package compile is the public API for compiling Quell source code to
// third-party quantum SDK formats (Qiskit, OpenQASM 3, Cirq, Braket, Q#).
package compile

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/internal/topology"
	"github.com/magnobit/quell/log"
	"github.com/magnobit/quell/qerr"
)

// Target is a supported compilation target.
type Target = compiler.Target

// Supported compile targets.
const (
	Qiskit    Target = compiler.TargetQiskit
	OpenQASM  Target = compiler.TargetOpenQASM
	OpenQASM2 Target = compiler.TargetOpenQASM2
	Cirq      Target = compiler.TargetCirq
	Braket    Target = compiler.TargetBraket
	QSharp    Target = compiler.TargetQSharp
)

// Targets is the ordered list of all supported targets.
var Targets = []Target{Qiskit, OpenQASM, OpenQASM2, Cirq, Braket, QSharp}

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
	// Fingerprint identifies this source, target, and optimize flag.
	// It is empty when the caller did not supply the original source.
	Fingerprint string
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
//
// src must not contain "import" lines — those need a file on disk to
// resolve paths against, so use CompileFile/CompileFileWithWarnings instead.
func CompileWithWarnings(src string, target Target, optimize bool) (CompileResult, error) {
	c, err := parser.Parse(src)
	if err != nil {
		log.Error("parse failed", "err", err, "target", string(target))
		return CompileResult{}, qerr.Wrap(qerr.KindParse, "compile", err)
	}
	if err := check.Fail(c); err != nil {
		return CompileResult{}, err
	}
	r, err := compileCircuit(c, target, optimize)
	if err != nil {
		log.Error("compile failed", "err", err, "target", string(target), "qubits", c.NumQubits)
		return CompileResult{}, qerr.Compile("compile", err)
	}
	r.Fingerprint = Fingerprint(src, string(target), optimize)
	log.Debug("compile ok", "target", string(target), "qubits", r.NumQubits, "ops", r.NumInstructions, "warnings", len(r.Warnings))
	return r, nil
}

// ResolveTargets parses a target spec. "all" is every supported target.
// A comma-separated list selects those targets, in that order, without duplicates.
func ResolveTargets(spec string) ([]Target, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "all" {
		out := make([]Target, len(Targets))
		copy(out, Targets)
		return out, nil
	}
	if spec == "" {
		return nil, fmt.Errorf("empty target")
	}
	seen := map[Target]bool{}
	var out []Target
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var match Target
		ok := false
		for _, t := range Targets {
			if string(t) == part {
				match, ok = t, true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("unsupported target %q", part)
		}
		if !seen[match] {
			out = append(out, match)
			seen[match] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty target")
	}
	return out, nil
}

// Emit prints each target from an already-lowered program.
// Optimization, when requested, runs once and is shared by every target.
func Emit(prog *ir.Program, targets []Target, optimize bool) (map[Target]CompileResult, error) {
	if prog == nil {
		return nil, qerr.Compile("compile", fmt.Errorf("nil program"))
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("empty target")
	}
	notes := []string{}
	if optimize {
		var optNotes []string
		prog, optNotes = optimizer.Optimize(prog)
		if optNotes != nil {
			notes = optNotes
		}
	}
	out := make(map[Target]CompileResult, len(targets))
	for _, target := range targets {
		code, _, err := compiler.CompileProgram(prog, compiler.Target(target), false)
		if err != nil {
			return nil, qerr.Compile("compile", err)
		}
		out[target] = CompileResult{
			Code:           code,
			Warnings:       []string{},
			OptimizerNotes: notes,
		}
	}
	return out, nil
}

// CompileMany parses, lowers, and optionally optimizes src once, then
// emits every target from that same IR. Target order follows targets.
// Coupling-aware routing is not applied; use CompileWithOptions per
// target when a backend coupling map is required.
func CompileMany(src string, targets []Target, optimize bool) (map[Target]CompileResult, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return nil, qerr.Wrap(qerr.KindParse, "compile", err)
	}
	if err := check.Fail(c); err != nil {
		return nil, err
	}
	prog := ir.Lower(c)
	emitted, err := Emit(prog, targets, optimize)
	if err != nil {
		return nil, err
	}
	warnings := c.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	for target, result := range emitted {
		result.Warnings = warnings
		result.NumQubits = c.NumQubits
		result.NumInstructions = len(c.Instructions)
		result.Fingerprint = Fingerprint(src, string(target), optimize)
		emitted[target] = result
	}
	return emitted, nil
}

// CompileFile parses the .quell file at path — resolving any "import"
// lines relative to its directory, or against an installed package under
// the project's quell.pkg.yml (see parser.ParseFile) — and compiles it to
// target, with the IR optimizer enabled by default.
func CompileFile(path string, target Target) (string, error) {
	r, err := CompileFileWithWarnings(path, target, true)
	if err != nil {
		return "", err
	}
	return r.Code, nil
}

// CompileFileWithWarnings is CompileWithWarnings for a file on disk,
// supporting "import" lines. See parser.ParseFile for import resolution
// rules (relative paths vs. package paths, cycle detection).
func CompileFileWithWarnings(path string, target Target, optimize bool) (CompileResult, error) {
	c, err := parser.ParseFile(path)
	if err != nil {
		log.Error("parse file failed", "path", path, "err", err)
		return CompileResult{}, qerr.Wrap(qerr.KindParse, "compile", err)
	}
	if err := check.Fail(c); err != nil {
		return CompileResult{}, err
	}
	r, err := compileCircuit(c, target, optimize)
	if err != nil {
		log.Error("compile file failed", "path", path, "err", err, "target", string(target))
		return CompileResult{}, qerr.Compile("compile", err)
	}
	log.Info("compiled", "path", path, "target", string(target), "qubits", r.NumQubits)
	return r, nil
}

func compileCircuit(c *parser.Circuit, target Target, optimize bool) (CompileResult, error) {
	return compileCircuitOpts(c, target, CompileOptions{Optimize: optimize})
}

// CompileOptions selects how the IR optimizer runs before target emission.
// Coupling is the selected backend's edge list (scheduler/provider
// topology). Nil Coupling keeps the existing generic Optimize path.
// Never pass a static teaching preset name as CouplingName for live data.
type CompileOptions struct {
	Optimize     bool
	Coupling     [][2]int
	CouplingName string
}

// CompileWithOptions is CompileWithWarnings plus optional coupling-aware
// routing via optimizer.OptimizeWithOptions.
func CompileWithOptions(src string, target Target, opts CompileOptions) (CompileResult, error) {
	c, err := parser.Parse(src)
	if err != nil {
		log.Error("parse failed", "err", err, "target", string(target))
		return CompileResult{}, qerr.Wrap(qerr.KindParse, "compile", err)
	}
	if err := check.Fail(c); err != nil {
		return CompileResult{}, err
	}
	r, err := compileCircuitOpts(c, target, opts)
	if err != nil {
		log.Error("compile failed", "err", err, "target", string(target), "qubits", c.NumQubits)
		return CompileResult{}, qerr.Compile("compile", err)
	}
	return r, nil
}

func compileCircuitOpts(c *parser.Circuit, target Target, opts CompileOptions) (CompileResult, error) {
	var code string
	var notes []string
	var err error
	if len(opts.Coupling) > 0 {
		code, notes, err = compiler.CompileOpts(c, target, opts.Optimize, optimizer.Options{
			Coupling: topology.FromEdges(opts.CouplingName, opts.Coupling),
		})
	} else {
		code, notes, err = compiler.Compile(c, target, opts.Optimize)
	}
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
