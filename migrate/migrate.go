// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package migrate deterministically converts Qiskit, Cirq, Q#, Braket, and
// OpenQASM source into Quell without calling any external API. It is the
// same conversion logic qubitlabs-platform's /api/v1/ai/convert endpoint
// falls back to when ANTHROPIC_API_KEY is unset, ported here so the
// quell/quell-cli CLIs can do the same `convert` work with no key and no
// network call.
package migrate

import (
	"regexp"
	"strings"

	"github.com/magnobit/quell/qasm"
)

// Result is Quell source plus soft warnings for skipped/unsupported lines.
type Result struct {
	Quell    string
	Language string
	Warnings []string
}

// IsPythonish reports whether an LLM fallback is worth trying after ToQuell's
// deterministic converter has already failed. requestedLang is what the
// caller asked for; detectedLang is what ToQuell's own detector guessed even
// though conversion still failed (empty if detection didn't get that far).
// An LLM prompt for Qiskit/Cirq conversion shouldn't be handed genuine Q# or
// OpenQASM source just because auto-detect confidently identified it and
// conversion still failed — that would silently produce a wrong "answer"
// instead of surfacing the real error.
func IsPythonish(requestedLang, detectedLang string) bool {
	switch requestedLang {
	case "cirq", "qiskit", "python":
		return true
	case "", "auto":
		switch detectedLang {
		case "", "cirq", "qiskit", "python":
			return true
		}
		return false
	default:
		return false
	}
}

// ToQuell routes by language (or auto-detect) to the matching deterministic converter.
func ToQuell(code, language string) (Result, error) {
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "" || lang == "auto" {
		lang = DetectLang(code)
	}
	code = stripSourceComments(code, lang)
	switch lang {
	case "openqasm", "qasm", "qasm2", "qasm3", "openqasm2", "openqasm3":
		r, err := qasm.Convert(code)
		return Result{Quell: r.Quell, Language: "openqasm", Warnings: r.Warnings}, err
	case "qsharp", "q#", "qs":
		out, warns, err := convertQSharpToQuell(code)
		return Result{Quell: out, Language: "qsharp", Warnings: warns}, err
	case "cirq":
		out, warns, err := convertCirqToQuell(code)
		return Result{Quell: out, Language: "cirq", Warnings: warns}, err
	case "braket":
		out, warns, err := convertPythonToQuell(code)
		return Result{Quell: out, Language: "braket", Warnings: warns}, err
	case "qiskit", "python":
		out, warns, err := convertPythonToQuell(code)
		used := lang
		if used == "python" {
			used = DetectLang(code)
			if used == "python" {
				used = "qiskit"
			}
		}
		return Result{Quell: out, Language: used, Warnings: warns}, err
	default:
		if looksLikeQASM(code) {
			r, err := qasm.Convert(code)
			return Result{Quell: r.Quell, Language: "openqasm", Warnings: r.Warnings}, err
		}
		if looksLikeQSharp(code) {
			out, warns, err := convertQSharpToQuell(code)
			return Result{Quell: out, Language: "qsharp", Warnings: warns}, err
		}
		if strings.Contains(code, "cirq.") || strings.Contains(code, "import cirq") {
			out, warns, err := convertCirqToQuell(code)
			return Result{Quell: out, Language: "cirq", Warnings: warns}, err
		}
		out, warns, err := convertPythonToQuell(code)
		return Result{Quell: out, Language: DetectLang(code), Warnings: warns}, err
	}
}

// DetectLang guesses the source language from content alone (no filename) —
// exported so callers that already have a resolved MigrateToQuell-style
// language string (e.g. qubitlabs-platform's Convert handler reporting the
// language used) can get the same detection ToQuell uses internally.
func DetectLang(code string) string {
	if looksLikeQASM(code) {
		return "openqasm"
	}
	if looksLikeQSharp(code) {
		return "qsharp"
	}
	if strings.Contains(code, "cirq.") || strings.Contains(code, "import cirq") {
		return "cirq"
	}
	if strings.Contains(code, "braket.circuits") || strings.Contains(code, "from braket") {
		return "braket"
	}
	if strings.Contains(code, "QuantumCircuit") || strings.Contains(code, "qiskit") || strings.Contains(code, "qc.") {
		return "qiskit"
	}
	return "python"
}

func looksLikeQASM(code string) bool {
	lower := strings.ToLower(code)
	return strings.Contains(lower, "openqasm") ||
		strings.Contains(lower, "qubit[") ||
		regexp.MustCompile(`(?m)^\s*(h|cx|measure)\s`).MatchString(lower)
}

func looksLikeQSharp(code string) bool {
	return strings.Contains(code, "operation ") ||
		strings.Contains(code, "namespace ") ||
		strings.Contains(code, "Microsoft.Quantum") ||
		strings.Contains(code, "Qubit[")
}
