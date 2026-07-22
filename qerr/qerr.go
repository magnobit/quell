// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qerr defines structured Quell errors for parse / compile / convert /
// simulate failures so callers can branch on Kind without string matching.
package qerr

import (
	"fmt"
)

// Kind classifies where an error originated.
type Kind string

const (
	KindParse    Kind = "parse"
	KindCompile  Kind = "compile"
	KindConvert  Kind = "convert"
	KindSimulate Kind = "simulate"
	KindConfig   Kind = "config"
	KindInternal Kind = "internal"
)

// Error is a Quell failure with optional line number and operation.
type Error struct {
	Kind Kind
	Op   string // e.g. "compile", "qasmimport", "WHILE"
	Line int    // 1-based; 0 if unknown
	Msg  string
	Err  error // wrapped cause
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	prefix := string(e.Kind)
	if e.Op != "" {
		prefix += "/" + e.Op
	}
	if e.Line > 0 {
		prefix += fmt.Sprintf(" line %d", e.Line)
	}
	if e.Err != nil {
		if e.Msg != "" {
			return fmt.Sprintf("%s: %s: %v", prefix, e.Msg, e.Err)
		}
		return fmt.Sprintf("%s: %v", prefix, e.Err)
	}
	if e.Msg != "" {
		return fmt.Sprintf("%s: %s", prefix, e.Msg)
	}
	return prefix
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsKind reports whether err (or any wrapped) is a *Error of kind k.
func IsKind(err error, k Kind) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			return e.Kind == k
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func Parse(op string, line int, format string, args ...any) error {
	return &Error{Kind: KindParse, Op: op, Line: line, Msg: fmt.Sprintf(format, args...)}
}

func Compile(op string, err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return &Error{Kind: KindCompile, Op: op, Err: err}
}

func Convert(op string, err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return &Error{Kind: KindConvert, Op: op, Err: err}
}

func ConvertMsg(op, format string, args ...any) error {
	return &Error{Kind: KindConvert, Op: op, Msg: fmt.Sprintf(format, args...)}
}

func Simulate(op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: KindSimulate, Op: op, Err: err}
}

func Wrap(kind Kind, op string, err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return &Error{Kind: kind, Op: op, Err: err}
}
