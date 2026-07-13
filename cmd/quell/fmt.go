// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"

	"github.com/magnobit/quell/format"
	"github.com/spf13/cobra"
)

func newFmtCmd() *cobra.Command {
	var write, check bool

	cmd := &cobra.Command{
		Use:   "fmt <file.quell>",
		Short: "Format a Quell source file",
		Example: `  quell fmt bell.quell            # print formatted source to stdout
  quell fmt --write bell.quell    # reformat the file in place
  quell fmt --check bell.quell    # exit 1 if the file isn't already formatted`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("cannot read file: %w", err)
			}
			formatted := format.Format(string(data))

			if check {
				if formatted != string(data) {
					fmt.Println(path)
					os.Exit(1)
				}
				return nil
			}

			if write {
				if formatted == string(data) {
					return nil
				}
				return os.WriteFile(path, []byte(formatted), 0644)
			}

			fmt.Print(formatted)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&write, "write", "w", false, "write result back to the file instead of stdout")
	cmd.Flags().BoolVar(&check, "check", false, "exit 1 and print the path if the file isn't already formatted (no output written)")
	return cmd
}
