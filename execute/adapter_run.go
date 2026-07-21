// Copyright 2026 Magnobit, Inc. All rights reserved.

package execute

import (
	"fmt"

	"github.com/magnobit/quell/adapter"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// RunOpts configures an IR-based adapter run.
type RunOpts struct {
	Optimize    bool
	Shots       int
	QuellSource string // required for NVIDIA/Intel fallback when Config has no Extra
	Noise       any
}

// RunQuell parses Quell source → IR → BackendAdapter. Preferred path for
// Cloud / CLI so every provider shares the same contract.
func RunQuell(backend, quellSource string, cfg any, opts RunOpts) (*Result, error) {
	if quellSource == "" {
		return nil, fmt.Errorf("execute: quell source required for IR adapter path")
	}
	circ, err := parser.Parse(quellSource)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	prog := ir.Lower(circ)
	name := backend
	if name == "" {
		name = Local
	}
	src := opts.QuellSource
	if src == "" {
		src = quellSource
	}
	return adapter.Run(name, &adapter.Job{
		Program:     prog,
		Optimize:    opts.Optimize,
		Shots:       opts.Shots,
		Config:      cfg,
		QuellSource: src,
		Noise:       opts.Noise,
	})
}

// RunProgram runs an already-lowered IR program on a named adapter.
// Callers inside this module (or tests) that already have IR use this.
func RunProgram(backend string, prog *ir.Program, cfg any, opts RunOpts) (*Result, error) {
	return adapter.Run(backend, &adapter.Job{
		Program:     prog,
		Optimize:    opts.Optimize,
		Shots:       opts.Shots,
		Config:      cfg,
		QuellSource: opts.QuellSource,
		Noise:       opts.Noise,
	})
}
