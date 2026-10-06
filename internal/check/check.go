// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package check is the single semantic checker for Quell host values.
// The parser accepts syntax. This package decides whether a let is well typed.
// LSP, the compiler, and the simulator all call Check. They do not reimplement it.
package check

import (
	"fmt"
	"math"
	"strconv"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

// Value is the constant result of a host expression.
type Value struct {
	Type  string
	Int   int64
	Float float64
	Bool  bool
	Str   string
	Elems []Value
}

// Check typechecks host declarations and host functions on c.
// A nil circuit has no diagnostics. Quantum instructions are not rewritten.
func Check(c *parser.Circuit) []qerr.Diagnostic {
	if c == nil {
		return nil
	}
	st := newFnState(c)
	var diags []qerr.Diagnostic
	diags = append(diags, st.signatures()...)
	diags = append(diags, st.bodies()...)
	diags = append(diags, st.recursion()...)
	diags = append(diags, st.globals()...)
	return diags
}

// Fail returns the first error diagnostic from Check, or nil.
func Fail(c *parser.Circuit) error {
	for _, d := range Check(c) {
		if d.Severity == qerr.SeverityError || d.Severity == "" {
			return qerr.New(qerr.KindCheck, d)
		}
	}
	return nil
}

// Eval evaluates a host expression against already-declared locals.
// params names a PARAM that must not be read as a local.
// Function calls need EvalIn, which supplies the circuit's functions.
func Eval(e parser.Expr, locals map[string]Value, line int, params map[string]bool) (Value, []qerr.Diagnostic) {
	return evalExpr(e, &evalEnv{locals: locals, params: params}, line)
}

func evalExpr(e parser.Expr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if env == nil {
		env = &evalEnv{}
	}
	if e == nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeSyntax, line, 1, 2, "missing expression", "Write an initializer after =.")}
	}
	switch n := e.(type) {
	case *parser.LiteralExpr:
		return Value{Type: n.Type, Int: n.Int, Float: n.Float, Bool: n.Bool, Str: n.Str}, nil
	case *parser.IdentExpr:
		if env.params != nil && env.params[n.Name] {
			col, end := n.Span()
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, line, col, end,
				fmt.Sprintf("unknown identifier %q; PARAM %q is a circuit parameter, not a host local", n.Name, n.Name),
				"Use the PARAM on a gate angle, or pick a different let name. A host function cannot read a PARAM.")}
		}
		v, ok := lookupLocal(env, n.Name)
		if !ok {
			col, end := n.Span()
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, line, col, end,
				fmt.Sprintf("unknown identifier %q", n.Name),
				"Declare it with let before this use. File scope is source order. A function sees parameters, its own lets, and file lets declared above the function.")}
		}
		return v, nil
	case *parser.CallExpr:
		return evalCall(n, env, line)
	case *parser.ArrayExpr:
		return evalArray(n, env, line)
	case *parser.IndexExpr:
		return evalIndex(n, env, line)
	case *parser.UnaryExpr:
		v, ds := evalExpr(n.X, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		col, end := n.Span()
		switch n.Op {
		case "!":
			if v.Type != parser.TypeBool {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end,
					fmt.Sprintf("operator ! is not valid for %s", v.Type),
					"Use ! only on bool.")}
			}
			return Value{Type: parser.TypeBool, Bool: !v.Bool}, nil
		case "-":
			switch v.Type {
			case parser.TypeInt:
				if v.Int == math.MinInt64 {
					return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "negation overflows int", "Use a smaller integer.")}
				}
				return Value{Type: parser.TypeInt, Int: -v.Int}, nil
			case parser.TypeFloat:
				return Value{Type: parser.TypeFloat, Float: -v.Float}, nil
			default:
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end,
					fmt.Sprintf("operator - is not valid for %s", v.Type),
					"Negate an int or a float.")}
			}
		default:
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "unknown unary operator "+n.Op, "")}
		}
	case *parser.BinaryExpr:
		l, ds := evalExpr(n.L, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		r, ds := evalExpr(n.R, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		return evalBinary(n, l, r, line)
	case *parser.ConvertExpr:
		v, ds := evalExpr(n.X, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		out, errd := convert(n.To, v, line, n)
		if errd != nil {
			return Value{}, []qerr.Diagnostic{*errd}
		}
		return out, nil
	case *parser.IfExpr:
		return evalIf(n, env, line)
	default:
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeSyntax, line, 1, 2, "unsupported expression", "")}
	}
}

