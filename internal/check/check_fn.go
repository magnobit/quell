// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

// evalEnv is the sandbox for host evaluation. It has no filesystem, network,
// shell, or dynamic library access. Call depth is bounded.
type evalEnv struct {
	locals  map[string]Value
	params  map[string]bool
	sigs    map[string]*parser.FnDecl
	stack   []string
	depth   int
	globals map[string]Value
	host    []parser.HostDecl
	// exprDepth counts host if nesting during evaluation.
	exprDepth int
	// cells holds mutable var bindings. Pointers are shared across branches.
	cells map[string]*Value
	circ  *parser.Circuit
}

type callEdge struct {
	from, to       string
	line, col, end int
}

type fnState struct {
	c      *parser.Circuit
	params map[string]bool
	sigs   map[string]*parser.FnDecl
	order  []string
	edges  []callEdge
	values map[string]Value
	// markLine asks infer to record the result type of a host if on that line.
	markLine int
	marked   string
	markedOK bool
	lengths  map[string]int
}

func newFnState(c *parser.Circuit) *fnState {
	params := map[string]bool{}
	if c != nil {
		for _, name := range c.Params {
			params[name] = true
		}
	}
	return &fnState{
		c:       c,
		params:  params,
		sigs:    map[string]*parser.FnDecl{},
		values:  map[string]Value{},
		lengths: map[string]int{},
	}
}

// Signatures is pass 1 of checking: function names, parameters, and collisions.
// Bodies are not checked. The returned names are the functions that were kept.
func Signatures(c *parser.Circuit) ([]string, []qerr.Diagnostic) {
	if c == nil {
		return nil, nil
	}
	st := newFnState(c)
	ds := st.signatures()
	return append([]string(nil), st.order...), ds
}

// EvalIn evaluates e using c's functions and file-level lets.
// It runs the same checks as Check first.
func EvalIn(c *parser.Circuit, e parser.Expr) (Value, []qerr.Diagnostic) {
	if c == nil || e == nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeSyntax, 1, 1, 2, "missing expression", "Write a host expression.")}
	}
	st := newFnState(c)
	ds := st.signatures()
	ds = append(ds, st.bodies()...)
	ds = append(ds, st.recursion()...)
	ds = append(ds, st.globals()...)
	if len(ds) > 0 {
		return Value{}, ds
	}
	env := &evalEnv{
		locals:  st.values,
		params:  st.params,
		sigs:    st.sigs,
		globals: st.values,
		host:    c.Host,
		circ:    c,
	}
	return evalExpr(e, env, 1)
}

func (st *fnState) signatures() []qerr.Diagnostic {
	lets := map[string]bool{}
	for _, h := range st.c.Host {
		if h.Kind == "let" {
			lets[h.Name] = true
		}
	}
	macros := map[string]string{}
	for _, m := range st.c.Macros {
		macros[strings.ToUpper(m.Name)] = m.Name
	}
	var diags []qerr.Diagnostic
	for i := range st.c.Functions {
		fn := &st.c.Functions[i]
		if prev, ok := st.sigs[fn.Name]; ok {
			diags = append(diags, diag(qerr.CodeDuplicateFunc, fn.Line, fn.Col, fn.Col+len(fn.Name),
				fmt.Sprintf("duplicate function %q", fn.Name),
				fmt.Sprintf("Function %q is already declared at line %d. Remove one of them.", fn.Name, prev.Line)))
			continue
		}
		if lets[fn.Name] {
			diags = append(diags, diag(qerr.CodeDuplicate, fn.Line, fn.Col, fn.Col+len(fn.Name),
				fmt.Sprintf("duplicate name %q in file scope", fn.Name),
				"A function and a let cannot share a name. Rename one of them."))
		}
		if st.params[fn.Name] {
			diags = append(diags, diag(qerr.CodeParamClash, fn.Line, fn.Col, fn.Col+len(fn.Name),
				fmt.Sprintf("PARAM %q and function %q cannot share a name", fn.Name, fn.Name),
				"Rename the function or the PARAM."))
		}
		upper := strings.ToUpper(fn.Name)
		if _, ok := macros[upper]; ok {
			diags = append(diags, diag(qerr.CodeFuncGate, fn.Line, fn.Col, fn.Col+len(fn.Name),
				fmt.Sprintf("function %q collides with gate macro %s", fn.Name, upper),
				"Rename the function or the gate macro. They are different declarations."))
		} else if _, ok := parser.GateArity[upper]; ok {
			diags = append(diags, diag(qerr.CodeFuncGate, fn.Line, fn.Col, fn.Col+len(fn.Name),
				fmt.Sprintf("function %q collides with built-in gate %s", fn.Name, upper),
				"Rename the function. Gate names stay compile-time quantum operations."))
		}
		seen := map[string]bool{}
		for _, p := range fn.Params {
			if seen[p.Name] {
				diags = append(diags, diag(qerr.CodeDupParam, fn.Line, p.Col, p.EndCol,
					fmt.Sprintf("duplicate parameter %q", p.Name),
					"Remove the repeated parameter."))
				continue
			}
			seen[p.Name] = true
			if st.params[p.Name] {
				diags = append(diags, diag(qerr.CodeParamClash, fn.Line, p.Col, p.EndCol,
					fmt.Sprintf("PARAM %q and parameter %q cannot share a name", p.Name, p.Name),
					"Rename the function parameter or the PARAM."))
			}
		}
		st.sigs[fn.Name] = fn
		st.order = append(st.order, fn.Name)
	}
	return diags
}

