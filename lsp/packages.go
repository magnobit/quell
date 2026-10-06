// Copyright 2026 Magnobit, Inc. All rights reserved.

package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/pkgmgr"
)

type pkgSym struct {
	Name   string
	Kind   int
	Detail string
	URI    string
	Line   int
}

func importedSymbols(docURI, text string) []pkgSym {
	var out []pkgSym
	seenPath := map[string]bool{}
	for _, target := range importTargets(docURI, text) {
		if seenPath[target] {
			continue
		}
		seenPath[target] = true
		body, err := os.ReadFile(target)
		if err != nil {
			continue
		}
		uri := pathToURI(target)
		src := string(body)
		for _, m := range macrosIn(src) {
			out = append(out, pkgSym{Name: m.Name, Kind: 3, Detail: "imported macro", URI: uri, Line: m.Line})
		}
		for _, fn := range functionsIn(src) {
			out = append(out, pkgSym{Name: fn.Name, Kind: 3, Detail: "imported host function", URI: uri, Line: fn.Line})
		}
		for _, d := range qubitDeclsIn(src) {
			out = append(out, pkgSym{Name: d.Name, Kind: 6, Detail: "imported qubit", URI: uri, Line: d.Line})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func importedSymbol(docURI, text, word string) (pkgSym, bool) {
	upper := strings.ToUpper(word)
	for _, sym := range importedSymbols(docURI, text) {
		if sym.Name == word || sym.Name == upper {
			return sym, true
		}
	}
	return pkgSym{}, false
}

func importTargets(docURI, text string) []string {
	docPath, err := uriToPath(docURI)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(docPath)
	root := parser.FindProjectRoot(dir)
	var targets []string
	for _, line := range docLines(text) {
		m := parser.ImportLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		target, err := parser.ResolveImportPath(m[1], dir, root)
		if err != nil {
			continue
		}
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func packageDiagnostics(docURI, text string) []diagnostic {
	docPath, err := uriToPath(docURI)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(docPath)
	root := parser.FindProjectRoot(dir)
	lock, _ := pkgmgr.LoadLock(root)
	strict := os.Getenv("QUELL_PKG_STRICT") == "1"
	var diags []diagnostic
	if strict {
		if _, err := os.Stat(filepath.Join(root, pkgmgr.LockFile)); err != nil {
			diags = append(diags, diagnostic{
				Range: lineRange(1), Severity: 1, Source: "quell",
				Message: "strict package mode requires " + pkgmgr.LockFile,
			})
		}
	}
	if lock != nil {
		bySource := map[string]map[string]bool{}
		for _, e := range lock.Require {
			if bySource[e.Source] == nil {
				bySource[e.Source] = map[string]bool{}
			}
			if e.Version != "" {
				bySource[e.Source][e.Version] = true
			}
		}
		for src, vers := range bySource {
			if len(vers) > 1 {
				diags = append(diags, diagnostic{
					Range: lineRange(1), Severity: 1, Source: "quell",
					Message: "version conflict for " + src,
				})
			}
		}
	}
	lines := docLines(text)
	for i, line := range lines {
		m := parser.ImportLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		target, err := parser.ResolveImportPath(m[1], dir, root)
		if err != nil {
			diags = append(diags, diagnostic{
				Range: lineRange(i + 1), Severity: 1, Source: "quell",
				Message: "unresolved dependency " + m[1],
			})
			continue
		}
		body, err := os.ReadFile(target)
		if err != nil {
			diags = append(diags, diagnostic{
				Range: lineRange(i + 1), Severity: 1, Source: "quell",
				Message: "unresolved dependency " + m[1],
			})
			continue
		}
		if lock == nil {
			continue
		}
		for _, e := range lock.Require {
			if e.SHA256 == "" || !lockMatchesSpec(e, m[1]) {
				continue
			}
			if pkgmgr.Sum(body) != e.SHA256 {
				diags = append(diags, diagnostic{
					Range: lineRange(i + 1), Severity: 1, Source: "quell",
					Message: "checksum mismatch for " + m[1],
				})
			}
		}
	}
	return diags
}

func lockMatchesSpec(e pkgmgr.LockEntry, spec string) bool {
	return e.Source == spec || strings.HasSuffix(e.Source, spec) || e.Resolved == spec
}
