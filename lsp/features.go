// Copyright 2026 Magnobit, Inc. All rights reserved.

package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/magnobit/quell/internal/parser"
)

var identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// docLines splits doc text into lines without the trailing newline, for
// 0-indexed LSP line access.
func docLines(text string) []string {
	return strings.Split(text, "\n")
}

// wordAt returns the identifier at pos and its column span, or ok=false if
// the cursor isn't inside/adjacent to one. Treats character as a byte
// offset into the line — an approximation of LSP's UTF-16 code-unit offset
// that's exact for the ASCII-only identifiers Quell source actually uses.
func wordAt(text string, pos position) (word string, startCol, endCol int, ok bool) {
	lines := docLines(text)
	if pos.Line < 0 || pos.Line >= len(lines) {
		return "", 0, 0, false
	}
	line := lines[pos.Line]
	for _, loc := range identRe.FindAllStringIndex(line, -1) {
		start, end := loc[0], loc[1]
		if pos.Character >= start && pos.Character <= end {
			return line[start:end], start, end, true
		}
	}
	return "", 0, 0, false
}

// macrosIn returns every gate macro declared in text. It tries a real parse
// first (authoritative — matches exactly what Parse would accept), and
// falls back to a line-scan for the common LSP case of a document that
// doesn't fully parse yet because the user is still typing.
func macrosIn(text string) []parser.Macro {
	if c, err := parser.Parse(text); err == nil {
		return c.Macros
	}
	var macros []parser.Macro
	for i, line := range docLines(text) {
		trimmed := strings.TrimSpace(line)
		if ci := strings.Index(trimmed, "//"); ci >= 0 {
			trimmed = strings.TrimSpace(trimmed[:ci])
		}
		m := gateHeaderRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		var params []string
		for _, p := range strings.Fields(strings.ReplaceAll(m[2], ",", " ")) {
			params = append(params, p)
		}
		macros = append(macros, parser.Macro{Name: strings.ToUpper(m[1]), Params: params, Line: i + 1})
	}
	return macros
}

var gateHeaderRe = regexp.MustCompile(`(?i)^gate\s+(\w+)\s*([\w\s,]*)\{`)

// findMacro looks up name (case-insensitive — Quell upper-cases macro names
// at parse time) among text's macros.
func findMacro(text, name string) (parser.Macro, bool) {
	upper := strings.ToUpper(name)
	for _, m := range macrosIn(text) {
		if m.Name == upper {
			return m, true
		}
	}
	return parser.Macro{}, false
}

// qubitDeclsIn returns every named-qubit declaration ("qubit alice, bob")
// in text, with the same real-parse-first, regex-fallback strategy as
// macrosIn — for the same reason (completion/hover need to work while the
// document doesn't fully parse yet).
func qubitDeclsIn(text string) []parser.QubitDecl {
	if c, err := parser.Parse(text); err == nil {
		return c.QubitDecls
	}
	seen := map[string]bool{}
	var decls []parser.QubitDecl
	for i, line := range docLines(text) {
		trimmed := strings.TrimSpace(line)
		if ci := strings.Index(trimmed, "//"); ci >= 0 {
			trimmed = strings.TrimSpace(trimmed[:ci])
		}
		m := qubitDeclRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		for _, name := range strings.Split(m[1], ",") {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			decls = append(decls, parser.QubitDecl{Name: name, Line: i + 1})
		}
	}
	return decls
}

var qubitDeclRe = regexp.MustCompile(`(?i)^qubit\s+(.+)$`)

// findQubitDecl looks up name (case-sensitive — unlike macros, Quell never
// upper-cases named qubits, so "alice" and "Alice" are genuinely different
// qubits) among text's qubit declarations.
func findQubitDecl(text, name string) (parser.QubitDecl, bool) {
	for _, d := range qubitDeclsIn(text) {
		if d.Name == name {
			return d, true
		}
	}
	return parser.QubitDecl{}, false
}

// ─── textDocument/hover ───────────────────────────────────────────────────

