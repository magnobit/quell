// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/magnobit/quell/internal/host"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func (st *fnState) inferPrint(n *parser.CallExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	if n.Name == "format" {
		if len(n.Args) < 1 {
			return "", []qerr.Diagnostic{diag(qerr.CodeFormatCount, line, n.Col, n.Col+len(n.Name),
				"format requires a format string",
				"Write format(\"{}\", value).")}
		}
		ft, ds := st.infer(n.Args[0], types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if ft != parser.TypeString {
			return "", []qerr.Diagnostic{diag(qerr.CodeFormatType, line, n.Col, n.Col+6,
				"format string must be string",
				"Pass a string literal as the first argument.")}
		}
		for _, a := range n.Args[1:] {
			if _, ds := st.infer(a, types, from, line); len(ds) > 0 {
				return "", ds
			}
		}
		if lit, ok := n.Args[0].(*parser.LiteralExpr); ok && lit.Type == parser.TypeString {
			if ds := checkFormatLiteral(lit.Str, n, types, st, from, line); len(ds) > 0 {
				return "", ds
			}
		}
		return parser.TypeString, nil
	}
	var diags []qerr.Diagnostic
	for _, a := range n.Args {
		if lit, ok := a.(*parser.LiteralExpr); ok && lit.Type == parser.TypeString {
			if ds := st.checkInterp(lit.Str, types, line, n.Col); len(ds) > 0 {
				diags = append(diags, ds...)
			}
			continue
		}
		if _, ds := st.infer(a, types, from, line); len(ds) > 0 {
			diags = append(diags, ds...)
		}
	}
	return "", diags
}

func checkFormatLiteral(spec string, n *parser.CallExpr, types map[string]string, st *fnState, from string, line int) []qerr.Diagnostic {
	var args []string
	for _, a := range n.Args[1:] {
		t, ds := st.infer(a, types, from, line)
		if len(ds) > 0 {
			return ds
		}
		args = append(args, t)
	}
	ai := 0
	for i := 0; i < len(spec); i++ {
		if spec[i] != '{' {
			continue
		}
		if i+1 < len(spec) && spec[i+1] == '{' {
			i++
			continue
		}
		end := strings.IndexByte(spec[i:], '}')
		if end < 0 {
			return []qerr.Diagnostic{diag(qerr.CodeFormat, line, n.Col, n.Col+6, "invalid format specifier", "Close the placeholder with }.")}
		}
		body := spec[i+1 : i+end]
		if ai >= len(args) {
			return []qerr.Diagnostic{diag(qerr.CodeFormatCount, line, n.Col, n.Col+6, "format placeholder has no argument", "Pass one argument for each placeholder.")}
		}
		if _, fe := formatOne(body, zeroOf(args[ai])); fe != nil {
			return []qerr.Diagnostic{diag(fe.code, line, n.Col, n.Col+6, fe.msg, fe.fix)}
		}
		ai++
		i += end
	}
	if ai != len(args) {
		return []qerr.Diagnostic{diag(qerr.CodeFormatCount, line, n.Col, n.Col+6, "format has unused arguments", "Remove the extra arguments or add placeholders.")}
	}
	return nil
}

func zeroOf(typ string) Value {
	return Value{Type: typ}
}

