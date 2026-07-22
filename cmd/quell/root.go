// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/log"
	"github.com/spf13/cobra"
)

const version = "0.0.9"

func newRootCmd() *cobra.Command {
	var logLevel string
	root := &cobra.Command{
		Use:          "quell",
		Short:        "Quell — backend-agnostic quantum circuit language",
		Long:         "Quell is an open-source, backend-agnostic quantum circuit language.\nWrite once, run on IBM Quantum, AWS Braket, Google Quantum Engine, IonQ, Rigetti, or Azure Quantum.",
		Version:      version,
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if logLevel != "" {
				log.SetLevel(logLevel)
			}
		},
	}
	root.SetVersionTemplate("quell {{.Version}}\n")
	root.PersistentFlags().StringVar(&logLevel, "log-level", os.Getenv("QUELL_LOG_LEVEL"), "log level: debug|info|warn|error (env QUELL_LOG_LEVEL)")

	root.AddCommand(newRunCmd())
	root.AddCommand(newAnnealCmd())
	root.AddCommand(newSimulateCmd())
	root.AddCommand(newCompileCmd())
	root.AddCommand(newFmtCmd())
	root.AddCommand(newLSPCmd())
	root.AddCommand(newPkgCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newAskCmd())
	root.AddCommand(newConvertCmd())
	root.AddCommand(newEstimateCmd())
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the quell version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("quell " + version)
		},
	})

	return root
}

// loadConfigFrom loads quell.config.yml/.yaml, or the file at path if given,
// falling back to config.Default() when nothing is found. It's just the
// base layer — callers then apply CLI-flag and --set overrides on top.
func loadConfigFrom(path string) *config.Config {
	paths := []string{"quell.config.yml", "quell.config.yaml"}
	if path != "" {
		paths = []string{path}
	}
	for _, p := range paths {
		cfg, err := config.Load(p)
		if err == nil {
			return cfg
		}
	}
	return config.Default()
}

func readFile(path string) string {
	if !strings.HasSuffix(path, ".quell") && !strings.HasSuffix(path, ".py") {
		fatalf("expected .quell or .py file, got: %s", filepath.Ext(path))
	}
	data, err := os.ReadFile(path)
	must(err, "cannot read file")
	return string(data)
}

func must(err error, msg string) {
	if err != nil {
		fatalf("%s: %v", msg, err)
	}
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Error(msg)
	fmt.Fprintf(os.Stderr, "quell: %s\n", msg)
	os.Exit(1)
}
