// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qerr defines structured Quell errors for parse / compile / convert /
// simulate failures so callers can branch on Kind without string matching.
package qerr

import (
	"encoding/json"
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
	KindCheck    Kind = "check"
)

// Diagnostic codes. QL1xxx is syntax, QL2xxx is host/semantic.
const (
	CodeSyntax        = "QL1001"
	CodeExprDepth     = "QL1002"
	CodeLimit         = "QL1003"
	CodeUnknownIdent  = "QL2001"
	CodeDuplicate     = "QL2002"
	CodeParamClash    = "QL2003"
	CodeTypeMismatch  = "QL2004"
	CodeBadOperator   = "QL2005"
	CodeBadConvert    = "QL2006"
	CodeImmutable     = "QL2007"
	CodeDivZero       = "QL2008"
	CodeUnknownFunc   = "QL2101"
	CodeDuplicateFunc = "QL2102"
	CodeFuncGate      = "QL2103"
	CodeArgCount      = "QL2104"
	CodeArgType       = "QL2105"
	CodeMissingReturn = "QL2201"
	CodeReturnType    = "QL2202"
	CodeRecursion     = "QL2203"
	CodeDupParam      = "QL2204"
	CodeIfCond        = "QL2301"
	CodeIfType        = "QL2302"
	CodeIfElse        = "QL2303"
	CodeIfBlock       = "QL2304"
	CodeIfPaths       = "QL2305"
	CodeUnreachable   = "QL2306"
	CodeFormat        = "QL2401"
	CodeFormatCount   = "QL2402"
	CodeFormatType    = "QL2403"
	CodeInterp        = "QL2404"
	CodePrintValue    = "QL2405"
	CodeLoopLimit     = "QL2501"
	CodeBreak         = "QL2502"
	CodeHostLoop      = "QL2503"
	CodeIndex         = "QL2601"
	CodeArrayLimit    = "QL2602"
	CodeArrayType     = "QL2603"
)

// SeverityError is the diagnostic severity for a rejected program.
const SeverityError = "error"

// SeverityWarning is a diagnostic that does not reject the program.
const SeverityWarning = "warning"

// DocsHostValues is the specification anchor for typed host values.
const DocsHostValues = "https://github.com/magnobit/quell/blob/main/SPEC.md#typed-host-values"

// DocsHostFunctions is the specification anchor for host functions.
const DocsHostFunctions = "https://github.com/magnobit/quell/blob/main/SPEC.md#host-functions"

// DocsHostIf is the specification anchor for host conditionals.
const DocsHostIf = "https://github.com/magnobit/quell/blob/main/SPEC.md#host-conditionals"

// DocsHostLoops is the specification anchor for host loops and mutation.
const DocsHostLoops = "https://github.com/magnobit/quell/blob/main/SPEC.md#host-loops"

// DocsHostOutput is the specification anchor for print and format.
const DocsHostOutput = "https://github.com/magnobit/quell/blob/main/SPEC.md#host-output"

// DocsHostArrays is the specification anchor for host arrays.
const DocsHostArrays = "https://github.com/magnobit/quell/blob/main/SPEC.md#host-arrays"

// Diagnostic is a stable, source-located message for tooling.
type Diagnostic struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Line         int    `json:"line"`
	Column       int    `json:"column"`
	EndLine      int    `json:"endLine"`
	EndColumn    int    `json:"endColumn"`
	SuggestedFix string `json:"suggestedFix,omitempty"`
	DocsURL      string `json:"docsURL,omitempty"`
}

// JSON returns the diagnostic as a single JSON object.
func (d Diagnostic) JSON() ([]byte, error) {
	return json.Marshal(d)
}

// DiagnosticsJSON returns a JSON array of diagnostics.
func DiagnosticsJSON(ds []Diagnostic) ([]byte, error) {
	if ds == nil {
		ds = []Diagnostic{}
	}
	return json.Marshal(ds)
}

// Error is a Quell failure with optional line number and operation.
type Error struct {
	Kind Kind
	Op   string // e.g. "compile", "qasmimport", "WHILE"
	Line int    // 1-based; 0 if unknown
	Msg  string
	Err  error // wrapped cause
	Diag *Diagnostic
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
	if e.Diag != nil && e.Diag.Code != "" {
		return fmt.Sprintf("%s: %s: %s", prefix, e.Diag.Code, e.Diag.Message)
	}
	if e.Msg != "" {
		return fmt.Sprintf("%s: %s", prefix, e.Msg)
	}
	return prefix
}

// New returns an error that carries a structured diagnostic.
func New(kind Kind, d Diagnostic) error {
	if d.Severity == "" {
		d.Severity = SeverityError
	}
	if d.DocsURL == "" && len(d.Code) >= 4 && d.Code[:4] == "QL23" {
		d.DocsURL = DocsHostIf
	}
	if d.DocsURL == "" && len(d.Code) >= 4 && d.Code[:4] == "QL24" {
		d.DocsURL = DocsHostOutput
	}
	if d.DocsURL == "" && len(d.Code) >= 4 && d.Code[:4] == "QL25" {
		d.DocsURL = DocsHostLoops
	}
	if d.DocsURL == "" && len(d.Code) >= 4 && d.Code[:4] == "QL26" {
		d.DocsURL = DocsHostArrays
	}
	if d.DocsURL == "" && len(d.Code) >= 4 && (d.Code[:4] == "QL21" || d.Code[:4] == "QL22") {
		d.DocsURL = DocsHostFunctions
	}
	if d.DocsURL == "" && len(d.Code) >= 3 && (d.Code[:3] == "QL1" || d.Code[:3] == "QL2") {
		d.DocsURL = DocsHostValues
	}
	if d.EndLine == 0 {
		d.EndLine = d.Line
	}
	cp := d
	return &Error{Kind: kind, Op: "host", Line: d.Line, Msg: d.Message, Diag: &cp}
}

// AsDiagnostic reports the structured diagnostic carried by err, if any.
func AsDiagnostic(err error) (Diagnostic, bool) {
	for err != nil {
		if e, ok := err.(*Error); ok && e.Diag != nil {
			return *e.Diag, true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return Diagnostic{}, false
		}
		err = u.Unwrap()
	}
	return Diagnostic{}, false
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
