// Copyright 2026 Magnobit, Inc. All rights reserved.

package lsp

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magnobit/quell/internal/parser"
)

// ─── unit tests for the pure helpers ───────────────────────────────────────

func TestWordAt(t *testing.T) {
	text := "gate bell a b {\n  H a\n}\n"
	word, start, end, ok := wordAt(text, position{Line: 0, Character: 6})
	if !ok || word != "bell" {
		t.Fatalf("wordAt at (0,6) = %q, %v, want \"bell\", true", word, ok)
	}
	if start != 5 || end != 9 {
		t.Errorf("wordAt span = [%d,%d), want [5,9)", start, end)
	}

	_, _, _, ok = wordAt(text, position{Line: 0, Character: 4}) // the space before "gate"'s g... actually inside "gate"
	if !ok {
		t.Errorf("expected a hit inside the word \"gate\" itself")
	}

	_, _, _, ok = wordAt("", position{Line: 5, Character: 0})
	if ok {
		t.Error("expected no match past the end of a short document")
	}
}

func TestMacrosIn_ValidParse(t *testing.T) {
	text := "gate bell a b {\n  H a\n  CNOT a b\n}\n\nbell 0 1\nMEASURE\n"
	macros := macrosIn(text)
	if len(macros) != 1 {
		t.Fatalf("expected 1 macro, got %d: %v", len(macros), macros)
	}
	if macros[0].Name != "BELL" || macros[0].Line != 1 {
		t.Errorf("macro = %+v, want Name=BELL Line=1", macros[0])
	}
	if len(macros[0].Params) != 2 || macros[0].Params[0] != "a" || macros[0].Params[1] != "b" {
		t.Errorf("macro params = %v, want [a b]", macros[0].Params)
	}
}

// This is the case that actually matters for an editor: the user is
// mid-edit and the document doesn't parse, but completion/hover for the
// macro they already typed should still work.
func TestMacrosIn_FallsBackWhenUnparseable(t *testing.T) {
	text := "gate bell a b {\n  H a\n  CNOT a b\n}\n\nbell 0 1\nTHIS IS NOT VALID QUELL AT ALL\n"
	macros := macrosIn(text)
	if len(macros) != 1 || macros[0].Name != "BELL" {
		t.Fatalf("expected fallback scan to still find macro BELL, got %v", macros)
	}
}

func TestFindMacro_CaseInsensitive(t *testing.T) {
	text := "gate Bell a b {\n  H a\n}\n\nBell 0 1\nMEASURE\n"
	m, ok := findMacro(text, "bell")
	if !ok || m.Name != "BELL" {
		t.Fatalf("findMacro(\"bell\") = %+v, %v, want a case-insensitive hit", m, ok)
	}
	if _, ok := findMacro(text, "nonexistent"); ok {
		t.Error("expected no match for an undefined name")
	}
}