func (st *fnState) bodies() []qerr.Diagnostic {
	var diags []qerr.Diagnostic
	for _, name := range st.order {
		diags = append(diags, st.checkBody(st.sigs[name])...)
	}
	return diags
}

func (st *fnState) checkBody(fn *parser.FnDecl) []qerr.Diagnostic {
	var diags []qerr.Diagnostic
	types := map[string]string{}
	for _, h := range st.c.Host {
		if h.Kind == "let" && h.Line < fn.Line {
			types[h.Name] = h.Type
		}
	}
	local := map[string]bool{}
	for _, p := range fn.Params {
		types[p.Name] = p.Type
		local[p.Name] = true
	}
	flow, diags := st.checkStmts(fn.Body, types, local, map[string]bool{}, fn, false)
	diags = append(diags, constReachable(fn.Body)...)
	switch flow {
	case flowAlways:
	case flowSometimes:
		diags = append(diags, diag(qerr.CodeIfPaths, fn.Line, fn.Col, fn.Col+len(fn.Name),
			fmt.Sprintf("not all host paths return in %q", fn.Name),
			"Return on every path. Add else { return ... }, or a return after the if."))
	default:
		diags = append(diags, diag(qerr.CodeMissingReturn, fn.Line, fn.Col, fn.Col+len(fn.Name),
			fmt.Sprintf("function %q is missing a return", fn.Name),
			"Add one return of the declared type."))
	}
	return diags
}

