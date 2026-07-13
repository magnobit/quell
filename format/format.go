// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package format is Quell's canonical source formatter ("quell fmt" /
// `gofmt`-equivalent).
package format

import "strings"

// Format returns src reformatted into Quell's canonical style:
//   - gate keywords uppercased (H, CNOT, MEASURE, ...); the "qubit"
//     declaration keyword stays lowercase, matching existing convention
//   - single-space token separation, comma-space-separated qubit name lists
//     ("qubit alice,bob" → "qubit alice, bob")
//   - trailing "//" comments aligned to a common column within each
//     contiguous block of statement lines (blank lines and comment-only
//     lines start a new block)
//   - at most one blank line between blocks, no leading/trailing blank lines
//   - exactly one trailing newline
//
// This works at the source level, not the parsed AST, deliberately:
// parser.Parse strips comments and normalizes angle expressions (PI/2 →
// 1.5707963267948966), so formatting off the AST would destroy both.
// Argument tokens (qubit names/indices, angle expressions) are never
// rewritten — only the leading keyword and inter-token whitespace are
// normalized — so a named qubit or an angle expression's exact spelling
// (e.g. "PI/2" vs "1.5708") always survives formatting unchanged.
//
// Format is not a validator: a line that doesn't look like a valid
// statement is left alone (whitespace-trimmed only), the same way gofmt
// still emits syntactically-invalid-but-parseable constructs without
// complaint — real validation is parser.Parse's job.
//
// Format is idempotent: Format(Format(src)) == Format(src).
func Format(src string) string {
	rawLines := strings.Split(src, "\n")

	type line struct {
		blank   bool
		comment bool   // comment-only line (rendered verbatim, trimmed)
		text    string // for blank/comment lines
		code    string // for statement lines
		trail   string // trailing "// ..." comment, or ""
	}

	lines := make([]line, 0, len(rawLines))
	for _, raw := range rawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			lines = append(lines, line{blank: true})
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			lines = append(lines, line{comment: true, text: trimmed})
			continue
		}

		code := trimmed
		trail := ""
		if ci := strings.Index(trimmed, "//"); ci >= 0 {
			code = strings.TrimSpace(trimmed[:ci])
			trail = strings.TrimSpace(trimmed[ci:])
		}
		if code == "" {
			// Whitespace before an inline comment with nothing else on the
			// line — treat like any other comment-only line.
			lines = append(lines, line{comment: true, text: trail})
			continue
		}

		lines = append(lines, line{code: formatStatement(code), trail: trail})
	}

	// Align trailing comments within each contiguous run of statement lines.
	var out []string
	i := 0
	for i < len(lines) {
		l := lines[i]
		switch {
		case l.blank:
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			i++
		case l.comment:
			out = append(out, l.text)
			i++
		default:
			j := i
			maxLen := 0
			for j < len(lines) && !lines[j].blank && !lines[j].comment {
				if lines[j].trail != "" && len(lines[j].code) > maxLen {
					maxLen = len(lines[j].code)
				}
				j++
			}
			for ; i < j; i++ {
				s := lines[i]
				if s.trail == "" {
					out = append(out, s.code)
					continue
				}
				out = append(out, s.code+strings.Repeat(" ", maxLen-len(s.code)+2)+s.trail)
			}
		}
	}

	// Trim leading/trailing blank lines.
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// formatStatement normalizes one statement's keyword and inter-token
// whitespace. Argument token content is never modified.
func formatStatement(code string) string {
	tokens := strings.Fields(code)
	if len(tokens) == 0 {
		return code
	}
	keyword := tokens[0]

	if strings.EqualFold(keyword, "qubit") {
		rest := strings.Join(tokens[1:], " ")
		names := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
		return "qubit " + strings.Join(names, ", ")
	}

	formatted := strings.ToUpper(keyword)
	if len(tokens) > 1 {
		formatted += " " + strings.Join(tokens[1:], " ")
	}
	return formatted
}