func evalPrint(n *parser.CallExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if n.Name == "format" {
		s, ds := evalFormat(n, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		return Value{Type: parser.TypeString, Str: s}, nil
	}
	parts := make([]string, 0, len(n.Args))
	for _, a := range n.Args {
		if lit, ok := a.(*parser.LiteralExpr); ok && lit.Type == parser.TypeString {
			text, ds := evalInterp(lit.Str, env, line, n.Col)
			if len(ds) > 0 {
				return Value{}, ds
			}
			if text != "" || hasInterp(lit.Str) {
				parts = append(parts, text)
				continue
			}
		}
		v, ds := evalExpr(a, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		parts = append(parts, display(v))
	}
	text := strings.Join(parts, " ")
	if n.Name == "println" {
		text += "\n"
	}
	host.WriteOut(text)
	return Value{}, nil
}

func evalFormat(n *parser.CallExpr, env *evalEnv, line int) (string, []qerr.Diagnostic) {
	if len(n.Args) < 1 {
		return "", []qerr.Diagnostic{diag(qerr.CodeFormatCount, line, n.Col, n.Col+6, "format requires a format string", "Write format(\"{}\", value).")}
	}
	fmtVal, ds := evalExpr(n.Args[0], env, line)
	if len(ds) > 0 {
		return "", ds
	}
	if fmtVal.Type != parser.TypeString {
		return "", []qerr.Diagnostic{diag(qerr.CodeFormatType, line, n.Col, n.Col+6, "format string must be string", "Pass a string literal as the first argument.")}
	}
	var args []Value
	for _, a := range n.Args[1:] {
		v, ds := evalExpr(a, env, line)
		if len(ds) > 0 {
			return "", ds
		}
		args = append(args, v)
	}
	out, err := applyFormat(fmtVal.Str, args)
	if err != nil {
		return "", []qerr.Diagnostic{diag(err.code, line, n.Col, n.Col+6, err.msg, err.fix)}
	}
	return out, nil
}

type fmtErr struct {
	code, msg, fix string
}

func applyFormat(spec string, args []Value) (string, *fmtErr) {
	var b strings.Builder
	ai := 0
	for i := 0; i < len(spec); i++ {
		if spec[i] != '{' {
			b.WriteByte(spec[i])
			continue
		}
		if i+1 < len(spec) && spec[i+1] == '{' {
			b.WriteByte('{')
			i++
			continue
		}
		end := strings.IndexByte(spec[i:], '}')
		if end < 0 {
			return "", &fmtErr{qerr.CodeFormat, "invalid format specifier", "Close the placeholder with }."}
		}
		body := spec[i+1 : i+end]
		if ai >= len(args) {
			return "", &fmtErr{qerr.CodeFormatCount, "format placeholder has no argument", "Pass one argument for each placeholder."}
		}
		piece, fe := formatOne(body, args[ai])
		if fe != nil {
			return "", fe
		}
		b.WriteString(piece)
		ai++
		i += end
	}
	if ai != len(args) {
		return "", &fmtErr{qerr.CodeFormatCount, "format has unused arguments", "Remove the extra arguments or add placeholders."}
	}
	return b.String(), nil
}

func formatOne(body string, v Value) (string, *fmtErr) {
	switch body {
	case "", "s":
		return display(v), nil
	case "d":
		if v.Type != parser.TypeInt {
			return "", &fmtErr{qerr.CodeFormatType, "format {:d} requires int", "Pass an int or change the specifier."}
		}
		return strconv.FormatInt(v.Int, 10), nil
	case "b":
		if v.Type != parser.TypeBool {
			return "", &fmtErr{qerr.CodeFormatType, "format {:b} requires bool", "Pass a bool or change the specifier."}
		}
		if v.Bool {
			return "true", nil
		}
		return "false", nil
	}
	if strings.HasPrefix(body, ":.") && strings.HasSuffix(body, "f") {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(body, ":."), "f"))
		if err != nil || n < 0 || n > 12 {
			return "", &fmtErr{qerr.CodeFormat, "invalid float format", "Use {:.4f} with precision from 0 to 12."}
		}
		if v.Type != parser.TypeFloat && v.Type != parser.TypeInt {
			return "", &fmtErr{qerr.CodeFormatType, "float format requires float", "Pass a float."}
		}
		f := v.Float
		if v.Type == parser.TypeInt {
			f = float64(v.Int)
		}
		return strconv.FormatFloat(f, 'f', n, 64), nil
	}
	if body == ":d" {
		return formatOne("d", v)
	}
	if body == ":s" {
		return formatOne("s", v)
	}
	if body == ":b" {
		return formatOne("b", v)
	}
	return "", &fmtErr{qerr.CodeFormat, "invalid format specifier {:" + body + "}", "Use {}, {:d}, {:b}, {:s}, or {:.4f}."}
}

func display(v Value) string {
	switch v.Type {
	case parser.TypeBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case parser.TypeInt:
		return strconv.FormatInt(v.Int, 10)
	case parser.TypeFloat:
		return strconv.FormatFloat(v.Float, 'g', -1, 64)
	case parser.TypeString:
		return v.Str
	default:
		if _, ok := parser.ElemType(v.Type); ok {
			parts := make([]string, len(v.Elems))
			for i, el := range v.Elems {
				parts[i] = display(el)
			}
			return "[" + strings.Join(parts, ", ") + "]"
		}
		return fmt.Sprintf("<%s>", v.Type)
	}
}