func (st *fnState) infer(e parser.Expr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	if e == nil {
		return "", []qerr.Diagnostic{diag(qerr.CodeSyntax, line, 1, 2, "missing expression", "Write an expression.")}
	}
	switch n := e.(type) {
	case *parser.LiteralExpr:
		return n.Type, nil
	case *parser.IdentExpr:
		if t, ok := types[n.Name]; ok {
			return t, nil
		}
		col, end := n.Span()
		if st.params[n.Name] {
			return "", []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, line, col, end,
				fmt.Sprintf("unknown identifier %q; PARAM %q is a circuit parameter, not a host local", n.Name, n.Name),
				"A host function cannot read a PARAM. Bind the PARAM on a gate angle.")}
		}
		return "", []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, line, col, end,
			fmt.Sprintf("unknown identifier %q", n.Name),
			"Declare it with let before this use, or pass it as a parameter.")}
	case *parser.UnaryExpr:
		t, ds := st.infer(n.X, types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		col, end := n.Span()
		switch n.Op {
		case "!":
			if t != parser.TypeBool {
				return "", []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, fmt.Sprintf("operator ! is not valid for %s", t), "Use ! only on bool.")}
			}
			return parser.TypeBool, nil
		case "-":
			if t != parser.TypeInt && t != parser.TypeFloat {
				return "", []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, fmt.Sprintf("operator - is not valid for %s", t), "Negate an int or a float.")}
			}
			return t, nil
		default:
			return "", []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "unknown unary operator "+n.Op, "")}
		}
	case *parser.BinaryExpr:
		lt, ds := st.infer(n.L, types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		rt, ds := st.infer(n.R, types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		return inferBinary(n, lt, rt, line)
	case *parser.ConvertExpr:
		t, ds := st.infer(n.X, types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if !conversionOK(n.To, t) {
			col, end := n.Span()
			return "", []qerr.Diagnostic{diag(qerr.CodeBadConvert, line, col, end,
				fmt.Sprintf("cannot convert %s to %s", t, n.To),
				"Use a conversion listed for this type.")}
		}
		return n.To, nil
	case *parser.CallExpr:
		return st.inferCall(n, types, from, line)
	case *parser.ArrayExpr:
		return st.inferArray(n, types, from, line)
	case *parser.IndexExpr:
		return st.inferIndex(n, types, from, line)
	case *parser.IfExpr:
		return st.inferIf(n, types, from, line)
	default:
		return "", []qerr.Diagnostic{diag(qerr.CodeSyntax, line, 1, 2, "unsupported expression", "")}
	}
}

func (st *fnState) inferCall(n *parser.CallExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	if n.Name == "print" || n.Name == "println" {
		return "", []qerr.Diagnostic{diag(qerr.CodePrintValue, line, n.Col, n.Col+len(n.Name),
			n.Name+" does not yield a value",
			"Use it as a statement, or use format(...) when you need a string.")}
	}
	if n.Name == "format" {
		return st.inferPrint(n, types, from, line)
	}
	if n.Name == "len" {
		return st.inferLen(n, types, from, line)
	}
	if n.Name == "expectation" {
		return st.inferExpectation(n, line)
	}
	if isTaskCall(n.Name) && st.sigs[n.Name] == nil {
		return st.inferTask(n, types, from, line)
	}
	col := n.Col
	nameEnd := col + len(n.Name)
	fn := st.sigs[n.Name]
	if fn == nil {
		return "", []qerr.Diagnostic{diag(qerr.CodeUnknownFunc, line, col, nameEnd,
			fmt.Sprintf("unknown function %q", n.Name),
			"Declare fn "+n.Name+" in this file. Forward calls in the same file are allowed.")}
	}
	st.edges = append(st.edges, callEdge{from: from, to: n.Name, line: line, col: col, end: nameEnd})
	if len(n.Args) != len(fn.Params) {
		_, end := n.Span()
		return "", []qerr.Diagnostic{diag(qerr.CodeArgCount, line, col, end,
			fmt.Sprintf("function %s expects %d argument(s), got %d", fn.Name, len(fn.Params), len(n.Args)),
			fmt.Sprintf("Pass %d argument(s).", len(fn.Params)))}
	}
	for i, a := range n.Args {
		t, ds := st.infer(a, types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if t != fn.Params[i].Type {
			acol, aend := a.Span()
			return "", []qerr.Diagnostic{diag(qerr.CodeArgType, line, acol, aend,
				fmt.Sprintf("argument %d of %s expects %s, expression has type %s", i+1, fn.Name, fn.Params[i].Type, t),
				fmt.Sprintf("Convert the argument with %s(...).", fn.Params[i].Type))}
		}
	}
	return fn.Ret, nil
}

func inferBinary(n *parser.BinaryExpr, lt, rt string, line int) (string, []qerr.Diagnostic) {
	col, end := n.Span()
	bad := func(msg, fix string) (string, []qerr.Diagnostic) {
		return "", []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, msg, fix)}
	}
	switch n.Op {
	case "&&", "||":
		if lt != parser.TypeBool || rt != parser.TypeBool {
			return bad(fmt.Sprintf("operator %s requires bool operands", n.Op), "Use && and || only on bool.")
		}
		return parser.TypeBool, nil
	case "==", "!=":
		if lt != rt {
			return bad(fmt.Sprintf("operator %s requires two values of the same type", n.Op), "Convert one side explicitly so both types match.")
		}
		return parser.TypeBool, nil
	case "<", "<=", ">", ">=":
		if lt != rt || (lt != parser.TypeInt && lt != parser.TypeFloat) {
			return bad(fmt.Sprintf("operator %s requires two ints or two floats", n.Op), "Compare ints with ints, or floats with floats.")
		}
		return parser.TypeBool, nil
	case "+", "-", "*", "/", "%":
		if lt != rt || (lt != parser.TypeInt && lt != parser.TypeFloat) {
			return bad(fmt.Sprintf("operator %s requires two ints or two floats, not %s and %s", n.Op, lt, rt), "Convert one side explicitly.")
		}
		if n.Op == "%" && lt != parser.TypeInt {
			return bad("operator % is only valid for int", "Use % on integers.")
		}
		return lt, nil
	default:
		return bad("unknown operator "+n.Op, "")
	}
}