func evalBinary(n *parser.BinaryExpr, l, r Value, line int) (Value, []qerr.Diagnostic) {
	col, end := n.Span()
	bad := func(msg, fix string) (Value, []qerr.Diagnostic) {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, msg, fix)}
	}
	switch n.Op {
	case "&&", "||":
		if l.Type != parser.TypeBool || r.Type != parser.TypeBool {
			return bad(fmt.Sprintf("operator %s requires bool operands", n.Op), "Use && and || only on bool.")
		}
		if n.Op == "&&" {
			return Value{Type: parser.TypeBool, Bool: l.Bool && r.Bool}, nil
		}
		return Value{Type: parser.TypeBool, Bool: l.Bool || r.Bool}, nil
	case "==", "!=":
		if l.Type != r.Type {
			return bad(fmt.Sprintf("operator %s requires two values of the same type", n.Op), "Convert one side explicitly so both types match.")
		}
		eq := valuesEqual(l, r)
		if n.Op == "!=" {
			eq = !eq
		}
		return Value{Type: parser.TypeBool, Bool: eq}, nil
	case "<", "<=", ">", ">=":
		if l.Type != r.Type || (l.Type != parser.TypeInt && l.Type != parser.TypeFloat) {
			return bad(fmt.Sprintf("operator %s requires two ints or two floats", n.Op), "Compare ints with ints, or floats with floats. Convert explicitly.")
		}
		return Value{Type: parser.TypeBool, Bool: compare(n.Op, l, r)}, nil
	case "+", "-", "*", "/", "%":
		if l.Type != r.Type || (l.Type != parser.TypeInt && l.Type != parser.TypeFloat) {
			return bad(fmt.Sprintf("operator %s requires two ints or two floats, not %s and %s", n.Op, l.Type, r.Type),
				"Convert one side explicitly. int and float are not mixed.")
		}
		if n.Op == "%" && l.Type != parser.TypeInt {
			return bad("operator % is only valid for int", "Use % on integers, or rewrite the float expression.")
		}
		if (n.Op == "/" || n.Op == "%") && ((l.Type == parser.TypeInt && r.Int == 0) || (l.Type == parser.TypeFloat && r.Float == 0)) {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeDivZero, line, col, end, "division by zero", "Use a non-zero divisor.")}
		}
		return arith(n.Op, l, r, line, col, end)
	default:
		return bad("unknown operator "+n.Op, "")
	}
}

func arith(op string, l, r Value, line, col, end int) (Value, []qerr.Diagnostic) {
	if l.Type == parser.TypeInt {
		var out int64
		switch op {
		case "+":
			if addOverflow(l.Int, r.Int) {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "integer addition overflows", "Use a smaller value or float.")}
			}
			out = l.Int + r.Int
		case "-":
			if subOverflow(l.Int, r.Int) {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "integer subtraction overflows", "Use a smaller value or float.")}
			}
			out = l.Int - r.Int
		case "*":
			if mulOverflow(l.Int, r.Int) {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "integer multiplication overflows", "Use a smaller value or float.")}
			}
			out = l.Int * r.Int
		case "/":
			out = l.Int / r.Int
		case "%":
			out = l.Int % r.Int
		}
		return Value{Type: parser.TypeInt, Int: out}, nil
	}
	var out float64
	switch op {
	case "+":
		out = l.Float + r.Float
	case "-":
		out = l.Float - r.Float
	case "*":
		out = l.Float * r.Float
	case "/":
		out = l.Float / r.Float
	}
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeBadOperator, line, col, end, "float result is not finite", "Use a finite float expression.")}
	}
	return Value{Type: parser.TypeFloat, Float: out}, nil
}

func addOverflow(a, b int64) bool {
	if b > 0 {
		return a > math.MaxInt64-b
	}
	return a < math.MinInt64-b
}

func subOverflow(a, b int64) bool {
	if b < 0 {
		return a > math.MaxInt64+b
	}
	return a < math.MinInt64+b
}

func mulOverflow(a, b int64) bool {
	if a == 0 || b == 0 {
		return false
	}
	if a == math.MinInt64 && b == -1 || b == math.MinInt64 && a == -1 {
		return true
	}
	c := a * b
	return c/b != a
}

func valuesEqual(l, r Value) bool {
	switch l.Type {
	case parser.TypeBool:
		return l.Bool == r.Bool
	case parser.TypeInt:
		return l.Int == r.Int
	case parser.TypeFloat:
		return l.Float == r.Float
	default:
		if _, ok := parser.ElemType(l.Type); ok {
			if len(l.Elems) != len(r.Elems) {
				return false
			}
			for i := range l.Elems {
				if !valuesEqual(l.Elems[i], r.Elems[i]) {
					return false
				}
			}
			return true
		}
		return l.Str == r.Str
	}
}

func compare(op string, l, r Value) bool {
	var less, eq bool
	if l.Type == parser.TypeInt {
		less, eq = l.Int < r.Int, l.Int == r.Int
	} else {
		less, eq = l.Float < r.Float, l.Float == r.Float
	}
	switch op {
	case "<":
		return less
	case "<=":
		return less || eq
	case ">":
		return !less && !eq
	default:
		return !less
	}
}

