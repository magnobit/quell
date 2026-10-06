// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

// hostFlow is the structured return analysis for a host block.
// It is reusable for a later loop: a loop body would join the body's flow
// with the path that never enters the body.
type hostFlow int

const (
	flowFall hostFlow = iota
	flowSometimes
	flowAlways
	flowBreak
	flowContinue
)

func joinFlow(a, b hostFlow) hostFlow {
	if a == b {
		return a
	}
	return flowSometimes
}

func seqFlow(prev, next hostFlow) hostFlow {
	if prev == flowAlways || prev == flowBreak || prev == flowContinue {
		return prev
	}
	if next == flowAlways {
		return flowAlways
	}
	if prev == flowSometimes || next == flowSometimes {
		return flowSometimes
	}
	return flowFall
}

func (st *fnState) checkStmts(stmts []parser.FnStmt, types map[string]string, blockLocal, mutable map[string]bool, fn *parser.FnDecl, inLoop bool) (hostFlow, []qerr.Diagnostic) {
	var diags []qerr.Diagnostic
	flow := flowFall
	for _, stmt := range stmts {
		if flow == flowAlways || flow == flowBreak || flow == flowContinue {
			col, end := stmt.Col, stmt.EndCol
			if col < 1 {
				col = 1
			}
			if end <= col {
				end = col + 1
			}
			diags = append(diags, diagWarn(qerr.CodeUnreachable, stmt.Line, col, end,
				"unreachable host statement",
				"Remove the statements after return."))
		}
		sf, ds := st.checkStmt(stmt, types, blockLocal, mutable, fn, inLoop)
		diags = append(diags, ds...)
		flow = seqFlow(flow, sf)
	}
	return flow, diags
}

