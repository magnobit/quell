// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"

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

	cmd := &cobra.Command{
		Use:     "simulate <file.quell>",
		Short:   "Simulate a circuit locally (no backend, no credentials, no network)",
		Example: "  quell simulate bell.quell --shots 2000",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			circ, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			fmt.Printf("Qubits  : %d\n", circ.NumQubits)
			fmt.Printf("Gates   : %d\n", len(circ.Instructions))

			result, err := simulate.RunFile(args[0], shots)
			if err != nil {
				return fmt.Errorf("simulate error: %w", err)
			}
			result.Print()
			return nil
		},
	}

	cmd.Flags().IntVar(&shots, "shots", 1000, "number of measurement samples")
	return cmd
}
