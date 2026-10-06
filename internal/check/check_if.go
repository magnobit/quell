// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"fmt"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func hasIf(e parser.Expr) bool {
	found := false
	parser.WalkIf(e, func(*parser.IfExpr) { found = true })
	return found
}

func (st *fnState) inferIf(n *parser.IfExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	if n == nil {
		return "", []qerr.Diagnostic{diag(qerr.CodeSyntax, line, 1, 2, "missing host if", "Write if condition { expr } else { expr }.")}
	}
	if n.Else == nil || n.Then == nil {
		return "", []qerr.Diagnostic{diag(qerr.CodeIfElse, n.Line, n.Col, n.EndCol,
			"conditional expression requires else",
			"Add else { expr }. Both branches are required.")}
	}
	ct, ds := st.infer(n.Cond, types, from, n.Line)
	if len(ds) > 0 {
		return "", ds
	}
	if ct != parser.TypeBool {
		col, end := n.Cond.Span()
		return "", []qerr.Diagnostic{diag(qerr.CodeIfCond, n.Line, col, end,
			fmt.Sprintf("host condition must be bool, got %s", ct),
			"Compare or convert the condition to bool. This is lowercase host if, not QPU IF.")}
	}
	tt, ds := st.inferBlock(n.Then, types, from)
	if len(ds) > 0 {
		return "", ds
	}
	et, eds := st.inferBlock(n.Else, types, from)
	if len(eds) > 0 {
		return "", eds
	}
	if tt != et {
		el := n.Else.ResultLine
		if el == 0 {
			el = n.Line
		}
		return "", []qerr.Diagnostic{diag(qerr.CodeIfType, el, 1, 2,
			fmt.Sprintf("conditional branches have different types: %s and %s", tt, et),
			"Convert one branch explicitly so both types match.")}
	}
	if st.markLine == n.Line {
		st.marked = tt
		st.markedOK = true
	}
	return tt, nil
}

func (st *fnState) inferBlock(b *parser.BlockExpr, outer map[string]string, from string) (string, []qerr.Diagnostic) {
	if b == nil || b.Result == nil {
		line := 1
		if b != nil && b.Line > 0 {
			line = b.Line
		}
		return "", []qerr.Diagnostic{diag(qerr.CodeIfBlock, line, 1, 2,
			"invalid host block value",
			"End the branch with one expression.")}
	}
	types := copyTypes(outer)
	seen := map[string]bool{}
	var diags []qerr.Diagnostic
	for _, d := range b.Lets {
		if st.params[d.Name] {
			diags = append(diags, clash(d))
			continue
		}
		if seen[d.Name] {
			diags = append(diags, diag(qerr.CodeDuplicate, d.Line, d.Col, d.Col+len(d.Name),
				fmt.Sprintf("duplicate local %q", d.Name),
				"A block may declare each name once. This let does not escape the branch."))
			continue
		}
		seen[d.Name] = true
		typ, ds := st.infer(d.Expr, types, from, d.Line)
		diags = append(diags, ds...)
		if len(ds) > 0 {
			continue
		}
		if typ != d.Type {
			diags = append(diags, diag(qerr.CodeTypeMismatch, d.Line, d.Col, d.EndCol,
				fmt.Sprintf("type mismatch: let %s expects %s, expression has type %s", d.Name, d.Type, typ),
				fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", d.Type, typ)))
			continue
		}
		types[d.Name] = d.Type
	}
	if len(diags) > 0 {
		return "", diags
	}
	rl := b.ResultLine
	if rl == 0 {
		rl = b.Line
	}
	return st.infer(b.Result, types, from, rl)
}

func copyTypes(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func evalIf(n *parser.IfExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if n == nil || n.Else == nil || n.Then == nil {
		ln := line
		if n != nil && n.Line > 0 {
			ln = n.Line
		}
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeIfElse, ln, 1, 2,
			"conditional expression requires else",
			"Add else { expr }. Both branches are required.")}
	}
	if env.exprDepth >= parser.MaxExprDepth {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeExprDepth, n.Line, n.Col, n.EndCol,
			"expression is nested more than 64 levels",
			"Split the conditional.")}
	}
	cond, ds := evalExpr(n.Cond, env, n.Line)
	if len(ds) > 0 {
		return Value{}, ds
	}
	if cond.Type != parser.TypeBool {
		col, end := n.Cond.Span()
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeIfCond, n.Line, col, end,
			fmt.Sprintf("host condition must be bool, got %s", cond.Type),
			"Compare or convert the condition to bool. This is lowercase host if, not QPU IF.")}
	}
	child := *env
	child.exprDepth = env.exprDepth + 1
	if cond.Bool {
		return evalBlock(n.Then, &child, n.Line)
	}
	return evalBlock(n.Else, &child, n.Line)
}

func evalBlock(b *parser.BlockExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if b == nil || b.Result == nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeIfBlock, line, 1, 2,
			"invalid host block value",
			"End the branch with one expression.")}
	}
	locals := make(map[string]Value, len(env.locals)+len(b.Lets))
	for k, v := range env.locals {
		locals[k] = v
	}
	child := *env
	child.locals = locals
	for _, d := range b.Lets {
		v, ds := evalExpr(d.Expr, &child, d.Line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		if v.Type != d.Type {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, d.Line, d.Col, d.EndCol,
				fmt.Sprintf("type mismatch: let %s expects %s, expression has type %s", d.Name, d.Type, v.Type),
				fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", d.Type, v.Type))}
		}
		child.locals[d.Name] = v
	}
	rl := b.ResultLine
	if rl == 0 {
		rl = line
	}
	return evalExpr(b.Result, &child, rl)
}

// IfTypeAt reports the result type of the host if whose keyword is on line.
// Both branches are typechecked. A failing conditional returns false.
func IfTypeAt(c *parser.Circuit, line int) (string, bool) {
	if c == nil || line < 1 {
		return "", false
	}
	st := newFnState(c)
	st.markLine = line
	_ = st.signatures()
	_ = st.bodies()
	types := map[string]string{}
	for _, h := range c.Host {
		if h.Kind != "let" {
			continue
		}
		if hasIf(h.Expr) {
			_, _ = st.infer(h.Expr, types, "", h.Line)
		}
		types[h.Name] = h.Type
	}
	if st.markedOK {
		return st.marked, true
	}
	return "", false
}
