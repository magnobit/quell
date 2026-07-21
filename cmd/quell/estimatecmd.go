// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/magnobit/quell/estimate"
	"github.com/spf13/cobra"
)

func newEstimateCmd() *cobra.Command {
	var shots int
	var asJSON bool
	var backends []string
	var coupling string
	var noiseAware bool

	cmd := &cobra.Command{
		Use:   "estimate <file.quell>",
		Short: "Digital twin — estimate cost, fidelity, queue, and runtime across providers (no hardware)",
		Long: `Estimate circuit metrics and educational multi-provider cost/fidelity without submitting jobs.

Uses local IR stats, the conservative optimizer, and teaching noise models.
Optional --coupling heavyhex-toy|linear-5 inserts SWAPs for hardware-aware routing.
Estimates are not invoices or SLAs — see the disclaimer in the output.`,
		Example: "  quell estimate bell.quell --shots 1000\n  quell estimate grover.quell --coupling heavyhex-toy --json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			rep, err := estimate.Analyze(string(data), estimate.Options{
				Shots:      shots,
				Backends:   backends,
				Seed:       42,
				Coupling:   coupling,
				NoiseAware: noiseAware || coupling != "",
			})
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}
			printEstimate(rep)
			return nil
		},
	}

	cmd.Flags().IntVar(&shots, "shots", 1000, "shot count used for cost and runtime estimates")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	cmd.Flags().StringSliceVar(&backends, "backend", nil, "backends to estimate (default: ibm,ionq,google,rigetti)")
	cmd.Flags().StringVar(&coupling, "coupling", "", "topology preset: heavyhex-toy, linear-5, linear-7")
	cmd.Flags().BoolVar(&noiseAware, "noise-aware", false, "append noise-aware score note")
	return cmd
}

func printEstimate(r *estimate.Report) {
	fmt.Printf("Digital twin (estimates only)\n")
	fmt.Printf("  Qubits      : %d\n", r.Circuit.NumQubits)
	fmt.Printf("  Gates       : %d → %d (%.1f%% fewer after optimize)\n",
		r.Optimizer.BeforeGates, r.Optimizer.AfterGates, r.Optimizer.GateReductionPct)
	fmt.Printf("  Depth       : %d → %d (%.1f%% lower)\n",
		r.Optimizer.BeforeDepth, r.Optimizer.AfterDepth, r.Optimizer.DepthReductionPct)
	fmt.Printf("  Two-qubit   : %d (optimized)\n", r.OptimizedCircuit.TwoQubitGates)
	fmt.Printf("  Shots       : %d\n", r.Shots)
	if len(r.Optimizer.Notes) > 0 {
		fmt.Println("  Optimizer:")
		for _, n := range r.Optimizer.Notes {
			fmt.Printf("    - %s\n", n)
		}
	}
	fmt.Println()
	fmt.Printf("%-10s %10s %10s %10s %10s\n", "Backend", "Cost $", "Fidelity", "Queue s", "Runtime s")
	for _, p := range r.Providers {
		fmt.Printf("%-10s %10.4f %9.1f%% %10.0f %10.2f\n",
			p.Backend, p.EstimatedCostUSD, p.EstimatedFidelity*100, p.EstimatedQueueSec, p.EstimatedRuntimeSec)
	}
	fmt.Println()
	fmt.Printf("Recommended : %s (balanced)\n", r.Recommendation.Balanced)
	fmt.Printf("  cheapest  : %s\n", r.Recommendation.Cheapest)
	fmt.Printf("  fidelity  : %s\n", r.Recommendation.BestFidelity)
	fmt.Printf("  %s\n", r.Recommendation.Reason)
	fmt.Println()
	fmt.Println(r.Disclaimer)
}
