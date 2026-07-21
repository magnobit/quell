// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
	"github.com/spf13/cobra"
)

// newSimulateCmd is an explicit, discoverable entry point for local
// simulation — equivalent to `quell run --backend local`, which does the
// same thing, but named for anyone specifically looking for a "just run it
// locally, no config file needed" command rather than the backend-selection
// mental model `run` uses.
func newSimulateCmd() *cobra.Command {
	var shots int
	var noiseFlags []string

	cmd := &cobra.Command{
		Use:     "simulate <file.quell>",
		Short:   "Simulate a circuit locally (no backend, no credentials, no network)",
		Example: "  quell simulate bell.quell --shots 2000\n  quell simulate bell.quell --noise depolarizing=0.01",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			circ, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			fmt.Printf("Qubits  : %d\n", circ.NumQubits)
			fmt.Printf("Gates   : %d\n", len(circ.Instructions))

			noise, err := mergeNoiseFlags(noiseFlags)
			if err != nil {
				return err
			}
			if noise.Active() {
				fmt.Printf("Noise   : depolarizing=%g amplitude_damping=%g\n", noise.Depolarizing, noise.AmplitudeDamping)
			}

			prog := ir.Lower(circ)
			result, err := simulate.RunProgramOpts(prog, simulate.Options{Shots: shots, Noise: noise})
			if err != nil {
				return fmt.Errorf("simulate error: %w", err)
			}
			result.Print()
			return nil
		},
	}

	cmd.Flags().IntVar(&shots, "shots", 1000, "number of measurement samples")
	cmd.Flags().StringArrayVar(&noiseFlags, "noise", nil, "noise model: depolarizing=0.01 or amplitude_damping=0.05 (repeatable)")
	return cmd
}

func mergeNoiseFlags(flags []string) (simulate.NoiseModel, error) {
	var n simulate.NoiseModel
	for _, f := range flags {
		part, err := simulate.ParseNoiseFlag(f)
		if err != nil {
			return n, err
		}
		n = simulate.MergeNoise(n, part)
	}
	return n, n.Validate()
}
