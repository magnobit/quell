// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package profile times the local compile pipeline and estimates resources.
// Provider stages are recorded only when a caller supplies them.
package profile

import (
	"fmt"
	"strings"
	"time"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

// Stage is one timed step. Local is false for scheduler and provider time.
type Stage struct {
	Name     string
	Local    bool
	Duration time.Duration
	Skipped  bool
}

// Resources is a static estimate from canonical IR. It is not a quote.
type Resources struct {
	Qubits    int
	Gates     int
	TwoQubit  int
	Depth     int
	Swaps     int
	SimMemory int
	Shots     int
	Target    string
}

// Report separates local pipeline time from remote stages.
type Report struct {
	Stages    []Stage
	Resources Resources
}

// Measure times parse, check, lower, optimize, bind, compile, and simulate.
// Schedule, queue, provider execution, and result retrieval stay skipped
// unless FillRemote is called with a measured duration.
func Measure(src string, shots int) Report {
	var rep Report
	rep.Resources.Shots = shots
	rep.Resources.Target = "local-statevector"
	var circ *parser.Circuit
	rep.Stages = append(rep.Stages, timeStage("parse", true, func() {
		circ, _ = parser.Parse(src)
	}))
	rep.Stages = append(rep.Stages, timeStage("check", true, func() {
		if circ != nil {
			_ = check.Check(circ)
		}
	}))
	var prog *ir.Program
	rep.Stages = append(rep.Stages, timeStage("lower", true, func() {
		if circ != nil {
			prog = ir.Lower(circ)
		}
	}))
	rep.Stages = append(rep.Stages, timeStage("optimize", true, func() {
		if prog != nil {
			prog, _ = optimizer.Optimize(prog)
		}
	}))
	rep.Stages = append(rep.Stages, timeStage("bind", true, func() {
		if prog != nil && ir.NeedsBind(prog) {
			prog, _ = ir.Bind(prog, nil)
		}
	}))
	rep.Stages = append(rep.Stages, timeStage("compile", true, func() {
		_, _ = compile.CompileWithWarnings(src, compile.OpenQASM, true)
	}))
	rep.Stages = append(rep.Stages, timeStage("simulate", true, func() {
		if shots > 0 {
			_, _ = simulate.Run(src, shots)
		}
	}))
	for _, name := range []string{"schedule", "queue", "provider", "result"} {
		rep.Stages = append(rep.Stages, Stage{Name: name, Local: false, Skipped: true})
	}
	if prog != nil {
		rep.Resources.Qubits = prog.NumQubits
		rep.Resources.Depth = optimizer.DepthLayers(prog)
		for _, op := range prog.Ops {
			if op.Kind == ir.OpMEASURE || op.Kind == ir.OpBARRIER {
				continue
			}
			rep.Resources.Gates++
			if len(op.Qubits) >= 2 {
				rep.Resources.TwoQubit++
			}
			if op.Kind == ir.OpSWAP {
				rep.Resources.Swaps++
			}
		}
		if prog.NumQubits > 0 && prog.NumQubits <= 20 {
			rep.Resources.SimMemory = (1 << prog.NumQubits) * 16
		}
	}
	return rep
}

// FillRemote records a provider-side duration. It does not change local stages.
func (r *Report) FillRemote(name string, d time.Duration) {
	for i := range r.Stages {
		if r.Stages[i].Name == name {
			r.Stages[i].Duration = d
			r.Stages[i].Skipped = false
			r.Stages[i].Local = false
		}
	}
}

func timeStage(name string, local bool, fn func()) Stage {
	start := time.Now()
	fn()
	return Stage{Name: name, Local: local, Duration: time.Since(start)}
}

// Text prints one stage per line. Skipped remote stages say "not run".
// They are not filled with a placeholder duration.
func (r Report) Text() string {
	var b strings.Builder
	for _, s := range r.Stages {
		label := stageLabel(s.Name)
		if s.Skipped {
			fmt.Fprintf(&b, "%-16s %s\n", label, "not run")
			continue
		}
		ms := s.Duration.Milliseconds()
		if ms == 0 && s.Duration > 0 {
			fmt.Fprintf(&b, "%-16s %s\n", label, "<1 ms")
			continue
		}
		fmt.Fprintf(&b, "%-16s %d ms\n", label, ms)
	}
	return b.String()
}

func stageLabel(name string) string {
	switch name {
	case "schedule":
		return "Schedule"
	case "queue":
		return "Queue"
	case "provider":
		return "QPU runtime"
	case "result":
		return "Retrieve"
	case "parse":
		return "Parse"
	case "check":
		return "Check"
	case "lower":
		return "Lower"
	case "optimize":
		return "Optimize"
	case "bind":
		return "Bind"
	case "compile":
		return "Compile"
	case "simulate":
		return "Simulate"
	default:
		return name
	}
}