func TestArityText(t *testing.T) {
	cases := []struct {
		name string
		spec parser.GateSpec
		want string
	}{
		{"H", parser.GateSpec{Qubits: 1, Args: 0}, "1 qubit"},
		{"CNOT", parser.GateSpec{Qubits: 2, Args: 0}, "2 qubits"},
		{"RX", parser.GateSpec{Qubits: 1, Args: 1}, "1 qubit, 1 angle argument"},
		{"variadic", parser.GateSpec{Qubits: -1, Args: 0}, "variadic qubits"},
	}
	for _, c := range cases {
		got := arityText(c.spec)
		if got != c.want {
			t.Errorf("arityText(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCompletionSnippet(t *testing.T) {
	if got := completionSnippet("H"); got != "H ${1:q0}" {
		t.Errorf("completionSnippet(H) = %q, want \"H ${1:q0}\"", got)
	}
	if got := completionSnippet("RX"); got != "RX ${1:angle1} ${2:q0}" {
		t.Errorf("completionSnippet(RX) = %q, want \"RX ${1:angle1} ${2:q0}\"", got)
	}
	if got := completionSnippet("MEASURE"); got != "MEASURE" {
		t.Errorf("completionSnippet(MEASURE) = %q, want plain \"MEASURE\" (variadic, no fixed slots)", got)
	}
}

func TestURIPathRoundTrip(t *testing.T) {
	uri := pathToURI(filepath.Join("testdata", "bell.quell"))
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("pathToURI result doesn't look like a file URI: %q", uri)
	}
	back, err := uriToPath(uri)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(filepath.Join("testdata", "bell.quell"))
	if filepath.Clean(back) != filepath.Clean(abs) {
		t.Errorf("round-tripped path = %q, want %q", back, abs)
	}
}

// ─── full-pipe integration tests for the new LSP methods ──────────────────

type lspClient struct {
	w  io.Writer
	br *bufio.Reader
}

func startServer(t *testing.T) *lspClient {
	t.Helper()
	serverInR, clientToServer := io.Pipe()
	serverToClient, serverOutW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Run(serverInR, serverOutW) }()
	t.Cleanup(func() {
		writeMsg(clientToServer, map[string]any{"jsonrpc": "2.0", "method": "exit"})
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return &lspClient{w: clientToServer, br: bufio.NewReader(serverToClient)}
}

func (c *lspClient) send(v any) map[string]any {
	writeMsg(c.w, v)
	return readMsg(c.br)
}

func (c *lspClient) openDoc(uri, text string) {
	writeMsg(c.w, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen",
		"params": map[string]any{"textDocument": map[string]any{"uri": uri, "text": text}},
	})
	readMsg(c.br) // diagnostics notification
}

func TestHover_BuiltinGate(t *testing.T) {
	c := startServer(t)
	c.openDoc("file:///t.quell", "H 0\nMEASURE\n")
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 0, "character": 0},
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a hover result for a built-in gate, got nil")
	}
	contents, _ := result["contents"].(map[string]any)
	value, _ := contents["value"].(string)
	if !strings.Contains(value, "Hadamard") {
		t.Errorf("hover text = %q, want it to mention Hadamard", value)
	}
}

func TestHover_Macro(t *testing.T) {
	c := startServer(t)
	src := "gate bell a b {\n  H a\n  CNOT a b\n}\n\nbell 0 1\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 5, "character": 1}, // inside "bell 0 1"
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a hover result for a macro invocation, got nil")
	}
	contents, _ := result["contents"].(map[string]any)
	value, _ := contents["value"].(string)
	if !strings.Contains(value, "line 1") {
		t.Errorf("macro hover text = %q, want it to reference its definition line", value)
	}
}

func TestHover_NoMatch(t *testing.T) {
	c := startServer(t)
	c.openDoc("file:///t.quell", "H 0\nMEASURE\n")
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 10, "character": 0},
		},
	})
	if resp["result"] != nil {
		t.Errorf("expected nil result out of document bounds, got %v", resp["result"])
	}
}

func TestCompletion_IncludesGatesAndMacros(t *testing.T) {
	c := startServer(t)
	src := "gate bell a b {\n  H a\n  CNOT a b\n}\n\nbell 0 1\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 6, "character": 0},
		},
	})
	items, _ := resp["result"].([]any)
	if len(items) == 0 {
		t.Fatal("expected completion items, got none")
	}
	var sawH, sawBell bool
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		switch item["label"] {
		case "H":
			sawH = true
			if item["insertText"] != "H ${1:q0}" {
				t.Errorf("H completion insertText = %v, want snippet with a qubit slot", item["insertText"])
			}
		case "BELL":
			sawBell = true
		}
	}
	if !sawH {
		t.Error("expected built-in gate H in completion items")
	}
	if !sawBell {
		t.Error("expected user macro BELL in completion items")
	}
}

func TestDefinition_MacroInvocation(t *testing.T) {
	c := startServer(t)
	src := "gate bell a b {\n  H a\n  CNOT a b\n}\n\nbell 0 1\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/definition",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 5, "character": 1},
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a definition location, got nil")
	}
	rng, _ := result["range"].(map[string]any)
	start, _ := rng["start"].(map[string]any)
	if line, _ := start["line"].(float64); line != 0 {
		t.Errorf("definition line = %v, want 0 (macro declared on source line 1)", line)
	}
}

