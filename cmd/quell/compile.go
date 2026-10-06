// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"
	"strings"

	"encoding/json"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/parser"
	"github.com/spf13/cobra"
)

func newCompileCmd() *cobra.Command {
	var target, outFile string
	var optimize, noOptimize, verifyOptimization bool
	var paramFlags, paramSetFlags []string

	cmd := &cobra.Command{
		Use:   "compile <file.quell>",
		Short: "Compile to OpenQASM, Qiskit, Cirq, Braket, or Q#",
		Example: `  quell compile bell.quell
  quell compile --target qiskit bell.quell
  quell compile --target all bell.quell
  quell compile --target qiskit,cirq bell.quell
  quell compile --target qsharp bell.quell
  quell compile --target cirq --no-optimize -o out.py bell.quell
  quell compile --target qiskit --param-set theta=0.5 --param-set theta=1.0 sweep.quell
  quell compile --verify-optimization bell.quell`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !strings.HasSuffix(args[0], ".quell") {
				return fmt.Errorf("expected a .quell file, got: %s", args[0])
			}
			// ParseFile (not Parse) so "import" lines resolve relative to
			// this file's directory, or against an installed package.
			circ, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}
			if err := check.Fail(circ); err != nil {
				return err
			}

			params, err := parseParamFlags(paramFlags)
			if err != nil {
				return err
			}
			sets, err := parseParamSets(paramSetFlags)
			if err != nil {
				return err
			}
			if len(sets) > 0 && len(params) > 0 {
				return fmt.Errorf("use either --param or --param-set, not both")
			}
			prog := ir.Lower(circ)
			programs := []*ir.Program{prog}
			if len(sets) > 0 {
				programs = make([]*ir.Program, len(sets))
				for i, set := range sets {
					bound, berr := ir.Bind(prog, set)
					if berr != nil {
						return fmt.Errorf("compile error: %w", berr)
					}
					programs[i] = bound
				}
			} else if len(params) > 0 || ir.NeedsBind(prog) {
				bound, berr := ir.Bind(prog, params)
				if berr != nil && verifyOptimization {
					ev := optequiv.VerifyOptimization(prog, optequiv.Options{Params: params, OptimizerVersion: compile.BuildIdentity().PinnedOptimizer()})
					writeEquiv(ev)
					return fmt.Errorf("optimizer verification: %s", ev.Status)
				}
				if berr != nil {
					return fmt.Errorf("compile error: %w", berr)
				}
				programs[0] = bound
			}

			finalOptimize := optimize
			if noOptimize {
				finalOptimize = false
			}

			useOptimized := finalOptimize
			var ev optequiv.Evidence
			var verified bool
			if verifyOptimization && finalOptimize {
				ev = optequiv.VerifyOptimization(prog, optequiv.Options{
					Params:           params,
					OptimizerVersion: compile.BuildIdentity().PinnedOptimizer(),
				})
				verified = true
				writeEquiv(ev)
				switch ev.Status {
				case optequiv.StatusNotEquivalent:
					useOptimized = false
					fmt.Fprintln(os.Stderr, "Optimizer verification failed — compiling the original unoptimized IR.")
				case optequiv.StatusUnsupported, optequiv.StatusInconclusive:
					fmt.Fprintf(os.Stderr, "Optimizer verification %s — compiling optimized IR without claiming equivalence.\n", ev.Status)
				}
			}

			targets, err := compile.ResolveTargets(target)
			if err != nil {
				return err
			}
			multi := len(programs) > 1 || len(targets) > 1
			if multi && outFile != "" {
				return fmt.Errorf("--output writes one file; omit it when compiling several targets or parameter sets")
			}
			notesPrinted := false
			for i, program := range programs {
				emitted, err := compile.Emit(program, targets, useOptimized)
				if err != nil {
					return fmt.Errorf("compile error: %w", err)
				}
				if !notesPrinted {
					for _, n := range emitted[targets[0]].OptimizerNotes {
						fmt.Printf("Optimizer: %s\n", n)
					}
					notesPrinted = true
				}
				for _, one := range targets {
					body := emitted[one].Code
					if !multi && outFile != "" {
						if err := os.WriteFile(outFile, []byte(body), 0644); err != nil {
							return fmt.Errorf("write error: %w", err)
						}
						fmt.Printf("Written to %s\n", outFile)
						continue
					}
					if multi {
						label := string(one)
						if len(programs) > 1 {
							label = fmt.Sprintf("set %d %s", i+1, one)
						}
						fmt.Printf("===== %s =====\n", label)
					}
					fmt.Println(body)
				}
			}
			if verified && ev.Status == optequiv.StatusNotEquivalent {
				return fmt.Errorf("optimizer verification: NOT_EQUIVALENT")
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&target, "target", string(compiler.TargetOpenQASM), "one target, a comma-separated list, or all")
	f.StringVarP(&outFile, "output", "o", "", "write compiled output to file instead of stdout")
	f.BoolVar(&optimize, "optimize", true, "enable the IR optimizer (default)")
	f.BoolVar(&noOptimize, "no-optimize", false, "disable the IR optimizer")
	f.BoolVar(&verifyOptimization, "verify-optimization", false, "compare original vs optimized IR (exact when possible); fall back to unoptimized IR on NOT_EQUIVALENT")
	f.StringArrayVar(&paramFlags, "param", nil, "bind symbolic angle: --param theta=1.5708 (repeatable)")
	f.StringArrayVar(&paramSetFlags, "param-set", nil, "one sweep point, comma-separated angles: --param-set theta=0.5 (repeatable)")

	return cmd
}

func parseParamSets(flags []string) ([]map[string]float64, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out := make([]map[string]float64, len(flags))
	for i, spec := range flags {
		parts := strings.Split(spec, ",")
		set, err := parseParamFlags(parts)
		if err != nil {
			return nil, err
		}
		out[i] = set
	}
	return out, nil
}

func writeEquiv(ev optequiv.Evidence) {
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "optimizer verification: %s (%s)\n", ev.Status, ev.Reason)
		return
	}
	fmt.Fprintln(os.Stderr, string(b))
}
