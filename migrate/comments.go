// Copyright 2026 Magnobit, Inc. All rights reserved.

package migrate

import (
	"strings"
	"unicode"
)

// stripSourceComments removes language comments before regex converters scan
// the source — otherwise Quell-style // or Python # examples inside comments
// are treated as live gates.
func stripSourceComments(code, language string) string {
	lang := strings.ToLower(strings.TrimSpace(language))
	switch lang {
	case "qiskit", "python", "cirq", "braket":
		return stripHashAndDocComments(code)
	case "qsharp", "q#", "qs":
		return stripCStyleLineComments(code)
	case "openqasm", "qasm", "qasm2", "qasm3", "openqasm2", "openqasm3":
		return stripQASMComments(code)
	default:
		// Auto / unknown: strip both # and // line comments + block comments.
		return stripQASMComments(stripHashLineComments(code))
	}
}

func stripHashAndDocComments(code string) string {
	code = stripPythonDocstrings(code)
	return stripHashLineComments(code)
}

func stripHashLineComments(code string) string {
	var b strings.Builder
	for _, line := range strings.Split(code, "\n") {
		inStr := false
		escaped := false
		cut := -1
		for i, r := range line {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && inStr {
				escaped = true
				continue
			}
			if r == '"' || r == '\'' {
				inStr = !inStr
				continue
			}
			if !inStr && r == '#' {
				cut = i
				break
			}
		}
		if cut >= 0 {
			line = line[:cut]
		}
		b.WriteString(strings.TrimRightFunc(line, unicode.IsSpace))
		b.WriteByte('\n')
	}
	return b.String()
}

func stripPythonDocstrings(code string) string {
	// Remove """…""" and '''…''' blocks (simple scan; good enough for migrate samples).
	var b strings.Builder
	i := 0
	for i < len(code) {
		if i+2 < len(code) && ((code[i] == '"' && code[i+1] == '"' && code[i+2] == '"') ||
			(code[i] == '\'' && code[i+1] == '\'' && code[i+2] == '\'')) {
			quote := code[i]
			i += 3
			for i+2 < len(code) {
				if code[i] == quote && code[i+1] == quote && code[i+2] == quote {
					i += 3
					break
				}
				i++
			}
			continue
		}
		b.WriteByte(code[i])
		i++
	}
	return b.String()
}

func stripCStyleLineComments(code string) string {
	var b strings.Builder
	i := 0
	for i < len(code) {
		if i+1 < len(code) && code[i] == '/' && code[i+1] == '/' {
			for i < len(code) && code[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(code) && code[i] == '/' && code[i+1] == '*' {
			i += 2
			for i+1 < len(code) && !(code[i] == '*' && code[i+1] == '/') {
				i++
			}
			if i+1 < len(code) {
				i += 2
			}
			continue
		}
		b.WriteByte(code[i])
		i++
	}
	return b.String()
}

func stripQASMComments(code string) string {
	return stripCStyleLineComments(code)
}
