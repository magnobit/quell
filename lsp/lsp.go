// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package lsp is a minimal Language Server Protocol server for Quell,
// speaking JSON-RPC 2.0 over stdio (the standard LSP transport — any
// editor that can launch a subprocess and talk LSP, e.g. VS Code via a
// thin client extension, Neovim's built-in LSP client, or Helix, can use
// this directly by pointing it at `quell lsp`).
//
// v1 scope is deliberately bounded to the two capabilities that come
// almost for free from existing infrastructure and give the most
// real-editor value: diagnostics (parser.Parse's existing line-numbered
// errors and Circuit.Warnings, surfaced as red squiggles) and
// documentFormatting (the format package, surfaced as format-on-save).
// Hover/completion/go-to-definition are natural next steps, not built here.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/format"
)

// Run starts the LSP server, reading requests from r and writing
// responses/notifications to w. It blocks until r is closed (e.g. the
// client sent "exit", or its stdio pipe closed).
func Run(r io.Reader, w io.Writer) error {
	s := &server{
		docs: map[string]string{},
		out:  w,
		mu:   &sync.Mutex{},
	}
	return s.loop(r)
}

type server struct {
	docs map[string]string // URI -> current content
	out  io.Writer
	mu   *sync.Mutex // serializes writes to out
}

// ─── JSON-RPC 2.0 / LSP framing ──────────────────────────────────────────

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *server) loop(r io.Reader) error {
	br := bufio.NewReader(r)
	for {
		msg, err := readMessage(br)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		switch msg.Method {
		case "initialize":
			s.respond(msg.ID, map[string]any{
				"capabilities": map[string]any{
					"textDocumentSync":           1, // full document sync
					"documentFormattingProvider": true,
				},
				"serverInfo": map[string]any{"name": "quell-lsp", "version": "0.1.0"},
			})
		case "initialized":
			// no-op notification
		case "shutdown":
			s.respond(msg.ID, nil)
		case "exit":
			return nil
		case "textDocument/didOpen":
			var p didOpenParams
			json.Unmarshal(msg.Params, &p)
			s.docs[p.TextDocument.URI] = p.TextDocument.Text
			s.publishDiagnostics(p.TextDocument.URI, p.TextDocument.Text)
		case "textDocument/didChange":
			var p didChangeParams
			json.Unmarshal(msg.Params, &p)
			if len(p.ContentChanges) > 0 {
				text := p.ContentChanges[len(p.ContentChanges)-1].Text
				s.docs[p.TextDocument.URI] = text
				s.publishDiagnostics(p.TextDocument.URI, text)
			}
		case "textDocument/didClose":
			var p didCloseParams
			json.Unmarshal(msg.Params, &p)
			delete(s.docs, p.TextDocument.URI)
			s.notify("textDocument/publishDiagnostics", map[string]any{
				"uri": p.TextDocument.URI, "diagnostics": []diagnostic{},
			})
		case "textDocument/formatting":
			var p formattingParams
			json.Unmarshal(msg.Params, &p)
			text, ok := s.docs[p.TextDocument.URI]
			if !ok {
				s.respond(msg.ID, []textEdit{})
				break
			}
			formatted := format.Format(text)
			if formatted == text {
				s.respond(msg.ID, []textEdit{})
				break
			}
			s.respond(msg.ID, []textEdit{{
				Range:   fullRange(text),
				NewText: formatted,
			}})
		default:
			// Unknown request with an ID must still get a response, per spec.
			if len(msg.ID) > 0 {
				s.respondError(msg.ID, -32601, "method not found: "+msg.Method)
			}
		}
	}
}

// publishDiagnostics compiles text (target doesn't matter — only parse
// errors/warnings are used) and sends textDocument/publishDiagnostics.
func (s *server) publishDiagnostics(uri, text string) {
	var diags []diagnostic

	result, err := compile.CompileWithWarnings(text, compile.OpenQASM, true)
	if err != nil {
		diags = append(diags, diagnosticFromMessage(err.Error(), 1))
	} else {
		for _, w := range result.Warnings {
			diags = append(diags, diagnosticFromMessage(w, 2))
		}
	}
	if diags == nil {
		diags = []diagnostic{}
	}

	s.notify("textDocument/publishDiagnostics", map[string]any{
		"uri": uri, "diagnostics": diags,
	})
}

var lineRe = regexp.MustCompile(`^line (\d+): (.*)$`)

// diagnosticFromMessage extracts a 1-indexed line number from Quell's
// standard "line N: ..." message prefix (used by both parser.Parse errors
// and line-scoped Circuit.Warnings), falling back to line 1 for
// circuit-level warnings that have no specific line (e.g. "no MEASURE
// instruction..."). severity: 1 = Error, 2 = Warning (LSP DiagnosticSeverity).
func diagnosticFromMessage(msg string, severity int) diagnostic {
	line := 1
	text := msg
	if m := lineRe.FindStringSubmatch(msg); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			line = n
		}
		text = m[2]
	}
	return diagnostic{
		Range:    lineRange(line),
		Severity: severity,
		Source:   "quell",
		Message:  text,
	}
}

// ─── LSP types (the minimal subset this server needs) ───────────────────

type position struct {
	Line      int `json:"line"`      // 0-indexed, per LSP spec
	Character int `json:"character"` // 0-indexed UTF-16 code unit offset
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

type textEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type textDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   textDocumentIdentifier `json:"textDocument"`
	ContentChanges []struct {
		Text string `json:"text"`
	} `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type formattingParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

// lineRange spans an entire 1-indexed source line, converted to LSP's
// 0-indexed line numbers.
func lineRange(line1Indexed int) lspRange {
	l := line1Indexed - 1
	if l < 0 {
		l = 0
	}
	return lspRange{Start: position{Line: l, Character: 0}, End: position{Line: l, Character: 1 << 20}}
}

// fullRange spans an entire document, for a whole-document formatting edit.
func fullRange(text string) lspRange {
	lines := strings.Count(text, "\n") + 1
	return lspRange{
		Start: position{Line: 0, Character: 0},
		End:   position{Line: lines, Character: 0},
	}
}

// ─── transport ────────────────────────────────────────────────────────

func readMessage(br *bufio.Reader) (*rpcMessage, error) {
	contentLength := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length header: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(br, body); err != nil {
		return nil, err
	}

	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("invalid JSON-RPC message: %w", err)
	}
	return &msg, nil
}

func (s *server) write(v any) {
	body, _ := json.Marshal(v)
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body))
	s.out.Write(body)
}

// respond always includes a "result" key, even for a nil result (e.g.
// shutdown's success response, per spec, must send result: null — not omit
// the field). rpcMessage's Result field can't do this: encoding/json's
// omitempty treats a nil `any` as empty and drops the key entirely, so the
// response is built as a map here instead, where every key we set is
// always emitted.
func (s *server) respond(id json.RawMessage, result any) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) respondError(id json.RawMessage, code int, message string) {
	s.write(rpcMessage{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *server) notify(method string, params any) {
	body, _ := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body))
	s.out.Write(body)
}
