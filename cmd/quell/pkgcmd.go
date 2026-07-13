// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"

	"github.com/magnobit/quell/pkgmgr"
	"github.com/spf13/cobra"
)

func newPkgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pkg",
		Short: "Manage Quell packages (quell.pkg.yml)",
		Long: `Manage Quell packages (quell.pkg.yml)

A package is a git repository containing .quell files. There's no hosted
registry — "source" is a git-clonable host+path, e.g.
github.com/someuser/quell-gates. Installed packages live in
.quell/pkg/<source>/ under your project root (the directory containing
quell.pkg.yml), where "import" statements can reference them.`,
	}
	cmd.AddCommand(newPkgAddCmd())
	cmd.AddCommand(newPkgGetCmd())
	cmd.AddCommand(newPkgListCmd())
	return cmd
}

func newPkgAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <source> [version]",
		Short: "Add a package to quell.pkg.yml and fetch it",
		Example: `  quell pkg add github.com/someuser/quell-gates
  quell pkg add github.com/someuser/quell-gates v1.2.0`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := pkgRoot()
			version := ""
			if len(args) == 2 {
				version = args[1]
			}
			m, err := pkgmgr.AddRequirement(root, args[0], version)
			if err != nil {
				return fmt.Errorf("add %s: %w", args[0], err)
			}
			fmt.Printf("Added %s to %s\n", args[0], pkgmgr.ManifestFile)
			return pkgmgr.Get(root, m)
		},
	}
}

func newPkgGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Fetch every package listed in quell.pkg.yml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := pkgRoot()
			m, err := pkgmgr.LoadManifest(root)
			if err != nil {
				return err
			}
			if len(m.Require) == 0 {
				fmt.Println("no packages required — nothing to do (see `quell pkg add`)")
				return nil
			}
			if err := pkgmgr.Get(root, m); err != nil {
				return err
			}
			fmt.Printf("Fetched %d package(s)\n", len(m.Require))
			return nil
		},
	}
}

func newPkgListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sources, err := pkgmgr.List(pkgRoot())
			if err != nil {
				return err
			}
			if len(sources) == 0 {
				fmt.Println("no packages installed (see `quell pkg get`)")
				return nil
			}
			for _, s := range sources {
				fmt.Println(s)
			}
			return nil
		},
	}
}

// pkgRoot is the project root for package commands: the nearest ancestor
// (starting from the working directory) containing quell.pkg.yml, or the
// working directory itself if none is found yet (e.g. the first `quell pkg add`
// in a brand new project, before quell.pkg.yml exists).
func pkgRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		fatalf("cannot determine working directory: %v", err)
	}
	if root := pkgmgr.FindProjectRoot(wd); root != "" {
		return root
	}
	return wd
}
