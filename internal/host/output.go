// Copyright 2026 Magnobit, Inc. All rights reserved.

package host

import (
	"io"
	"os"
	"strings"
	"sync"
)

// MaxOutputBytes is the cap for one host print. Extra bytes are truncated.
const MaxOutputBytes = 65536

// Writer receives host program output. Compiler diagnostics do not use it.
type Writer interface {
	Write(s string)
}

// Buffer captures host output for tests and notebooks.
type Buffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *Buffer) Write(s string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.WriteString(limitOutput(s))
}

func (b *Buffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *Buffer) Reset() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

type streamWriter struct{ w io.Writer }

func (s streamWriter) Write(text string) {
	if s.w == nil {
		return
	}
	_, _ = io.WriteString(s.w, limitOutput(text))
}

// Stdout is the CLI destination.
func Stdout() Writer { return streamWriter{w: os.Stdout} }

type discard struct{}

func (discard) Write(string) {}

var (
	outMu sync.Mutex
	out   Writer = discard{}
)

// SetWriter installs the process host-output destination.
// Nil selects a discard writer so library tests stay quiet.
func SetWriter(w Writer) {
	outMu.Lock()
	defer outMu.Unlock()
	if w == nil {
		out = discard{}
		return
	}
	out = w
}

// WriteOut sends host program text to the installed writer.
func WriteOut(s string) {
	outMu.Lock()
	w := out
	outMu.Unlock()
	w.Write(s)
}

func limitOutput(s string) string {
	if len(s) <= MaxOutputBytes {
		return redact(s)
	}
	return redact(s[:MaxOutputBytes]) + "...\n"
}

func redact(s string) string {
	if strings.Contains(strings.ToLower(s), "bearer ") || strings.Contains(s, "sk-") {
		return "[redacted]\n"
	}
	return s
}
