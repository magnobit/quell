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

	"github.com/magnobit/quell/internal/check"
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

func localsIn(text string) []parser.HostDecl {
	c, err := parser.Parse(text)
	if err != nil {
		return nil
	}
	var out []parser.HostDecl
	for _, h := range c.Host {
		if h.Kind == "let" {
			out = append(out, h)
		}
	}
	return out
}

func functionsIn(text string) []parser.FnDecl {
	c, err := parser.Parse(text)
	if err != nil {
		return nil
	}
	return c.Functions
}

func findFunction(text, name string) (parser.FnDecl, bool) {
	for _, fn := range functionsIn(text) {
		if fn.Name == name {
			return fn, true
		}
	}
	return parser.FnDecl{}, false
}

func functionAt(text string, line1 int) (parser.FnDecl, bool) {
	for _, fn := range functionsIn(text) {
		if line1 >= fn.Line && (fn.EndLine == 0 || line1 <= fn.EndLine) {
			return fn, true
		}
	}
	return parser.FnDecl{}, false
}

func fnSignature(fn parser.FnDecl) string {
	parts := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		parts[i] = p.Name + ": " + p.Type
	}
	return fmt.Sprintf("fn %s(%s) -> %s", fn.Name, strings.Join(parts, ", "), fn.Ret)
}