func TestDefinition_Import(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bell_pair.quell")
	if err := os.WriteFile(target, []byte("H 0\nMEASURE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.quell")

	c := startServer(t)
	c.openDoc(pathToURI(mainPath), `import "./bell_pair.quell"`+"\n")
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/definition",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": pathToURI(mainPath)},
			"position":     map[string]any{"line": 0, "character": 10},
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a definition location for the import, got nil")
	}
	gotURI, _ := result["uri"].(string)
	if gotURI != pathToURI(target) {
		t.Errorf("import definition uri = %q, want %q", gotURI, pathToURI(target))
	}
}

func TestRename_Macro(t *testing.T) {
	c := startServer(t)
	src := "gate bell a b {\n  H a\n  CNOT a b\n}\n\nbell 0 1\nbell 2 3\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/rename",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 5, "character": 1},
			"newName":      "epr",
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a workspace edit, got nil")
	}
	changes, _ := result["changes"].(map[string]any)
	edits, _ := changes["file:///t.quell"].([]any)
	// Header "gate bell" + two invocations "bell 0 1" / "bell 2 3" = 3 occurrences.
	if len(edits) != 3 {
		t.Fatalf("expected 3 rename edits (1 header + 2 invocations), got %d: %v", len(edits), edits)
	}
	for _, raw := range edits {
		e, _ := raw.(map[string]any)
		if e["newText"] != "epr" {
			t.Errorf("edit newText = %v, want \"epr\"", e["newText"])
		}
	}
}

func TestRename_RejectsNonMacro(t *testing.T) {
	c := startServer(t)
	c.openDoc("file:///t.quell", "H 0\nMEASURE\n")
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/rename",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 0, "character": 0}, // "H" — a built-in gate, not a macro
			"newName":      "hadamard",
		},
	})
	if resp["error"] == nil {
		t.Errorf("expected an error renaming a built-in gate name, got result: %v", resp["result"])
	}
}

// ─── named qubit support (hover/completion/definition/rename) ─────────────

func TestQubitDeclsIn_ValidParse(t *testing.T) {
	text := "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n"
	decls := qubitDeclsIn(text)
	if len(decls) != 2 {
		t.Fatalf("expected 2 qubit decls, got %d: %v", len(decls), decls)
	}
	if decls[0].Name != "alice" || decls[0].Line != 1 {
		t.Errorf("decls[0] = %+v, want Name=alice Line=1", decls[0])
	}
	if decls[1].Name != "bob" || decls[1].Line != 1 {
		t.Errorf("decls[1] = %+v, want Name=bob Line=1", decls[1])
	}
}

func TestFindQubitDecl_IsCaseSensitive(t *testing.T) {
	text := "qubit alice\nH alice\nMEASURE\n"
	if _, ok := findQubitDecl(text, "alice"); !ok {
		t.Error("expected an exact-case match for \"alice\"")
	}
	if _, ok := findQubitDecl(text, "Alice"); ok {
		t.Error("named qubits are case-sensitive in Quell — \"Alice\" must not match a declaration of \"alice\"")
	}
}

func TestHover_NamedQubit(t *testing.T) {
	c := startServer(t)
	src := "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 1, "character": 2}, // inside "H alice"
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a hover result for a named qubit reference, got nil")
	}
	contents, _ := result["contents"].(map[string]any)
	value, _ := contents["value"].(string)
	if !strings.Contains(value, "named qubit") || !strings.Contains(value, "line 1") {
		t.Errorf("qubit hover text = %q, want it to say \"named qubit\" and reference line 1", value)
	}
}

