// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/internal/qir"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "inspect <file.quell>",
		Short: "Print the parse, canonical IR, optimized IR, or QIR subset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			buf, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			c, err := parser.Parse(string(buf))
			if err != nil {
				return err
			}
			prog := ir.Lower(c)
			opt, _ := optimizer.Optimize(prog)
			out := cmd.OutOrStdout()
			switch strings.ToLower(kind) {
			case "ir":
				_, err = fmt.Fprint(out, string(ir.CanonicalBytes(prog)))
			case "optimized":
				_, err = fmt.Fprint(out, string(ir.CanonicalBytes(opt)))
			case "qir":
				text, qerr := qir.Emit(prog)
				if qerr != nil {
					return qerr
				}
				_, err = fmt.Fprint(out, text)
			default:
				_, err = fmt.Fprintf(out, "functions %d\ngates %d\n", len(c.Functions), len(c.Instructions))
			}
			return err
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "ast", "ast, ir, optimized, or qir")
	return cmd
}
