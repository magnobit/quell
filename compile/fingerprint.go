// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"sort"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
)

// Version identifiers snapshotted into Experiment toolchain JSON at
// create time. They name the IR/compiler/optimizer *schema*, not a
// mutable "whatever this process happens to be running later."
const (
	IRVersion        = ir.CanonicalVersion
	CompilerVersion  = "quell-compiler-v1"
	OptimizerVersion = "quell-optimizer-v1"
)

// ParamBinding is one named input used (or declared) by a parameterized circuit.
type ParamBinding struct {
	Name  string   `json:"name"`
	Value *float64 `json:"value"`
	Bound bool     `json:"bound"`
}

// ExecutionSnapshot is the compile-time provenance captured when a
// scheduled job becomes an Experiment: the IR actually associated with
// the run (bound when parameter values were supplied), plus the optimizer
// configuration the execution path uses today.
type ExecutionSnapshot struct {
	IRHash           string
	OriginalIRHash   string
	OptimizedIRHash  string
	Parameters       []ParamBinding
	IRVersion        string
	CompilerVersion  string
	OptimizerVersion string
	OptimizerPasses  []string
	OptimizerNotes   []string
	OptimizerConfig  map[string]any
}

// SnapshotExecution parses src, optionally binds params, hashes the
// canonical IR, and records the conservative optimizer passes/notes.
// A parse error is returned; callers that already resolved source to
// Quell should treat that as unexpected.
func SnapshotExecution(src string, params map[string]float64) (ExecutionSnapshot, error) {
	id := BuildIdentity()
	out := ExecutionSnapshot{
		IRVersion:        IRVersion,
		CompilerVersion:  id.PinnedCompiler(),
		OptimizerVersion: id.PinnedOptimizer(),
		OptimizerPasses:  optimizer.PassNames(),
		OptimizerNotes:   []string{},
		OptimizerConfig: map[string]any{
			"optimize":   true,
			"coupling":   nil,
			"noiseAware": false,
		},
	}

	c, err := parser.Parse(src)
	if err != nil {
		return out, err
	}
	prog := ir.Lower(c)
	out.Parameters = collectBindings(prog, params)

	if len(params) > 0 {
		if bound, berr := ir.Bind(prog, params); berr == nil {
			prog = bound
		}
	}

	optProg, notes := optimizer.Optimize(prog)
	if notes != nil {
		out.OptimizerNotes = notes
	}
	out.IRHash = ir.Hash(prog)
	out.OriginalIRHash = out.IRHash
	out.OptimizedIRHash = ir.Hash(optProg)
	return out, nil
}

// HashIR is a convenience wrapper around SnapshotExecution for callers
// that only need the digest.
func HashIR(src string, params map[string]float64) (string, error) {
	snap, err := SnapshotExecution(src, params)
	return snap.IRHash, err
}

func collectBindings(prog *ir.Program, params map[string]float64) []ParamBinding {
	names := map[string]struct{}{}
	for _, n := range prog.Params {
		if n != "" {
			names[n] = struct{}{}
		}
	}
	for _, n := range ir.UnboundParams(prog) {
		if n != "" {
			names[n] = struct{}{}
		}
	}
	for n := range params {
		if n != "" {
			names[n] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)

	out := make([]ParamBinding, 0, len(ordered))
	for _, n := range ordered {
		b := ParamBinding{Name: n}
		if params != nil {
			if v, ok := params[n]; ok {
				val := v
				b.Value = &val
				b.Bound = true
			} else {
				for k, v := range params {
					if stringsEqualFold(k, n) {
						val := v
						b.Value = &val
						b.Bound = true
						break
					}
				}
			}
		}
		out = append(out, b)
	}
	return out
}

func stringsEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
