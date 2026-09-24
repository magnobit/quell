// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"
	"strings"

	"encoding/json"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/parser"
	"github.com/spf13/cobra"
)

func newCompileCmd() *cobra.Command {
	var target, outFile string
	var optimize, noOptimize, verifyOptimization bool
	var paramFlags []string

	cmd := &cobra.Command{
		Use:   "compile <file.quell>",
		Short: "Compile to OpenQASM, Qiskit, Cirq, Braket, or Q#",
		Example: `  quell compile bell.quell
  quell compile --target qiskit bell.quell
  quell compile --target qsharp bell.quell
  quell compile --target cirq --no-optimize -o out.py bell.quell
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

			params, err := parseParamFlags(paramFlags)
			if err != nil {
				return err
			}
			prog := ir.Lower(circ)
			if len(params) > 0 || ir.NeedsBind(prog) {
				bound, berr := ir.Bind(prog, params)
				if berr != nil && verifyOptimization {
					ev := optequiv.VerifyOptimization(prog, optequiv.Options{Params: params, OptimizerVersion: compile.BuildIdentity().PinnedOptimizer()})
					writeEquiv(ev)
					return fmt.Errorf("optimizer verification: %s", ev.Status)
				}
				if berr != nil {
					return fmt.Errorf("compile error: %w", berr)
				}
				prog = bound
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

			out, notes, err := compiler.CompileProgram(prog, compiler.Target(target), useOptimized)
			if err != nil {
				return fmt.Errorf("compile error: %w", err)
			}

			for _, n := range notes {
				fmt.Printf("Optimizer: %s\n", n)
			}

			if outFile != "" {
				if err := os.WriteFile(outFile, []byte(out), 0644); err != nil {
					return fmt.Errorf("write error: %w", err)
				}
				fmt.Printf("Written to %s\n", outFile)
			} else {
				fmt.Println(out)
			}
			if verified && ev.Status == optequiv.StatusNotEquivalent {
				return fmt.Errorf("optimizer verification: NOT_EQUIVALENT")
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&target, "target", string(compiler.TargetOpenQASM), "openqasm|openqasm2|qiskit|cirq|braket|qsharp")
	f.StringVarP(&outFile, "output", "o", "", "write compiled output to file instead of stdout")
	f.BoolVar(&optimize, "optimize", true, "enable the IR optimizer (default)")
	f.BoolVar(&noOptimize, "no-optimize", false, "disable the IR optimizer")
	f.BoolVar(&verifyOptimization, "verify-optimization", false, "compare original vs optimized IR (exact when possible); fall back to unoptimized IR on NOT_EQUIVALENT")
	f.StringArrayVar(&paramFlags, "param", nil, "bind symbolic angle: --param theta=1.5708 (repeatable)")

	return cmd
}

func writeEquiv(ev optequiv.Evidence) {
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "optimizer verification: %s (%s)\n", ev.Status, ev.Reason)
		return
	}
	fmt.Fprintln(os.Stderr, string(b))
}