func (s *server) hover(id json.RawMessage, uri string, pos position) {
	text, ok := s.docs[uri]
	if !ok {
		s.respond(id, nil)
		return
	}
	word, _, _, ok := wordAt(text, pos)
	if !ok {
		s.respond(id, nil)
		return
	}
	upper := strings.ToUpper(word)

	if md := gateHover(upper); md != "" {
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
		return
	}
	if m, found := findMacro(text, word); found {
		md := fmt.Sprintf("**%s** *(macro)*\n\nDefined at line %d — expects %d qubit argument(s): `%s`",
			m.Name, m.Line, len(m.Params), strings.Join(m.Params, ", "))
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
		return
	}
	if d, found := findQubitDecl(text, word); found {
		md := fmt.Sprintf("**%s** *(named qubit)*\n\nDeclared at line %d.", d.Name, d.Line)
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
		return
	}
	s.respond(id, nil)
}

// ─── textDocument/completion ──────────────────────────────────────────────

func (s *server) completion(id json.RawMessage, uri string) {
	text := s.docs[uri]

	var items []completionItem
	for _, name := range sortedGateNames() {
		item := completionItem{
			Label: name,
			Kind:  3, // Function
			Detail: func() string {
				if spec, isGate := parser.GateArity[name]; isGate {
					return arityText(spec)
				}
				return "control flow"
			}(),
			Documentation: gateDoc[name],
			InsertText:    completionSnippet(name),
		}
		if item.InsertText != name {
			item.InsertTextFormat = 2 // Snippet
		}
		items = append(items, item)
	}
	for _, m := range macrosIn(text) {
		items = append(items, completionItem{
			Label:         m.Name,
			Kind:          3, // Function
			Detail:        fmt.Sprintf("macro (line %d)", m.Line),
			Documentation: fmt.Sprintf("gate %s %s {…} — defined at line %d", m.Name, strings.Join(m.Params, " "), m.Line),
			InsertText:    m.Name,
		})
	}
	for _, d := range qubitDeclsIn(text) {
		items = append(items, completionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        fmt.Sprintf("named qubit (line %d)", d.Line),
			Documentation: fmt.Sprintf("qubit %s — declared at line %d", d.Name, d.Line),
			InsertText:    d.Name,
		})
	}
	s.respond(id, items)
}

// completionSnippet builds a tab-stop snippet ("RX ${1:theta} ${2:q0}") for
// gates that take arguments, so accepting the completion leaves the cursor
// ready to fill in qubits/angles instead of just inserting a bare gate name.
func completionSnippet(name string) string {
	spec, ok := parser.GateArity[name]
	if !ok || (spec.Qubits <= 0 && spec.Args <= 0) {
		return name
	}
	tab := 1
	var parts []string
	parts = append(parts, name)
	for i := 0; i < spec.Args; i++ {
		parts = append(parts, fmt.Sprintf("${%d:angle%d}", tab, i+1))
		tab++
	}
	n := spec.Qubits
	if n < 0 {
		n = 1
	}
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("${%d:q%d}", tab, i))
		tab++
	}
	return strings.Join(parts, " ")
}

// ─── textDocument/definition ───────────────────────────────────────────────

func (s *server) definition(id json.RawMessage, uri string, pos position) {
	text, ok := s.docs[uri]
	if !ok {
		s.respond(id, nil)
		return
	}
	lines := docLines(text)
	if pos.Line >= 0 && pos.Line < len(lines) {
		if m := parser.ImportLineRe.FindStringSubmatch(strings.TrimSpace(lines[pos.Line])); m != nil {
			if loc, ok := s.resolveImportLocation(uri, m[1]); ok {
				s.respond(id, loc)
				return
			}
			s.respond(id, nil)
			return
		}
	}

	word, _, _, ok := wordAt(text, pos)
	if !ok {
		s.respond(id, nil)
		return
	}
	if m, found := findMacro(text, word); found {
		s.respond(id, location{URI: uri, Range: lineRange(m.Line)})
		return
	}
	if d, found := findQubitDecl(text, word); found {
		s.respond(id, location{URI: uri, Range: lineRange(d.Line)})
		return
	}
	s.respond(id, nil)
}