func conversionOK(to, from string) bool {
	switch to {
	case parser.TypeBool:
		return from == parser.TypeBool || from == parser.TypeInt || from == parser.TypeFloat
	case parser.TypeInt, parser.TypeFloat, parser.TypeString:
		return from == parser.TypeBool || from == parser.TypeInt || from == parser.TypeFloat || from == parser.TypeString
	default:
		return false
	}
}

func constProblems(e parser.Expr, line int) []qerr.Diagnostic {
	_, ds := evalExpr(e, &evalEnv{locals: map[string]Value{}}, line)
	var out []qerr.Diagnostic
	for _, d := range ds {
		if d.Code == qerr.CodeDivZero || strings.Contains(d.Message, "overflow") {
			out = append(out, d)
		}
	}
	return out
}

func (st *fnState) recursion() []qerr.Diagnostic {
	by := map[string][]callEdge{}
	var callers []string
	seen := map[string]bool{}
	for _, e := range st.edges {
		if !seen[e.from] {
			seen[e.from] = true
			callers = append(callers, e.from)
		}
		by[e.from] = append(by[e.from], e)
	}
	color := map[string]int{}
	var diags []qerr.Diagnostic
	var walk func(string)
	walk = func(name string) {
		color[name] = 1
		for _, e := range by[name] {
			switch color[e.to] {
			case 1:
				diags = append(diags, diag(qerr.CodeRecursion, e.line, e.col, e.end,
					fmt.Sprintf("recursion is not supported: %s calls %s", e.from, e.to),
					"Rewrite the functions so no function calls itself, directly or through another function."))
			case 0:
				walk(e.to)
			}
		}
		color[name] = 2
	}
	for _, name := range callers {
		if color[name] == 0 {
			walk(name)
		}
	}
	return diags
}

