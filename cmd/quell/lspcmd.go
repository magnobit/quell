// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"os"

	"github.com/magnobit/quell/lsp"
	"github.com/spf13/cobra"
)

func newLSPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lsp",
		Short: "Start the Quell language server (speaks LSP over stdio)",
		Long: `Start the Quell language server (speaks LSP over stdio)

Point your editor's LSP client at "quell lsp" for a .quell file — it
provides diagnostics (parse errors and semantic warnings, as red
squiggles) and format-on-save (the same formatting as "quell fmt").`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return lsp.Run(os.Stdin, os.Stdout)
		},
	}
}