func (st *fnState) checkStmt(stmt parser.FnStmt, types map[string]string, blockLocal, mutable map[string]bool, fn *parser.FnDecl, inLoop bool) (hostFlow, []qerr.Diagnostic) {
	switch stmt.Kind {
	case "assign":
		if stmt.Decl.Index != nil {
			return flowFall, st.checkIndexAssign(stmt, types, mutable, fn)
		}
		if mutable[stmt.Decl.Name] {
			typ, ds := st.infer(stmt.Decl.Expr, types, fn.Name, stmt.Line)
			if len(ds) > 0 {
				return flowFall, ds
			}
			if typ != types[stmt.Decl.Name] {
				return flowFall, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Col, stmt.Col+len(stmt.Decl.Name),
					fmt.Sprintf("type mismatch: var %s has type %s, expression has type %s", stmt.Decl.Name, types[stmt.Decl.Name], typ),
					"Assign a value of the declared type.")}
			}
			return flowFall, nil
		}
		if blockLocal[stmt.Decl.Name] || types[stmt.Decl.Name] != "" {
			return flowFall, []qerr.Diagnostic{diag(qerr.CodeImmutable, stmt.Line, stmt.Col, stmt.Col+len(stmt.Decl.Name),
				fmt.Sprintf("assignment to immutable local %q", stmt.Decl.Name),
				"Declare it with var if it must change. let and parameters stay immutable.")}
		}
		return flowFall, []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, stmt.Line, stmt.Col, stmt.Col+len(stmt.Decl.Name),
			fmt.Sprintf("unknown identifier %q", stmt.Decl.Name),
			"Declare it with let or var inside the function.")}
	case "let", "var":
		if st.params[stmt.Decl.Name] {
			return flowFall, []qerr.Diagnostic{clash(stmt.Decl)}
		}
		if blockLocal[stmt.Decl.Name] {
			return flowFall, []qerr.Diagnostic{diag(qerr.CodeDuplicate, stmt.Line, stmt.Decl.Col, stmt.Decl.Col+len(stmt.Decl.Name),
				fmt.Sprintf("duplicate local %q", stmt.Decl.Name),
				"A parameter or let with this name is already in this block.")}
		}
		typ, ds := st.infer(stmt.Decl.Expr, types, fn.Name, stmt.Line)
		if len(ds) > 0 {
			return flowFall, ds
		}
		if typ != stmt.Decl.Type {
			ds = append(ds, diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Decl.Col, stmt.Decl.EndCol,
				fmt.Sprintf("type mismatch: %s %s expects %s, expression has type %s", stmt.Kind, stmt.Decl.Name, stmt.Decl.Type, typ),
				fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", stmt.Decl.Type, typ)))
			return flowFall, ds
		}
		blockLocal[stmt.Decl.Name] = true
		types[stmt.Decl.Name] = stmt.Decl.Type
		if arr, ok := stmt.Decl.Expr.(*parser.ArrayExpr); ok {
			st.lengths[stmt.Decl.Name] = len(arr.Elems)
		}
		if stmt.Kind == "var" {
			mutable[stmt.Decl.Name] = true
		} else {
			mutable[stmt.Decl.Name] = false
		}
		return flowFall, nil
	case "return":
		typ, ds := st.infer(stmt.Expr, types, fn.Name, stmt.Line)
		if len(ds) > 0 {
			return flowAlways, ds
		}
		if typ != fn.Ret {
			col, end := stmt.Expr.Span()
			ds = append(ds, diag(qerr.CodeReturnType, stmt.Line, col, end,
				fmt.Sprintf("return type mismatch: function %s returns %s, expression has type %s", fn.Name, fn.Ret, typ),
				fmt.Sprintf("Convert the expression to %s, or change the declared return type.", fn.Ret)))
		}
		return flowAlways, ds
	case "if":
		return st.checkIfStmt(stmt.If, types, mutable, fn, inLoop)
	case "for", "while":
		return st.checkLoop(stmt.Loop, types, mutable, fn)
	case "break":
		if !inLoop {
			return flowFall, []qerr.Diagnostic{diag(qerr.CodeBreak, stmt.Line, stmt.Col, stmt.EndCol,
				"break is only valid inside a host for or while",
				"Use break in a host loop. It does not apply to FOR or WHILE.")}
		}
		return flowBreak, nil
	case "continue":
		if !inLoop {
			return flowFall, []qerr.Diagnostic{diag(qerr.CodeBreak, stmt.Line, stmt.Col, stmt.EndCol,
				"continue is only valid inside a host for or while",
				"Use continue in a host loop. It does not apply to FOR or WHILE.")}
		}
		return flowContinue, nil
	case "call":
		call, ok := stmt.Expr.(*parser.CallExpr)
		if !ok || (call.Name != "print" && call.Name != "println") {
			return flowFall, []qerr.Diagnostic{diag(qerr.CodeSyntax, stmt.Line, stmt.Col, stmt.EndCol,
				"only print and println may be bare host calls",
				"Use the call in a let or return, or write println(...).")}
		}
		_, ds := st.inferPrint(call, types, fn.Name, stmt.Line)
		return flowFall, ds
	default:
		return flowFall, nil
	}
}

func (st *fnState) checkIfStmt(n *parser.IfStmt, types map[string]string, mutable map[string]bool, fn *parser.FnDecl, inLoop bool) (hostFlow, []qerr.Diagnostic) {
	if n == nil {
		return flowFall, nil
	}
	var diags []qerr.Diagnostic
	ct, ds := st.infer(n.Cond, types, fn.Name, n.Line)
	diags = append(diags, ds...)
	if len(ds) == 0 && ct != parser.TypeBool {
		col, end := n.Cond.Span()
		diags = append(diags, diag(qerr.CodeIfCond, n.Line, col, end,
			fmt.Sprintf("host condition must be bool, got %s", ct),
			"Compare or convert the condition to bool. This is lowercase host if, not QPU IF."))
	}
	tf, tds := st.checkStmts(n.Then, copyTypes(types), map[string]bool{}, copyBool(mutable), fn, inLoop)
	diags = append(diags, tds...)
	ef := flowFall
	if n.Else != nil {
		var eds []qerr.Diagnostic
		ef, eds = st.checkStmts(n.Else, copyTypes(types), map[string]bool{}, copyBool(mutable), fn, inLoop)
		diags = append(diags, eds...)
	}
	return joinFlow(tf, ef), diags
}