func (st *fnState) globals() []qerr.Diagnostic {
	var diags []qerr.Diagnostic
	env := &evalEnv{
		locals:  map[string]Value{},
		params:  st.params,
		sigs:    st.sigs,
		globals: map[string]Value{},
		host:    st.c.Host,
		circ:    st.c,
	}
	for _, h := range st.c.Host {
		switch h.Kind {
		case "assign":
			diags = append(diags, assignDiag(h, env.locals))
		case "let":
			if st.params[h.Name] {
				diags = append(diags, clash(h))
				continue
			}
			if _, ok := st.sigs[h.Name]; ok {
				diags = append(diags, diag(qerr.CodeDuplicate, h.Line, h.Col, h.Col+len(h.Name),
					fmt.Sprintf("duplicate name %q in file scope", h.Name),
					"A let and a function cannot share a name. Rename one of them."))
				continue
			}
			if _, ok := env.locals[h.Name]; ok {
				diags = append(diags, diag(qerr.CodeDuplicate, h.Line, h.Col, h.Col+len(h.Name),
					fmt.Sprintf("duplicate local %q", h.Name),
					"Remove the second let or give it another name."))
				continue
			}
			if hasIf(h.Expr) {
				types := map[string]string{}
				for name, v := range env.locals {
					types[name] = v.Type
				}
				typ, ids := st.infer(h.Expr, types, "", h.Line)
				diags = append(diags, ids...)
				if len(ids) > 0 {
					continue
				}
				if typ != h.Type {
					diags = append(diags, diag(qerr.CodeTypeMismatch, h.Line, h.Col, h.EndCol,
						fmt.Sprintf("type mismatch: let %s expects %s, expression has type %s", h.Name, h.Type, typ),
						fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", h.Type, typ)))
					continue
				}
			}
			v, ds := evalExpr(h.Expr, env, h.Line)
			diags = append(diags, ds...)
			if len(ds) > 0 {
				continue
			}
			if v.Type != h.Type && !hasIf(h.Expr) {
				diags = append(diags, diag(qerr.CodeTypeMismatch, h.Line, h.Col, h.EndCol,
					fmt.Sprintf("type mismatch: let %s expects %s, expression has type %s", h.Name, h.Type, v.Type),
					fmt.Sprintf("Convert explicitly with %s(...), or change the declared type to %s.", h.Type, v.Type)))
				continue
			}
			env.locals[h.Name] = v
			env.globals[h.Name] = v
			st.values[h.Name] = v
		}
	}
	return diags
}

func evalCall(n *parser.CallExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if env == nil {
		env = &evalEnv{}
	}
	if n.Name == "print" || n.Name == "println" || n.Name == "format" {
		return evalPrint(n, env, line)
	}
	if n.Name == "len" {
		return evalLen(n, env, line)
	}
	if n.Name == "expectation" {
		return evalExpectation(n, env, line)
	}
	if isTaskCall(n.Name) && (env.sigs == nil || env.sigs[n.Name] == nil) {
		return evalTask(n, env, line)
	}
	col := n.Col
	if col < 1 {
		col = 1
	}
	nameEnd := col + len(n.Name)
	if env.sigs == nil || env.sigs[n.Name] == nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeUnknownFunc, line, col, nameEnd,
			fmt.Sprintf("unknown function %q", n.Name),
			"Declare fn "+n.Name+" in this file.")}
	}
	fn := env.sigs[n.Name]
	if env.depth >= parser.MaxCallDepth {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeLimit, line, col, nameEnd,
			fmt.Sprintf("host call depth exceeds %d", parser.MaxCallDepth),
			"Reduce the nesting of host calls.")}
	}
	for _, s := range env.stack {
		if s == fn.Name {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeRecursion, line, col, nameEnd,
				fmt.Sprintf("recursion is not supported: %s calls %s", s, fn.Name),
				"Remove the recursive call.")}
		}
	}
	if len(n.Args) != len(fn.Params) {
		_, end := n.Span()
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeArgCount, line, col, end,
			fmt.Sprintf("function %s expects %d argument(s), got %d", fn.Name, len(fn.Params), len(n.Args)),
			fmt.Sprintf("Pass %d argument(s).", len(fn.Params)))}
	}
	args := make([]Value, len(n.Args))
	for i, a := range n.Args {
		v, ds := evalExpr(a, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		if v.Type != fn.Params[i].Type {
			acol, aend := a.Span()
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeArgType, line, acol, aend,
				fmt.Sprintf("argument %d of %s expects %s, expression has type %s", i+1, fn.Name, fn.Params[i].Type, v.Type),
				fmt.Sprintf("Convert the argument with %s(...).", fn.Params[i].Type))}
		}
		args[i] = v
	}
	inner := map[string]Value{}
	for _, h := range env.host {
		if h.Kind == "let" && h.Line < fn.Line {
			if v, ok := env.globals[h.Name]; ok {
				inner[h.Name] = v
			}
		}
	}
	for i, p := range fn.Params {
		inner[p.Name] = args[i]
	}
	next := &evalEnv{
		locals:    inner,
		cells:     map[string]*Value{},
		params:    env.params,
		sigs:      env.sigs,
		stack:     append(append([]string{}, env.stack...), fn.Name),
		depth:     env.depth + 1,
		exprDepth: env.exprDepth,
		globals:   env.globals,
		host:      env.host,
		circ:      env.circ,
	}
	for _, stmt := range fn.Body {
		v, sig, ds := execStmts([]parser.FnStmt{stmt}, next)
		if len(ds) > 0 {
			return Value{}, ds
		}
		if sig == execReturn {
			return v, nil
		}
		if sig == execBreak || sig == execContinue {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeBreak, stmt.Line, stmt.Col, stmt.EndCol,
				"break and continue are only valid inside a host loop",
				"Remove the statement, or place it in for or while.")}
		}
	}
	return Value{}, []qerr.Diagnostic{diag(qerr.CodeMissingReturn, fn.Line, fn.Col, fn.Col+len(fn.Name),
		fmt.Sprintf("function %q is missing a return", fn.Name),
		"Add one return of the declared type.")}
}