func TestCompletion_IncludesNamedQubits(t *testing.T) {
	c := startServer(t)
	src := "qubit alice, bob\nH alice\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 2, "character": 0},
		},
	})
	items, _ := resp["result"].([]any)
	var sawAlice, sawBob bool
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		switch item["label"] {
		case "alice":
			sawAlice = true
			if kind, _ := item["kind"].(float64); kind != 6 {
				t.Errorf("alice completion kind = %v, want 6 (Variable)", kind)
			}
		case "bob":
			sawBob = true
		}
	}
	if !sawAlice || !sawBob {
		t.Errorf("expected both named qubits alice and bob in completion items, sawAlice=%v sawBob=%v", sawAlice, sawBob)
	}
}

func TestDefinition_NamedQubit(t *testing.T) {
	c := startServer(t)
	src := "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/definition",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 2, "character": 5}, // "alice" inside "CNOT alice bob"
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a definition location for the named qubit, got nil")
	}
	rng, _ := result["range"].(map[string]any)
	start, _ := rng["start"].(map[string]any)
	if line, _ := start["line"].(float64); line != 0 {
		t.Errorf("definition line = %v, want 0 (qubit declared on source line 1)", line)
	}
}

func TestRename_NamedQubit(t *testing.T) {
	c := startServer(t)
	src := "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/rename",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 1, "character": 2}, // "alice" in "H alice"
			"newName":      "sender",
		},
	})
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("expected a workspace edit, got nil")
	}
	changes, _ := result["changes"].(map[string]any)
	edits, _ := changes["file:///t.quell"].([]any)
	// "qubit alice, bob" + "H alice" + "CNOT alice bob" = 3 occurrences of "alice".
	if len(edits) != 3 {
		t.Fatalf("expected 3 rename edits, got %d: %v", len(edits), edits)
	}
}

func TestRename_QubitIsCaseSensitive(t *testing.T) {
	c := startServer(t)
	// "Alice" (capital) appears nowhere — only "alice" does. A case-sensitive
	// rename must not touch a same-spelled-different-case token.
	src := "qubit alice\nH alice\n// Alice is a person's name in this comment, not the qubit\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/rename",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 1, "character": 2},
			"newName":      "sender",
		},
	})
	result, _ := resp["result"].(map[string]any)
	changes, _ := result["changes"].(map[string]any)
	edits, _ := changes["file:///t.quell"].([]any)
	if len(edits) != 2 {
		t.Fatalf("expected exactly 2 edits (declaration + \"H alice\"), got %d: %v", len(edits), edits)
	}
	for _, raw := range edits {
		e, _ := raw.(map[string]any)
		rng, _ := e["range"].(map[string]any)
		start, _ := rng["start"].(map[string]any)
		if line, _ := start["line"].(float64); line == 2 {
			t.Errorf("rename must not touch line 2 (the comment's capitalized \"Alice\"), got edit: %v", e)
		}
	}
}

func TestRename_IgnoresMatchInsideComment(t *testing.T) {
	c := startServer(t)
	// Same-case "alice" inside a comment must not be touched — rename
	// operates on code, not prose that happens to mention the same word.
	src := "qubit alice\nH alice // not alice, just a coincidental word here\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/rename",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 0, "character": 8}, // "alice" in "qubit alice"
			"newName":      "sender",
		},
	})
	result, _ := resp["result"].(map[string]any)
	changes, _ := result["changes"].(map[string]any)
	edits, _ := changes["file:///t.quell"].([]any)
	// Only the declaration and the real "H alice" reference — the two
	// "alice" mentions inside the trailing comment on line 1 must be excluded.
	if len(edits) != 2 {
		t.Fatalf("expected exactly 2 edits (declaration + \"H alice\", excluding the comment), got %d: %v", len(edits), edits)
	}
}

func TestRename_RejectsNameCollisionForQubit(t *testing.T) {
	c := startServer(t)
	src := "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n"
	c.openDoc("file:///t.quell", src)
	resp := c.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/rename",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///t.quell"},
			"position":     map[string]any{"line": 1, "character": 2}, // "alice"
			"newName":      "bob",                                     // already exists
		},
	})
	if resp["error"] == nil {
		t.Errorf("expected an error renaming alice to an already-existing qubit name \"bob\", got result: %v", resp["result"])
	}
}
