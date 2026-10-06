// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func hasInterp(s string) bool {
	_, _, ok, _ := rewriteInterp(s)
	return ok
}

func (st *fnState) checkInterp(s string, types map[string]string, line, col int) []qerr.Diagnostic {
	spec, holes, ok, err := rewriteInterp(s)
	if err != nil {
		return []qerr.Diagnostic{diag(qerr.CodeInterp, line, col, col+len(s), err.Error(), "Write {name} or {name:.4f}. Use {{ for a literal brace.")}
	}
	if !ok {
		return nil
	}
	args := make([]Value, len(holes))
	for i, h := range holes {
		t := types[h]
		if t == "" {
			return []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, line, col, col+len(h), "unknown interpolation "+h, "Declare the name with let or as a parameter.")}
		}
		args[i] = zeroOf(t)
	}
	if _, fe := applyFormat(spec, args); fe != nil {
		return []qerr.Diagnostic{diag(fe.code, line, col, col+6, fe.msg, fe.fix)}
	}
	return nil
}

func evalInterp(s string, env *evalEnv, line, col int) (string, []qerr.Diagnostic) {
	spec, holes, ok, err := rewriteInterp(s)
	if err != nil {
		return "", []qerr.Diagnostic{diag(qerr.CodeInterp, line, col, col+len(s), err.Error(), "Write {name} or {name:.4f}.")}
	}
	if !ok {
		return "", nil
	}
	args := make([]Value, len(holes))
	for i, h := range holes {
		v, ds := evalExpr(&parser.IdentExpr{Name: h, Col: col, EndCol: col + len(h)}, env, line)
		if len(ds) > 0 {
			return "", ds
		}
		args[i] = v
	}
	out, fe := applyFormat(spec, args)
	if fe != nil {
		return "", []qerr.Diagnostic{diag(fe.code, line, col, col+6, fe.msg, fe.fix)}
	}
	return out, nil
}

// rewriteInterp turns {name} and {name:.4f} into the format() grammar.
// ok is false when the string has no interpolation holes.
func rewriteInterp(s string) (spec string, holes []string, ok bool, err error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			b.WriteString("{{")
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			return "", nil, false, errInterp("interpolation is missing }")
		}
		body := s[i+1 : i+end]
		name, fmtSpec, splitErr := splitHole(body)
		if splitErr != nil {
			return "", nil, false, splitErr
		}
		holes = append(holes, name)
		b.WriteByte('{')
		b.WriteString(fmtSpec)
		b.WriteByte('}')
		ok = true
		i += end
	}
	return b.String(), holes, ok, nil
}

func splitHole(body string) (name, spec string, err error) {
	if body == "" {
		return "", "", errInterp("empty interpolation")
	}
	colon := strings.IndexByte(body, ':')
	raw := body
	if colon >= 0 {
		raw = body[:colon]
		spec = body[colon:]
	}
	if !identHole(raw) {
		return "", "", errInterp("interpolation name must be an identifier")
	}
	return raw, spec, nil
}

func identHole(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 0 {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
			continue
		}
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

type interpErr string

func (e interpErr) Error() string { return string(e) }

func errInterp(msg string) error { return interpErr(msg) }