func copyBool(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (st *fnState) checkLoop(n *parser.LoopStmt, types map[string]string, mutable map[string]bool, fn *parser.FnDecl) (hostFlow, []qerr.Diagnostic) {
	if n == nil {
		return flowFall, nil
	}
	var diags []qerr.Diagnostic
	mustEnter := false
	neverEnter := false
	if n.Kind == "while" {
		ct, ds := st.infer(n.Cond, types, fn.Name, n.Line)
		diags = append(diags, ds...)
		if len(ds) == 0 && ct != parser.TypeBool {
			col, end := n.Cond.Span()
			diags = append(diags, diag(qerr.CodeHostLoop, n.Line, col, end,
				fmt.Sprintf("host while condition must be bool, got %s", ct),
				"This is lowercase while, not QPU WHILE."))
		}
		if v, ok := constIntBool(n.Cond); ok {
			mustEnter = v
			neverEnter = !v
		}
	} else {
		ft, ds := st.infer(n.From, types, fn.Name, n.Line)
		diags = append(diags, ds...)
		tt, ds2 := st.infer(n.To, types, fn.Name, n.Line)
		diags = append(diags, ds2...)
		if len(ds) == 0 && ft != parser.TypeInt {
			col, end := n.From.Span()
			diags = append(diags, diag(qerr.CodeHostLoop, n.Line, col, end, "host for range must be int", "Write an inclusive int range."))
		}
		if len(ds2) == 0 && tt != parser.TypeInt {
			col, end := n.To.Span()
			diags = append(diags, diag(qerr.CodeHostLoop, n.Line, col, end, "host for range must be int", "Write an inclusive int range."))
		}
		if st.params[n.Name] {
			diags = append(diags, diag(qerr.CodeParamClash, n.Line, n.Col, n.Col+len(n.Name),
				fmt.Sprintf("PARAM %q and let %q cannot share a name", n.Name, n.Name),
				"Rename the loop index. It is not a PARAM."))
		}
		if from, to, ok := constRange(n.From, n.To); ok {
			mustEnter = from <= to
			neverEnter = from > to
		}
	}
	bodyTypes := copyTypes(types)
	bodyLocal := map[string]bool{}
	if n.Kind == "for" && n.Name != "" && !st.params[n.Name] {
		bodyTypes[n.Name] = parser.TypeInt
		bodyLocal[n.Name] = true
	}
	bf, bds := st.checkStmts(n.Body, bodyTypes, bodyLocal, copyBool(mutable), fn, true)
	diags = append(diags, bds...)
	if neverEnter && len(n.Body) > 0 {
		diags = append(diags, diagWarn(qerr.CodeUnreachable, n.Body[0].Line, 1, 2,
			"unreachable host statement",
			"This loop range or condition never runs."))
	}
	return loopResult(bf, mustEnter, neverEnter), diags
}

func loopResult(body hostFlow, mustEnter, neverEnter bool) hostFlow {
	if neverEnter {
		return flowFall
	}
	switch body {
	case flowAlways:
		if mustEnter {
			return flowAlways
		}
		return flowSometimes
	case flowSometimes:
		return flowSometimes
	default:
		return flowFall
	}
}

func constIntBool(e parser.Expr) (bool, bool) {
	lit, ok := e.(*parser.LiteralExpr)
	if !ok || lit.Type != parser.TypeBool {
		return false, false
	}
	return lit.Bool, true
}

func constRange(from, to parser.Expr) (int64, int64, bool) {
	a, ok := from.(*parser.LiteralExpr)
	if !ok || a.Type != parser.TypeInt {
		return 0, 0, false
	}
	b, ok := to.(*parser.LiteralExpr)
	if !ok || b.Type != parser.TypeInt {
		return 0, 0, false
	}
	return a.Int, b.Int, true
}

func constReachable(stmts []parser.FnStmt) []qerr.Diagnostic {
	var out []qerr.Diagnostic
	for _, st := range stmts {
		switch st.Kind {
		case "let":
			out = append(out, constProblems(st.Decl.Expr, st.Line)...)
		case "return":
			out = append(out, constProblems(st.Expr, st.Line)...)
		case "if":
			if st.If == nil {
				continue
			}
			v, ds := evalExpr(st.If.Cond, &evalEnv{locals: map[string]Value{}}, st.Line)
			for _, d := range ds {
				if d.Code == qerr.CodeDivZero || strings.Contains(d.Message, "overflow") {
					out = append(out, d)
				}
			}
			if len(ds) > 0 || v.Type != parser.TypeBool {
				continue
			}
			if v.Bool {
				out = append(out, constReachable(st.If.Then)...)
			} else {
				out = append(out, constReachable(st.If.Else)...)
			}
		case "for", "while":
			if st.Loop == nil {
				continue
			}
			if st.Loop.Kind == "while" {
				if v, ok := constIntBool(st.Loop.Cond); ok && v {
					out = append(out, constReachable(st.Loop.Body)...)
				}
				continue
			}
			if from, to, ok := constRange(st.Loop.From, st.Loop.To); ok && from <= to {
				out = append(out, constReachable(st.Loop.Body)...)
			}
		case "var":
			out = append(out, constProblems(st.Decl.Expr, st.Line)...)
		}
	}
	return out
}

const (
	execFall = iota
	execReturn
	execBreak
	execContinue
)

func lookupLocal(env *evalEnv, name string) (Value, bool) {
	if env != nil && env.locals != nil {
		if v, ok := env.locals[name]; ok {
			return v, true
		}
	}
	if env != nil && env.cells != nil {
		if p := env.cells[name]; p != nil {
			return *p, true
		}
	}
	return Value{}, false
}

func shareCells(src map[string]*Value) map[string]*Value {
	dst := make(map[string]*Value, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func childEnv(env *evalEnv) *evalEnv {
	if env.locals == nil {
		env.locals = map[string]Value{}
	}
	locals := make(map[string]Value, len(env.locals))
	for k, v := range env.locals {
		locals[k] = v
	}
	child := *env
	child.locals = locals
	child.cells = shareCells(env.cells)
	child.exprDepth = env.exprDepth + 1
	return &child
}

func execStmts(stmts []parser.FnStmt, env *evalEnv) (Value, int, []qerr.Diagnostic) {
	if env.locals == nil {
		env.locals = map[string]Value{}
	}
	if env.cells == nil {
		env.cells = map[string]*Value{}
	}
	for _, stmt := range stmts {
		switch stmt.Kind {
		case "let":
			v, ds := evalExpr(stmt.Decl.Expr, env, stmt.Line)
			if len(ds) > 0 {
				return Value{}, execFall, ds
			}
			if v.Type != stmt.Decl.Type {
				return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Decl.Col, stmt.Decl.EndCol,
					fmt.Sprintf("type mismatch: let %s expects %s, expression has type %s", stmt.Decl.Name, stmt.Decl.Type, v.Type),
					fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", stmt.Decl.Type, v.Type))}
			}
			env.locals[stmt.Decl.Name] = v
		case "var":
			v, ds := evalExpr(stmt.Decl.Expr, env, stmt.Line)
			if len(ds) > 0 {
				return Value{}, execFall, ds
			}
			if v.Type != stmt.Decl.Type {
				return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Decl.Col, stmt.Decl.EndCol,
					fmt.Sprintf("type mismatch: var %s expects %s, expression has type %s", stmt.Decl.Name, stmt.Decl.Type, v.Type),
					fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", stmt.Decl.Type, v.Type))}
			}
			cell := v
			env.cells[stmt.Decl.Name] = &cell
		case "assign":
			p := env.cells[stmt.Decl.Name]
			if p == nil {
				return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeImmutable, stmt.Line, stmt.Col, stmt.EndCol,
					fmt.Sprintf("assignment to immutable local %q", stmt.Decl.Name),
					"Declare it with var if it must change.")}
			}
			v, ds := evalExpr(stmt.Decl.Expr, env, stmt.Line)
			if len(ds) > 0 {
				return Value{}, execFall, ds
			}
			if stmt.Decl.Index != nil {
				return assignIndex(p, stmt, v, env)
			}
			*p = v
		case "return":
			v, ds := evalExpr(stmt.Expr, env, stmt.Line)
			if len(ds) > 0 {
				return Value{}, execFall, ds
			}
			return v, execReturn, nil
		case "if":
			v, sig, ds := execIfStmt(stmt.If, env)
			if len(ds) > 0 || sig != execFall {
				return v, sig, ds
			}
		case "for", "while":
			v, sig, ds := execLoop(stmt.Loop, env)
			if len(ds) > 0 || sig == execReturn {
				return v, sig, ds
			}
		case "break":
			return Value{}, execBreak, nil
		case "continue":
			return Value{}, execContinue, nil
		case "call":
			if _, ds := evalExpr(stmt.Expr, env, stmt.Line); len(ds) > 0 {
				return Value{}, execFall, ds
			}
		}
	}
	return Value{}, execFall, nil
}

func execIfStmt(n *parser.IfStmt, env *evalEnv) (Value, int, []qerr.Diagnostic) {
	if n == nil || env == nil {
		return Value{}, execFall, nil
	}
	if env.exprDepth >= parser.MaxExprDepth {
		return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeExprDepth, n.Line, n.Col, n.EndCol,
			"expression is nested more than 64 levels",
			"Split the conditional.")}
	}
	cond, ds := evalExpr(n.Cond, env, n.Line)
	if len(ds) > 0 {
		return Value{}, execFall, ds
	}
	if cond.Type != parser.TypeBool {
		col, end := n.Cond.Span()
		return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeIfCond, n.Line, col, end,
			fmt.Sprintf("host condition must be bool, got %s", cond.Type),
			"Compare or convert the condition to bool. This is lowercase host if, not QPU IF.")}
	}
	child := childEnv(env)
	if cond.Bool {
		return execStmts(n.Then, child)
	}
	return execStmts(n.Else, child)
}

