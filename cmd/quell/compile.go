// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/parser"
	"github.com/spf13/cobra"
)

func newCompileCmd() *cobra.Command {
	var target, outFile string
	var optimize, noOptimize bool

	cmd := &cobra.Command{
		Use:   "compile <file.quell>",
		Short: "Compile to OpenQASM, Qiskit, Cirq, or Braket",
		Example: `  quell compile bell.quell
  quell compile --target qiskit bell.quell
  quell compile --target cirq --no-optimize -o out.py bell.quell`,
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

			finalOptimize := optimize
			if noOptimize {
				finalOptimize = false
			}

			out, notes, err := compiler.Compile(circ, compiler.Target(target), finalOptimize)
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
				return nil
			}
			fmt.Println(out)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&target, "target", string(compiler.TargetOpenQASM), "openqasm|qiskit|cirq|braket")
	f.StringVarP(&outFile, "output", "o", "", "write compiled output to file instead of stdout")
	f.BoolVar(&optimize, "optimize", true, "enable the IR optimizer (default)")
	f.BoolVar(&noOptimize, "no-optimize", false, "disable the IR optimizer")

	return cmd
}