// resolveImportLocation turns an `import "spec"` path into a file:// Location
// at line 1, using the same relative/package resolution rules the compiler
// itself uses (parser.ResolveImportPath), so "go to definition" on an
// import always agrees with what would actually get spliced in at compile time.
func (s *server) resolveImportLocation(docURI, spec string) (location, bool) {
	docPath, err := uriToPath(docURI)
	if err != nil {
		return location{}, false
	}
	dir := filepath.Dir(docPath)
	root := parser.FindProjectRoot(dir)
	target, err := parser.ResolveImportPath(spec, dir, root)
	if err != nil {
		return location{}, false
	}
	if _, err := os.Stat(target); err != nil {
		return location{}, false
	}
	return location{URI: pathToURI(target), Range: lineRange(1)}, true
}

// ─── textDocument/rename ───────────────────────────────────────────────────

// rename is scoped deliberately narrow: a gate macro name or a named qubit,
// only within the file that declares it. Quell has no scoping/namespace
// concept (see ParseFile's doc comment) — a macro or named qubit declared
// in one file is visible to anything that imports it, splicing its text in
// verbatim. Renaming only within the current file is therefore not fully
// safe either (an importing file's own reference would go stale), but it's
// the same, disclosed tradeoff already accepted for macro rename, applied
// consistently rather than treating qubits as a separate, stricter case
// with no real difference in underlying risk. What's still declined:
// renaming a bare qubit *index* (0, 1, 2 — ambiguous with a plain number)
// and a macro's internal parameter name (scoped to only within that one
// macro's body, which this server doesn't track the line range of, so a
// naive whole-file match could rename an unrelated same-letter identifier
// elsewhere in the file).
func (s *server) rename(id json.RawMessage, uri string, pos position, newName string) {
	text, ok := s.docs[uri]
	if !ok {
		s.respond(id, nil)
		return
	}
	if !identRe.MatchString(newName) || !isIdentStart(newName) {
		s.respondError(id, -32602, "invalid identifier: "+newName)
		return
	}
	word, _, _, ok := wordAt(text, pos)
	if !ok {
		s.respond(id, nil)
		return
	}

	// Macro names are case-insensitive (parseGateDef upper-cases them), so
	// `bell`, `Bell`, and `BELL` all refer to the same macro and must all be
	// renamed together. Named qubits are case-sensitive — `alice` and
	// `Alice` are genuinely different qubits — so only exact-case
	// occurrences may be touched.
	var wordBoundary *regexp.Regexp
	if _, found := findMacro(text, word); found {
		if _, clash := findMacro(text, newName); clash {
			s.respondError(id, -32602, fmt.Sprintf("a macro named %q already exists in this file — pick a different name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
	} else if _, found := findQubitDecl(text, word); found {
		if _, clash := findQubitDecl(text, newName); clash {
			s.respondError(id, -32602, fmt.Sprintf("a qubit named %q already exists in this file — pick a different name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	} else {
		s.respondError(id, -32602, "rename is only supported for gate macro names and named qubits")
		return
	}

	var edits []textEdit
	for i, line := range docLines(text) {
		// Only match within actual code — a name that happens to appear
		// inside a "//" comment (in prose, unrelated to the declaration)
		// must not be rewritten.
		code := line
		if ci := strings.Index(line, "//"); ci >= 0 {
			code = line[:ci]
		}
		for _, loc := range wordBoundary.FindAllStringIndex(code, -1) {
			edits = append(edits, textEdit{
				Range:   lspRange{Start: position{Line: i, Character: loc[0]}, End: position{Line: i, Character: loc[1]}},
				NewText: newName,
			})
		}
	}
	if len(edits) == 0 {
		s.respond(id, nil)
		return
	}
	s.respond(id, workspaceEdit{Changes: map[string][]textEdit{uri: edits}})
}

func isIdentStart(s string) bool {
	if s == "" {
		return false
	}
	r := s[0]
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

// ─── file:// URI helpers ───────────────────────────────────────────────────

func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported URI scheme %q", u.Scheme)
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p), nil
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed}
	return u.String()
}
