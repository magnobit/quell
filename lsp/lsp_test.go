// Copyright 2026 Magnobit, Inc. All rights reserved.

package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// writeMsg frames a request/notification exactly like a real LSP client.
func writeMsg(w io.Writer, v any) {
	body, _ := json.Marshal(v)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	w.Write(body)
}

// readMsg reads one framed message from a real server response stream.
func readMsg(br *bufio.Reader) map[string]any {
	contentLength := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")), "%d", &contentLength)
		}
	}
	body := make([]byte, contentLength)
	io.ReadFull(br, body)
	var m map[string]any
	json.Unmarshal(body, &m)
	return m
}

func TestLSPFullFlow(t *testing.T) {
	serverInR, clientToServer := io.Pipe()
	serverToClient, serverOutW := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- Run(serverInR, serverOutW)
	}()

	br := bufio.NewReader(serverToClient)

	// 1. initialize
	writeMsg(clientToServer, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	resp := readMsg(br)
	if resp == nil {
		t.Fatal("no response to initialize")
	}
	result, _ := resp["result"].(map[string]any)
	caps, _ := result["capabilities"].(map[string]any)
	if caps["documentFormattingProvider"] != true {
		t.Errorf("expected documentFormattingProvider=true, got capabilities: %v", caps)
	}

	// 2. initialized notification (no response expected)
	writeMsg(clientToServer, map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})

	// 3. didOpen with a syntax error — expect a diagnostic
	writeMsg(clientToServer, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":  "file:///bad.quell",
				"text": "FOOGATE 0\n",
			},
		},
	})
	diagMsg := readMsg(br)
	if diagMsg["method"] != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics notification, got: %v", diagMsg)
	}
	params, _ := diagMsg["params"].(map[string]any)
	diags, _ := params["diagnostics"].([]any)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for unknown gate, got %d: %v", len(diags), diags)
	}
	d0 := diags[0].(map[string]any)
	if !strings.Contains(d0["message"].(string), "unknown gate") {
		t.Errorf("expected 'unknown gate' in diagnostic message, got: %v", d0["message"])
	}
	if sev, _ := d0["severity"].(float64); sev != 1 {
		t.Errorf("expected severity 1 (Error) for a parse error, got %v", d0["severity"])
	}

	// 4. didChange to valid-but-warning-producing content (no MEASURE)
	writeMsg(clientToServer, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": "file:///bad.quell"},
			"contentChanges": []map[string]any{{"text": "H 0\nCNOT 0 1\n"}},
		},
	})
	diagMsg2 := readMsg(br)
	params2, _ := diagMsg2["params"].(map[string]any)
	diags2, _ := params2["diagnostics"].([]any)
	if len(diags2) == 0 {
		t.Fatal("expected at least one warning diagnostic (missing MEASURE)")
	}
	d1 := diags2[0].(map[string]any)
	if sev, _ := d1["severity"].(float64); sev != 2 {
		t.Errorf("expected severity 2 (Warning) for a semantic warning, got %v", d1["severity"])
	}

	// 5. formatting request on messy-but-valid source
	writeMsg(clientToServer, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": "file:///bad.quell"},
			"contentChanges": []map[string]any{{"text": "h    0\ncnot 0 1\nmeasure\n"}},
		},
	})
	readMsg(br) // diagnostics notification for the didChange above

	writeMsg(clientToServer, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "textDocument/formatting",
		"params": map[string]any{"textDocument": map[string]any{"uri": "file:///bad.quell"}},
	})
	fmtResp := readMsg(br)
	edits, _ := fmtResp["result"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected exactly one text edit, got %d: %v", len(edits), edits)
	}
	edit := edits[0].(map[string]any)
	newText, _ := edit["newText"].(string)
	if newText != "H 0\nCNOT 0 1\nMEASURE\n" {
		t.Errorf("unexpected formatted text: %q", newText)
	}

	// 6. shutdown + exit
	writeMsg(clientToServer, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"})
	shutdownResp := readMsg(br)
	if _, ok := shutdownResp["result"]; !ok {
		t.Errorf("expected a result field in shutdown response, got: %v", shutdownResp)
	}
	writeMsg(clientToServer, map[string]any{"jsonrpc": "2.0", "method": "exit"})

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not exit after 'exit' notification")
	}
}

func TestDiagnosticFromMessage(t *testing.T) {
	d := diagnosticFromMessage("line 5: unknown gate \"FOO\" (valid gates: ...)", 1)
	if d.Range.Start.Line != 4 { // 0-indexed
		t.Errorf("expected 0-indexed line 4, got %d", d.Range.Start.Line)
	}
	if strings.Contains(d.Message, "line 5:") {
		t.Errorf("message should have the line prefix stripped, got: %q", d.Message)
	}

	d2 := diagnosticFromMessage("no MEASURE instruction — simulation results will all be |0⟩", 2)
	if d2.Range.Start.Line != 0 {
		t.Errorf("circuit-level warning with no line prefix should default to line 0, got %d", d2.Range.Start.Line)
	}
}

func TestReadMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	writeMsg(&buf, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	msg, err := readMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Method != "initialize" {
		t.Errorf("expected method=initialize, got %q", msg.Method)
	}
}
