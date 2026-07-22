// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package log provides structured logging for Quell libraries and the CLI.
//
// Default uses Go's log/slog to stderr. Set QUELL_LOG_LEVEL=debug|info|warn|error
// (or call SetLevel) before running commands. Libraries should log at Debug/Info
// for progress and Error when returning a failure to the caller — never panic
// for user input errors.
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	mu      sync.RWMutex
	logger  = newDefault()
	levelVar slog.LevelVar
)

func newDefault() *slog.Logger {
	levelVar.Set(parseLevel(os.Getenv("QUELL_LOG_LEVEL")))
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &levelVar})
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetLevel changes the global log level (debug, info, warn, error).
func SetLevel(level string) {
	levelVar.Set(parseLevel(level))
}

// SetOutput replaces the default stderr text handler (tests / embedding).
func SetOutput(w io.Writer, level string) {
	levelVar.Set(parseLevel(level))
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: &levelVar})
	mu.Lock()
	logger = slog.New(h)
	mu.Unlock()
}

// L returns the process-wide Quell logger.
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

// With returns a child logger with attrs.
func With(args ...any) *slog.Logger {
	return L().With(args...)
}

func Debug(msg string, args ...any) { L().Debug(msg, args...) }
func Info(msg string, args ...any)  { L().Info(msg, args...) }
func Warn(msg string, args ...any)  { L().Warn(msg, args...) }
func Error(msg string, args ...any) { L().Error(msg, args...) }

// DebugContext / InfoContext keep context for cancel-aware callers.
func DebugContext(ctx context.Context, msg string, args ...any) {
	L().DebugContext(ctx, msg, args...)
}
func InfoContext(ctx context.Context, msg string, args ...any) {
	L().InfoContext(ctx, msg, args...)
}
func ErrorContext(ctx context.Context, msg string, args ...any) {
	L().ErrorContext(ctx, msg, args...)
}
