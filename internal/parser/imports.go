// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ParseFile parses the .quell file at path, first resolving any "import"
// lines relative to path's directory (recursively), splicing each
// imported file's source in place. This is preprocessor-#include
// semantics, not a module/namespace system: an imported file's named
// qubits share the importing file's qubit namespace (as if its text had
// been pasted in directly), because Quell has no concept of an isolated
// scope or a callable/parameterized circuit to import instead.
//
//	import "./bell_pair.quell"                       // relative to this file
//	import "github.com/someuser/quell-gates/qft.quell" // a package — see pkgmgr
//
// A path starting with "." is always a plain relative filesystem path. Any
// other path is resolved against .quell/pkg/<path> under the project root
// (the nearest ancestor directory containing quell.pkg.yml) — see the
// pkgmgr package for how packages get there.
//
// Diamond imports (the same file reachable via two different, non-cyclic
// paths) are spliced in each time they're referenced, not deduplicated —
// the simplest, most literal "paste this text here" behavior, with only
// true cycles (a file importing itself, directly or transitively) rejected.
func ParseFile(path string) (*Circuit, error) {
	root := findProjectRoot(filepath.Dir(path))
	expanded, err := resolveImports(path, root, nil, 0)
	if err != nil {
		return nil, err
	}
	return Parse(expanded)
}

const maxImportDepth = 64

var importLineRe = regexp.MustCompile(`^import\s+"([^"]+)"\s*$`)

func resolveImports(path, projectRoot string, ancestors []string, depth int) (string, error) {
	if depth > maxImportDepth {
		return "", fmt.Errorf("import depth exceeds %d — likely a runaway import chain starting at %s", maxImportDepth, path)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %s: %w", path, err)
	}
	for _, a := range ancestors {
		if a == abs {
			chain := append(append([]string{}, ancestors...), abs)
			return "", fmt.Errorf("import cycle: %s", strings.Join(chain, " -> "))
		}
	}
	nextAncestors := append(append([]string{}, ancestors...), abs)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("import %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		if m := importLineRe.FindStringSubmatch(trimmed); m != nil {
			importPath, err := resolveImportPath(m[1], dir, projectRoot)
			if err != nil {
				return "", fmt.Errorf("%s:%d: %w", path, lineNum, err)
			}
			sub, err := resolveImports(importPath, projectRoot, nextAncestors, depth+1)
			if err != nil {
				return "", fmt.Errorf("%s:%d: %w", path, lineNum, err)
			}
			out.WriteString(sub)
			out.WriteString("\n")
			continue
		}

		out.WriteString(raw)
		out.WriteString("\n")
	}
	return out.String(), nil
}

func resolveImportPath(spec, currentDir, projectRoot string) (string, error) {
	if strings.HasPrefix(spec, ".") {
		return filepath.Join(currentDir, spec), nil
	}
	if projectRoot == "" {
		return "", fmt.Errorf("cannot resolve package import %q — no quell.pkg.yml found in this or any parent directory (run `quell pkg get` from your project root first)", spec)
	}
	return filepath.Join(projectRoot, ".quell", "pkg", filepath.FromSlash(normalizePackageSpec(spec))), nil
}

// normalizePackageSpec must produce exactly what pkgmgr.destPath would
// produce for the same source string (a scheme, if any, and any ":"
// stripped) — the two packages don't share code, but they have to agree
// on where a package ends up on disk. A real source ("github.com/user/repo")
// is untouched by this; the normalization only matters for a schemed
// source (file://, ssh://) that includes a host:port or a Windows drive
// letter, which would otherwise leave an invalid path segment.
func normalizePackageSpec(spec string) string {
	clean := spec
	if i := strings.Index(clean, "://"); i >= 0 {
		clean = clean[i+3:]
	}
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.ReplaceAll(clean, ":", "")
	return clean
}

// findProjectRoot walks up from dir looking for quell.pkg.yml, returning
// the directory that contains it, or "" if none is found (package imports
// will then fail with a clear error; relative imports still work fine).
func findProjectRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "quell.pkg.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