func execLoop(n *parser.LoopStmt, env *evalEnv) (Value, int, []qerr.Diagnostic) {
	if n == nil {
		return Value{}, execFall, nil
	}
	if n.Kind == "while" {
		for i := 0; i < parser.MaxHostLoopIters; i++ {
			cond, ds := evalExpr(n.Cond, env, n.Line)
			if len(ds) > 0 {
				return Value{}, execFall, ds
			}
			if cond.Type != parser.TypeBool || !cond.Bool {
				return Value{}, execFall, nil
			}
			v, sig, ds := execStmts(n.Body, childEnv(env))
			if len(ds) > 0 {
				return Value{}, execFall, ds
			}
			if sig == execReturn {
				return v, execReturn, nil
			}
			if sig == execBreak {
				return Value{}, execFall, nil
			}
		}
		return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeLoopLimit, n.Line, n.Col, n.EndCol,
			fmt.Sprintf("host while exceeded %d iterations", parser.MaxHostLoopIters),
			"Tighten the condition. This is not QPU WHILE MAX.")}
	}
	from, ds := evalExpr(n.From, env, n.Line)
	if len(ds) > 0 {
		return Value{}, execFall, ds
	}
	to, ds := evalExpr(n.To, env, n.Line)
	if len(ds) > 0 {
		return Value{}, execFall, ds
	}
	if from.Type != parser.TypeInt || to.Type != parser.TypeInt {
		return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeHostLoop, n.Line, n.Col, n.EndCol,
			"host for range must be int", "Write an inclusive int range.")}
	}
	count := 0
	for i := from.Int; i <= to.Int; i++ {
		count++
		if count > parser.MaxHostLoopIters {
			return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeLoopLimit, n.Line, n.Col, n.EndCol,
				fmt.Sprintf("host for exceeded %d iterations", parser.MaxHostLoopIters),
				"Shrink the inclusive range.")}
		}
		child := childEnv(env)
		delete(child.cells, n.Name)
		child.locals[n.Name] = Value{Type: parser.TypeInt, Int: i}
		v, sig, ds := execStmts(n.Body, child)
		if len(ds) > 0 {
			return Value{}, execFall, ds
		}
		if sig == execReturn {
			return v, execReturn, nil
		}
		if sig == execBreak {
			return Value{}, execFall, nil
		}
	}
	return Value{}, execFall, nil
}
