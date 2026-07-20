// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"

	"github.com/magnobit/quell/anneal"
	"github.com/magnobit/quell/internal/backends"
	"github.com/spf13/cobra"
)

func newAnnealCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "anneal",
		Short: "QUBO / annealer tools (D-Wave Leap or local SA)",
	}

	var configPath string
	var shots int
	var localOnly bool

	run := &cobra.Command{
		Use:   "run <file.qubo>",
		Short: "Sample a QUBO problem",
		Long:  "Parse a QUBO text file and sample it. With DWAVE_API_TOKEN + Ocean SDK → Leap; otherwise local simulated annealing.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			p, err := anneal.ParseQUBO(string(data))
			if err != nil {
				return err
			}
			cfg := loadConfigFrom(configPath)
			if shots > 0 {
				cfg.DWave.Shots = shots
			}
			if localOnly {
				_ = os.Unsetenv("DWAVE_API_TOKEN")
				cfg.DWave.APIToken = ""
			}
			fmt.Printf("  QUBO n=%d — submitting…\n", p.NumVars)
			result, err := backends.RunDWaveQUBO(&cfg.DWave, p)
			if err != nil {
				return err
			}
			result.Print()
			return nil
		},
	}
	run.Flags().StringVar(&configPath, "config", "", "quell.config.yml path")
	run.Flags().IntVar(&shots, "shots", 0, "number of reads/samples")
	run.Flags().BoolVar(&localOnly, "local", false, "force local simulated annealing")
	root.AddCommand(run)
	return root
}