func findLocal(text, name string) (parser.HostDecl, bool) {
	for _, h := range localsIn(text) {
		if h.Name == name {
			return h, true
		}
	}
	return parser.HostDecl{}, false
}

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
	if word == "if" {
		if circ, err := parser.Parse(text); err == nil {
			switch parser.HostIfKindAt(circ, pos.Line+1) {
			case "expr":
				if typ, found := check.IfTypeAt(circ, pos.Line+1); found {
					md := fmt.Sprintf("`if` → `%s`\n\nHost conditional expression. Both branches have type `%s`. Only the selected branch is evaluated. This is not QPU `IF`.", typ, typ)
					s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
					return
				}
				s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: "Host conditional expression. It yields one value. This is not QPU `IF`."}})
				return
			case "stmt":
				s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: "Host `if` statement. Only the selected branch runs. Both branches are typechecked. This is not QPU `IF` and it does not yield a value."}})
				return
			}
		}
	}
	if b, found := bindingAt(text, pos, word); found {
		md := fmt.Sprintf("**%s** : `%s`\n\n%s", b.name, b.typ, b.note)
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
		return
	}
	if fn, found := findFunction(text, word); found {
		md := fmt.Sprintf("**%s**\n\n`%s`\n\nHost function at line %d. This is not a gate macro.", fn.Name, fnSignature(fn), fn.Line)
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
		return
	}
	if h, found := findLocal(text, word); found {
		md := fmt.Sprintf("**%s** : `%s`\n\nImmutable host local, declared at line %d. This is not a PARAM.", h.Name, h.Type, h.Line)
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
		return
	}
	if parser.IsHostType(word) {
		md := fmt.Sprintf("`%s`\n\nHost scalar type for `let`. It is not a qubit type and not a PARAM type.", word)
		s.respond(id, hoverResult{Contents: markupContent{Kind: "markdown", Value: md}})
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
	for _, fn := range functionsIn(text) {
		items = append(items, completionItem{
			Label:         fn.Name,
			Kind:          3,
			Detail:        fnSignature(fn),
			Documentation: "Host function. Not a gate macro.",
			InsertText:    fn.Name,
		})
		for _, p := range fn.Params {
			items = append(items, completionItem{
				Label:         p.Name,
				Kind:          6,
				Detail:        p.Type + " parameter",
				Documentation: fnSignature(fn),
				InsertText:    p.Name,
			})
		}
		for _, st := range fn.Body {
			if st.Kind != "let" {
				continue
			}
			items = append(items, completionItem{
				Label:         st.Decl.Name,
				Kind:          6,
				Detail:        st.Decl.Type + " function local",
				Documentation: fmt.Sprintf("let %s: %s in %s", st.Decl.Name, st.Decl.Type, fn.Name),
				InsertText:    st.Decl.Name,
			})
		}
	}
	if circ, err := parser.Parse(text); err == nil {
		visitHostExprs(circ, func(e parser.Expr, _, _ int) {
			parser.WalkIf(e, func(n *parser.IfExpr) {
				addBlockLets(&items, n.Then)
				addBlockLets(&items, n.Else)
			})
		})
		for _, f := range circ.Functions {
			addStmtBranchLets(&items, f.Body)
		}
	}
	for _, name := range []string{"bool", "int", "float", "string"} {
		items = append(items, completionItem{
			Label:         name,
			Kind:          14, // Keyword
			Detail:        "host scalar type",
			Documentation: "Type of an immutable let. Not a PARAM and not a qubit.",
			InsertText:    name,
		})
	}
	for _, h := range localsIn(text) {
		items = append(items, completionItem{
			Label:         h.Name,
			Kind:          6, // Variable
			Detail:        h.Type + " local",
			Documentation: fmt.Sprintf("let %s: %s, line %d", h.Name, h.Type, h.Line),
			InsertText:    h.Name,
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
	for _, sym := range importedSymbols(uri, text) {
		items = append(items, completionItem{
			Label:         sym.Name,
			Kind:          sym.Kind,
			Detail:        sym.Detail,
			Documentation: "Imported from " + sym.URI,
			InsertText:    sym.Name,
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
	if b, found := bindingAt(text, pos, word); found {
		s.respond(id, location{URI: uri, Range: lineRange(b.line)})
		return
	}
	if fn, found := findFunction(text, word); found {
		s.respond(id, location{URI: uri, Range: lineRange(fn.Line)})
		return
	}
	if h, found := findLocal(text, word); found {
		s.respond(id, location{URI: uri, Range: lineRange(h.Line)})
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
	if sym, found := importedSymbol(uri, text, word); found {
		s.respond(id, location{URI: sym.URI, Range: lineRange(sym.Line)})
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
	if fn, found := findFunction(text, word); found {
		if parser.IsReservedHostName(newName) {
			s.respondError(id, -32602, fmt.Sprintf("%q is reserved and cannot be a function name", newName))
			return
		}
		if _, clash := findFunction(text, newName); clash && newName != fn.Name {
			s.respondError(id, -32602, fmt.Sprintf("a function named %q already exists in this file — pick a different name", newName))
			return
		}
		if _, clash := findMacro(text, newName); clash {
			s.respondError(id, -32602, fmt.Sprintf("a gate macro named %q already exists in this file — pick a different name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	} else if b, found := bindingAt(text, pos, word); found && b.fnLocal {
		if parser.IsReservedHostName(newName) {
			s.respondError(id, -32602, fmt.Sprintf("%q is reserved and cannot be a local name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		if b.blockEnd > 0 {
			s.renameInRange(id, uri, text, wordBoundary, newName, b.blockLine, b.blockEnd)
			return
		}
		s.renameInRange(id, uri, text, wordBoundary, newName, b.fnLine, b.fnEnd)
		return
	} else if _, found := findMacro(text, word); found {
		if _, clash := findMacro(text, newName); clash {
			s.respondError(id, -32602, fmt.Sprintf("a macro named %q already exists in this file — pick a different name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
	} else if h, found := findLocal(text, word); found {
		if parser.IsReservedHostName(newName) {
			s.respondError(id, -32602, fmt.Sprintf("%q is reserved and cannot be a local name", newName))
			return
		}
		if _, clash := findLocal(text, newName); clash && newName != h.Name {
			s.respondError(id, -32602, fmt.Sprintf("a local named %q already exists in this file — pick a different name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	} else if _, found := findQubitDecl(text, word); found {
		if _, clash := findQubitDecl(text, newName); clash {
			s.respondError(id, -32602, fmt.Sprintf("a qubit named %q already exists in this file — pick a different name", newName))
			return
		}
		wordBoundary = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	} else {
		s.respondError(id, -32602, "rename is only supported for gate macro names, host functions, host locals, and named qubits")
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

type hostBind struct {
	name, typ, note string
	line            int
	fnLocal         bool
	fnLine, fnEnd   int
	blockLine       int
	blockEnd        int
}

func innermostBlockBind(text string, pos position, word string) (hostBind, bool) {
	circ, err := parser.Parse(text)
	if err != nil {
		return hostBind{}, false
	}
	line := pos.Line + 1
	best := hostBind{}
	bestSpan := int(^uint(0) >> 1)
	found := false
	consider := func(b *parser.BlockExpr, fnLine, fnEnd int) {
		if b == nil || line < b.Line || (b.EndLine > 0 && line > b.EndLine) {
			return
		}
		for _, d := range b.Lets {
			if d.Name != word || line < d.Line {
				continue
			}
			span := b.EndLine - b.Line
			if b.EndLine == 0 {
				span = bestSpan
			}
			if !found || span < bestSpan {
				bestSpan = span
				found = true
				best = hostBind{
					name: d.Name, typ: d.Type, line: d.Line, fnLocal: true,
					fnLine: fnLine, fnEnd: fnEnd, blockLine: b.Line, blockEnd: b.EndLine,
					note: "Immutable local in this host if branch. It is not visible in the other branch.",
				}
			}
		}
	}
	visitHostExprs(circ, func(e parser.Expr, fnLine, fnEnd int) {
		parser.WalkIf(e, func(n *parser.IfExpr) {
			consider(n.Then, fnLine, fnEnd)
			consider(n.Else, fnLine, fnEnd)
		})
	})
	for _, f := range circ.Functions {
		considerStmtLets(f.Body, f.Line, f.EndLine, &best, &bestSpan, &found, line, word)
	}
	return best, found
}

func visitHostExprs(c *parser.Circuit, fn func(parser.Expr, int, int)) {
	if c == nil {
		return
	}
	for _, h := range c.Host {
		fn(h.Expr, 0, 0)
	}
	for _, f := range c.Functions {
		walkFnExprs(f.Body, f.Line, f.EndLine, fn)
	}
}

func walkFnExprs(stmts []parser.FnStmt, fnLine, fnEnd int, fn func(parser.Expr, int, int)) {
	for _, st := range stmts {
		switch st.Kind {
		case "let", "assign":
			fn(st.Decl.Expr, fnLine, fnEnd)
		case "return":
			fn(st.Expr, fnLine, fnEnd)
		case "if":
			if st.If != nil {
				fn(st.If.Cond, fnLine, fnEnd)
				walkFnExprs(st.If.Then, fnLine, fnEnd, fn)
				walkFnExprs(st.If.Else, fnLine, fnEnd, fn)
			}
		case "for", "while":
			if st.Loop != nil {
				fn(st.Loop.Cond, fnLine, fnEnd)
				fn(st.Loop.From, fnLine, fnEnd)
				fn(st.Loop.To, fnLine, fnEnd)
				walkFnExprs(st.Loop.Body, fnLine, fnEnd, fn)
			}
		}
	}
}

func addStmtBranchLets(items *[]completionItem, stmts []parser.FnStmt) {
	for _, st := range stmts {
		if st.If == nil {
			continue
		}
		addStmtLets(items, st.If.Then)
		addStmtLets(items, st.If.Else)
	}
}

func addStmtLets(items *[]completionItem, stmts []parser.FnStmt) {
	for _, st := range stmts {
		if st.Kind == "let" {
			*items = append(*items, completionItem{
				Label:         st.Decl.Name,
				Kind:          6,
				Detail:        fmt.Sprintf("%s branch local, line %d", st.Decl.Type, st.Line),
				Documentation: "Visible only in this host if branch.",
				InsertText:    st.Decl.Name,
			})
		}
		if st.If != nil {
			addStmtLets(items, st.If.Then)
			addStmtLets(items, st.If.Else)
		}
	}
}

func considerStmtLets(stmts []parser.FnStmt, fnLine, fnEnd int, best *hostBind, bestSpan *int, found *bool, line int, word string) {
	var walk func([]parser.FnStmt, int, int)
	walk = func(body []parser.FnStmt, blockLine, blockEnd int) {
		branch := !(blockLine == fnLine && blockEnd == fnEnd)
		for _, st := range body {
			if branch && st.Kind == "let" && st.Decl.Name == word && line >= st.Decl.Line && (blockLine == 0 || line >= blockLine) && (blockEnd == 0 || line <= blockEnd) {
				span := blockEnd - blockLine
				if blockEnd == 0 {
					span = fnEnd - fnLine
				}
				if !*found || span < *bestSpan {
					*bestSpan = span
					*found = true
					end := blockEnd
					start := blockLine
					if end == 0 {
						start, end = fnLine, fnEnd
					}
					*best = hostBind{
						name: st.Decl.Name, typ: st.Decl.Type, line: st.Decl.Line, fnLocal: true,
						fnLine: fnLine, fnEnd: fnEnd, blockLine: start, blockEnd: end,
						note: "Immutable local in this host if branch. It is not visible in the other branch.",
					}
				}
			}
			if st.If != nil {
				walk(st.If.Then, st.If.ThenLine, st.If.ThenEnd)
				walk(st.If.Else, st.If.ElseLine, st.If.ElseEnd)
			}
			if st.Loop != nil {
				walk(st.Loop.Body, st.Loop.BodyLine, st.Loop.BodyEnd)
			}
		}
	}
	walk(stmts, fnLine, fnEnd)
}

func (s *server) folding(id json.RawMessage, uri string) {
	text, ok := s.docs[uri]
	if !ok {
		s.respond(id, []any{})
		return
	}
	circ, err := parser.Parse(text)
	if err != nil {
		s.respond(id, []any{})
		return
	}
	type fr struct {
		StartLine int    `json:"startLine"`
		EndLine   int    `json:"endLine"`
		Kind      string `json:"kind"`
	}
	var out []fr
	add := func(start, end int) {
		if start < 1 || end <= start {
			return
		}
		out = append(out, fr{StartLine: start - 1, EndLine: end - 1, Kind: "region"})
	}
	var walk func([]parser.FnStmt)
	walk = func(stmts []parser.FnStmt) {
		for _, st := range stmts {
			if st.Kind == "let" || st.Kind == "assign" {
				parser.WalkIf(st.Decl.Expr, func(n *parser.IfExpr) { add(n.Line, n.EndLine) })
			}
			if st.Kind == "return" {
				parser.WalkIf(st.Expr, func(n *parser.IfExpr) { add(n.Line, n.EndLine) })
			}
			if st.If != nil {
				add(st.If.Line, st.If.EndLine)
				walk(st.If.Then)
				walk(st.If.Else)
			}
		}
	}
	for _, f := range circ.Functions {
		walk(f.Body)
	}
	for _, h := range circ.Host {
		parser.WalkIf(h.Expr, func(n *parser.IfExpr) { add(n.Line, n.EndLine) })
	}
	if out == nil {
		s.respond(id, []any{})
		return
	}
	s.respond(id, out)
}

func addBlockLets(items *[]completionItem, b *parser.BlockExpr) {
	if b == nil {
		return
	}
	for _, d := range b.Lets {
		*items = append(*items, completionItem{
			Label:         d.Name,
			Kind:          6,
			Detail:        fmt.Sprintf("%s branch local, line %d", d.Type, d.Line),
			Documentation: "Visible only in this host if branch.",
			InsertText:    d.Name,
		})
	}
}

func bindingAt(text string, pos position, word string) (hostBind, bool) {
	if b, ok := innermostBlockBind(text, pos, word); ok {
		return b, true
	}
	fn, inside := functionAt(text, pos.Line+1)
	if !inside {
		return hostBind{}, false
	}
	for _, p := range fn.Params {
		if p.Name == word {
			return hostBind{
				name: p.Name, typ: p.Type, line: fn.Line, fnLocal: true, fnLine: fn.Line, fnEnd: fn.EndLine,
				note: "Immutable function parameter of " + fn.Name + ". This is not a PARAM.",
			}, true
		}
	}
	for _, st := range fn.Body {
		if st.Kind == "let" && st.Decl.Name == word {
			return hostBind{
				name: st.Decl.Name, typ: st.Decl.Type, line: st.Line, fnLocal: true, fnLine: fn.Line, fnEnd: fn.EndLine,
				note: "Immutable local in " + fn.Name + ".",
			}, true
		}
	}
	return hostBind{}, false
}

func (s *server) renameInRange(id json.RawMessage, uri, text string, wordBoundary *regexp.Regexp, newName string, line1, end1 int) {
	var edits []textEdit
	for i, line := range docLines(text) {
		n := i + 1
		if n < line1 || (end1 > 0 && n > end1) {
			continue
		}
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

// signatureHelp reports the host function under the cursor when the user is
// inside a call. Conversions and gates are left to hover.
func (s *server) signatureHelp(id json.RawMessage, uri string, pos position) {
	text, ok := s.docs[uri]
	if !ok {
		s.respond(id, nil)
		return
	}
	lines := docLines(text)
	if pos.Line < 0 || pos.Line >= len(lines) {
		s.respond(id, nil)
		return
	}
	line := lines[pos.Line]
	if pos.Character < 0 || pos.Character > len(line) {
		s.respond(id, nil)
		return
	}
	name := callNameBefore(line[:pos.Character])
	fn, found := findFunction(text, name)
	if !found {
		s.respond(id, nil)
		return
	}
	s.respond(id, map[string]any{
		"signatures": []map[string]any{{
			"label": fnSignature(fn),
			"documentation": map[string]string{
				"kind":  "markdown",
				"value": "Host function. Arguments are immutable scalars.",
			},
		}},
		"activeSignature": 0,
	})
}

func callNameBefore(prefix string) string {
	i := strings.LastIndex(prefix, "(")
	if i < 0 {
		return ""
	}
	j := i - 1
	for j >= 0 && (prefix[j] == ' ' || prefix[j] == '\t') {
		j--
	}
	if j < 0 {
		return ""
	}
	end := j + 1
	for j >= 0 && (isIdentByte(prefix[j])) {
		j--
	}
	name := prefix[j+1 : end]
	if name == "" || !isIdentStart(name) {
		return ""
	}
	return name
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || (b >= '0' && b <= '9')
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
