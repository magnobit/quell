// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/magnobit/quell/pkgmgr"
	"github.com/spf13/cobra"
)

// registryBase is kept as a thin alias so existing call sites in this file
// don't all need touching — the actual env-var lookup lives in pkgmgr now,
// shared with GetOne's registry fetch path, instead of two copies that
// could disagree about the default URL or the QUELL_REGISTRY_URL name.
func registryBase() string { return pkgmgr.RegistryBase() }

func newPkgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pkg",
		Short: "Manage Quell packages (quell.pkg.yml)",
		Long: `Manage Quell packages (quell.pkg.yml)

A package can be a git repository (source: github.com/…), fetched into
.quell/pkg/<source>/, or a hosted registry package (quell pkg install
name@version), fetched into .quell/pkg/registry/<name>/<version>/ and
recorded in quell.pkg.yml so a fresh 'quell pkg get' reproduces it too.`,
	}
	cmd.AddCommand(newPkgAddCmd())
	cmd.AddCommand(newPkgGetCmd())
	cmd.AddCommand(newPkgListCmd())
	cmd.AddCommand(newPkgInstallCmd())
	cmd.AddCommand(newPkgPublishCmd())
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

func newPkgInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "install <name>@<version>",
		Short:   "Install a package from the hosted Quell registry, and record it in quell.pkg.yml",
		Example: "  quell pkg install grover@1.0.0\n  QUELL_REGISTRY_URL=http://localhost:8085/api/v1 quell pkg install chemistry@0.1.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := args[0]
			name, ver, ok := strings.Cut(spec, "@")
			if !ok || name == "" || ver == "" {
				return fmt.Errorf("expected name@version, got %q", spec)
			}
			root := pkgRoot()
			// AddRequirement both fetches it now and records "registry/<name>"
			// @ ver in quell.pkg.yml — so a fresh `quell pkg get` on another
			// machine (or after a clean .quell/pkg/) reproduces it, instead of
			// this install being a one-off that only this machine remembers.
			if _, err := pkgmgr.AddRequirement(root, "registry/"+name, ver); err != nil {
				return fmt.Errorf("install %s@%s: %w", name, ver, err)
			}
			dest := filepath.Join(root, ".quell", "pkg", "registry", name, ver)
			fmt.Printf("Installed registry/%s@%s → %s\n", name, ver, dest)
			return nil
		},
	}
}

func newPkgPublishCmd() *cobra.Command {
	var version, desc, token string
	cmd := &cobra.Command{
		Use:     "publish <name> <dir>",
		Short:   "Publish a directory of .quell files to the hosted registry",
		Example: "  quell pkg publish grover ./examples/grover --version 1.0.0\n  QUELL_AUTH_TOKEN=… quell pkg publish chemistry ./lib --version 0.1.0",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, dir := args[0], args[1]
			if version == "" {
				version = "0.1.0"
			}
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				if !strings.HasSuffix(path, ".quell") {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				hdr := &tar.Header{Name: filepath.Base(path), Mode: 0644, Size: int64(len(data))}
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
				_, err = tw.Write(data)
				return err
			})
			if err != nil {
				return err
			}
			tw.Close()
			gz.Close()

			var b bytes.Buffer
			w := multipart.NewWriter(&b)
			_ = w.WriteField("name", name)
			_ = w.WriteField("version", version)
			_ = w.WriteField("description", desc)
			part, err := w.CreateFormFile("file", name+".tar.gz")
			if err != nil {
				return err
			}
			if _, err := part.Write(buf.Bytes()); err != nil {
				return err
			}
			w.Close()

			req, err := http.NewRequest(http.MethodPost, registryBase()+"/packages", &b)
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", w.FormDataContentType())
			if token == "" {
				token = os.Getenv("QUELL_AUTH_TOKEN")
			}
			if token != "" {
				req.Header.Set("X-Auth-Token", token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("publish failed: %s — %s", resp.Status, strings.TrimSpace(string(body)))
			}
			fmt.Printf("Published %s@%s\n", name, version)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "0.1.0", "package version")
	cmd.Flags().StringVar(&desc, "description", "", "short description")
	cmd.Flags().StringVar(&token, "token", "", "Labs auth token (or QUELL_AUTH_TOKEN)")
	return cmd
}
