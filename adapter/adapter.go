// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package adapter defines the BackendAdapter contract: every provider runs
// from Quell IR (*ir.Program), not from raw source. Built-in adapters wrap
// the existing hardware submit paths; third parties register in-process via
// Register (Go plugin .so loading is Linux-only and not required for extension).
package adapter

import (
	"fmt"
	"sync"

	"github.com/magnobit/quell/internal/backends"
	"github.com/magnobit/quell/internal/ir"
)

// Result is the shared measurement / job outcome type.
type Result = backends.RunResult

// Job is the uniform input to every BackendAdapter.
type Job struct {
	Program *ir.Program
	// Optimize runs the IR optimizer before codegen (hardware adapters).
	Optimize bool
	// Shots overrides provider config when > 0 (simulator / local).
	Shots int
	// Config is provider credentials (*config.IBMConfig, etc.).
	Config any
	// QuellSource is optional original Quell text (NVIDIA/Intel local fallback).
	QuellSource string
	// Noise is only used by the simulator adapter.
	Noise any // *simulate.NoiseModel or simulate.NoiseModel — kept any to avoid cycles in docs
}

// BackendAdapter is the contract every backend must implement.
type BackendAdapter interface {
	// Name is the catalog id: ibm, ionq, google, rigetti, aws, azure, local, …
	Name() string
	// Run executes the IR program on this backend.
	Run(job *Job) (*Result, error)
}

var (
	mu       sync.RWMutex
	registry = map[string]BackendAdapter{}
)

// Register adds or replaces an adapter (built-ins and third-party plugins).
func Register(a BackendAdapter) {
	if a == nil || a.Name() == "" {
		return
	}
	mu.Lock()
	registry[a.Name()] = a
	mu.Unlock()
}

// Lookup returns a registered adapter by name.
func Lookup(name string) (BackendAdapter, error) {
	ensureBuiltins()
	mu.RLock()
	a, ok := registry[name]
	mu.RUnlock()
	if !ok || a == nil {
		return nil, fmt.Errorf("adapter: unknown backend %q — registered: %v", name, Names())
	}
	return a, nil
}

// Names returns registered backend ids (sorted for stability in tests via copy).
func Names() []string {
	ensureBuiltins()
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// Run looks up name and executes job.
func Run(name string, job *Job) (*Result, error) {
	a, err := Lookup(name)
	if err != nil {
		return nil, err
	}
	if job == nil || job.Program == nil {
		return nil, fmt.Errorf("adapter: job requires a non-nil IR program")
	}
	return a.Run(job)
}