func convert(to string, v Value, line int, n *parser.ConvertExpr) (Value, *qerr.Diagnostic) {
	col, end := n.Span()
	fail := func(msg string) (Value, *qerr.Diagnostic) {
		d := diag(qerr.CodeBadConvert, line, col, end, msg, "Use a conversion listed for this type, or change the expression.")
		return Value{}, &d
	}
	switch to {
	case parser.TypeBool:
		switch v.Type {
		case parser.TypeBool:
			return v, nil
		case parser.TypeInt:
			return Value{Type: parser.TypeBool, Bool: v.Int != 0}, nil
		case parser.TypeFloat:
			if math.IsNaN(v.Float) {
				return fail("cannot convert NaN to bool")
			}
			return Value{Type: parser.TypeBool, Bool: v.Float != 0}, nil
		default:
			return fail("cannot convert string to bool")
		}
	case parser.TypeInt:
		switch v.Type {
		case parser.TypeInt:
			return v, nil
		case parser.TypeBool:
			if v.Bool {
				return Value{Type: parser.TypeInt, Int: 1}, nil
			}
			return Value{Type: parser.TypeInt, Int: 0}, nil
		case parser.TypeFloat:
			if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) || v.Float > math.MaxInt64 || v.Float < math.MinInt64 {
				return fail("float is outside the int range")
			}
			return Value{Type: parser.TypeInt, Int: int64(v.Float)}, nil
		default:
			n, err := strconv.ParseInt(v.Str, 10, 64)
			if err != nil {
				return fail(fmt.Sprintf("cannot convert string %q to int", v.Str))
			}
			return Value{Type: parser.TypeInt, Int: n}, nil
		}
	case parser.TypeFloat:
		switch v.Type {
		case parser.TypeFloat:
			return v, nil
		case parser.TypeInt:
			return Value{Type: parser.TypeFloat, Float: float64(v.Int)}, nil
		case parser.TypeBool:
			if v.Bool {
				return Value{Type: parser.TypeFloat, Float: 1}, nil
			}
			return Value{Type: parser.TypeFloat, Float: 0}, nil
		default:
			f, err := strconv.ParseFloat(v.Str, 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return fail(fmt.Sprintf("cannot convert string %q to float", v.Str))
			}
			return Value{Type: parser.TypeFloat, Float: f}, nil
		}
	case parser.TypeString:
		switch v.Type {
		case parser.TypeString:
			return v, nil
		case parser.TypeBool:
			if v.Bool {
				return Value{Type: parser.TypeString, Str: "true"}, nil
			}
			return Value{Type: parser.TypeString, Str: "false"}, nil
		case parser.TypeInt:
			return Value{Type: parser.TypeString, Str: strconv.FormatInt(v.Int, 10)}, nil
		default:
			return Value{Type: parser.TypeString, Str: strconv.FormatFloat(v.Float, 'g', -1, 64)}, nil
		}
	default:
		return fail("unknown conversion target " + to)
	}
}

func assignDiag(h parser.HostDecl, locals map[string]Value) qerr.Diagnostic {
	if _, ok := locals[h.Name]; ok {
		return diag(qerr.CodeImmutable, h.Line, h.Col, h.Col+len(h.Name),
			fmt.Sprintf("assignment to immutable local %q", h.Name),
			"Declare a new let. Phase 3 locals cannot be reassigned.")
	}
	return diag(qerr.CodeUnknownIdent, h.Line, h.Col, h.Col+len(h.Name),
		fmt.Sprintf("unknown identifier %q", h.Name),
		"Locals are introduced with let and cannot be assigned.")
}

func clash(h parser.HostDecl) qerr.Diagnostic {
	return diag(qerr.CodeParamClash, h.Line, h.Col, h.Col+len(h.Name),
		fmt.Sprintf("PARAM %q and let %q cannot share a name", h.Name, h.Name),
		"Rename the local or the PARAM. They are different kinds of input.")
}

func diag(code string, line, col, end int, msg, fix string) qerr.Diagnostic {
	if col < 1 {
		col = 1
	}
	if end < col {
		end = col + 1
	}
	docs := qerr.DocsHostValues
	if len(code) >= 4 && code[:4] == "QL23" {
		docs = qerr.DocsHostIf
	} else if len(code) >= 4 && code[:4] == "QL24" {
		docs = qerr.DocsHostOutput
	} else if len(code) >= 4 && code[:4] == "QL25" {
		docs = qerr.DocsHostLoops
	} else if len(code) >= 4 && code[:4] == "QL26" {
		docs = qerr.DocsHostArrays
	} else if len(code) >= 4 && (code[:4] == "QL21" || code[:4] == "QL22") {
		docs = qerr.DocsHostFunctions
	}
	return qerr.Diagnostic{
		Code: code, Severity: qerr.SeverityError, Message: msg,
		Line: line, Column: col, EndLine: line, EndColumn: end,
		SuggestedFix: fix, DocsURL: docs,
	}
}

func diagWarn(code string, line, col, end int, msg, fix string) qerr.Diagnostic {
	d := diag(code, line, col, end, msg, fix)
	d.Severity = qerr.SeverityWarning
	return d
}
