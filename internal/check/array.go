// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"fmt"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

func (st *fnState) inferArray(n *parser.ArrayExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	if n == nil || len(n.Elems) == 0 {
		return "", []qerr.Diagnostic{diag(qerr.CodeArrayType, line, n.Col, n.EndCol, "empty array has no element type", "Write at least one element, or declare the array from a non-empty value.")}
	}
	if len(n.Elems) > parser.MaxArrayLen {
		return "", []qerr.Diagnostic{diag(qerr.CodeArrayLimit, line, n.Col, n.EndCol, fmt.Sprintf("array has %d elements; the limit is %d", len(n.Elems), parser.MaxArrayLen), "Split the collection or reduce it.")}
	}
	first, ds := st.infer(n.Elems[0], types, from, line)
	if len(ds) > 0 {
		return "", ds
	}
	if _, ok := parser.ElemType(parser.ArrayType(first)); !ok {
		return "", []qerr.Diagnostic{diag(qerr.CodeArrayType, line, n.Col, n.EndCol, "array elements must be bool, int, float, or string", "Do not nest arrays or put a function in an array.")}
	}
	for _, el := range n.Elems[1:] {
		t, more := st.infer(el, types, from, line)
		ds = append(ds, more...)
		if t != "" && t != first {
			col, end := el.Span()
			ds = append(ds, diag(qerr.CodeArrayType, line, col, end, fmt.Sprintf("array element has type %s, expected %s", t, first), "Use one element type in an array literal."))
		}
	}
	if len(ds) > 0 {
		return "", ds
	}
	return first + "[]", nil
}

func (st *fnState) inferIndex(n *parser.IndexExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	xt, ds := st.infer(n.X, types, from, line)
	if len(ds) > 0 {
		return "", ds
	}
	elem, ok := parser.ElemType(xt)
	if !ok {
		col, end := n.X.Span()
		return "", []qerr.Diagnostic{diag(qerr.CodeArrayType, line, col, end, fmt.Sprintf("cannot index type %s", xt), "Index an array such as int[].")}
	}
	it, more := st.infer(n.Index, types, from, line)
	ds = append(ds, more...)
	if it != "" && it != parser.TypeInt {
		col, end := n.Index.Span()
		ds = append(ds, diag(qerr.CodeTypeMismatch, line, col, end, "array index must be int", "Use an int index."))
	}
	if lit, ok := n.Index.(*parser.LiteralExpr); ok && lit.Type == parser.TypeInt {
		if arr, ok := n.X.(*parser.ArrayExpr); ok && (lit.Int < 0 || int(lit.Int) >= len(arr.Elems)) {
			col, end := n.Index.Span()
			ds = append(ds, diag(qerr.CodeIndex, line, col, end, "index out of bounds", "Use an index from 0 through len(array)-1. Host for ranges are inclusive."))
		}
		if id, ok := n.X.(*parser.IdentExpr); ok {
			if nlen, known := st.lengths[id.Name]; known && (lit.Int < 0 || int(lit.Int) >= nlen) {
				col, end := n.Index.Span()
				ds = append(ds, diag(qerr.CodeIndex, line, col, end, "index out of bounds", "Use an index from 0 through len(array)-1. Host for ranges are inclusive."))
			}
		}
	}
	if len(ds) > 0 {
		return "", ds
	}
	return elem, nil
}

func (st *fnState) inferLen(n *parser.CallExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	if len(n.Args) != 1 {
		return "", []qerr.Diagnostic{diag(qerr.CodeArrayType, line, n.Col, n.Col+3, "len expects one array", "Write len(xs).")}
	}
	t, ds := st.infer(n.Args[0], types, from, line)
	if len(ds) > 0 {
		return "", ds
	}
	if _, ok := parser.ElemType(t); !ok {
		return "", []qerr.Diagnostic{diag(qerr.CodeArrayType, line, n.Col, n.Col+3, "len expects an array", "Pass an int[], float[], bool[], or string[] value.")}
	}
	return parser.TypeInt, nil
}

func (st *fnState) checkIndexAssign(stmt parser.FnStmt, types map[string]string, mutable map[string]bool, fn *parser.FnDecl) []qerr.Diagnostic {
	if !mutable[stmt.Decl.Name] {
		return []qerr.Diagnostic{diag(qerr.CodeImmutable, stmt.Line, stmt.Col, stmt.Col+len(stmt.Decl.Name),
			fmt.Sprintf("assignment to immutable local %q", stmt.Decl.Name),
			"Declare the array with var if an element must change. let arrays stay immutable.")}
	}
	elem, ok := parser.ElemType(types[stmt.Decl.Name])
	if !ok {
		return []qerr.Diagnostic{diag(qerr.CodeArrayType, stmt.Line, stmt.Col, stmt.EndCol, "element assignment requires an array", "Index a var array.")}
	}
	it, ds := st.infer(stmt.Decl.Index, types, fn.Name, stmt.Line)
	if it != "" && it != parser.TypeInt {
		ds = append(ds, diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Col, stmt.EndCol, "array index must be int", "Use an int index."))
	}
	vt, more := st.infer(stmt.Decl.Expr, types, fn.Name, stmt.Line)
	ds = append(ds, more...)
	if vt != "" && vt != elem {
		ds = append(ds, diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Col, stmt.EndCol,
			fmt.Sprintf("type mismatch: element has type %s, expression has type %s", elem, vt),
			"Assign a value of the element type."))
	}
	return ds
}

func evalArray(n *parser.ArrayExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if len(n.Elems) > parser.MaxArrayLen {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeArrayLimit, line, n.Col, n.EndCol, "array exceeds the size limit", "Reduce the array.")}
	}
	elems := make([]Value, 0, len(n.Elems))
	var elemType string
	for _, el := range n.Elems {
		v, ds := evalExpr(el, env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		if elemType == "" {
			elemType = v.Type
		}
		elems = append(elems, v)
	}
	return Value{Type: elemType + "[]", Elems: elems}, nil
}

func evalIndex(n *parser.IndexExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	arr, ds := evalExpr(n.X, env, line)
	if len(ds) > 0 {
		return Value{}, ds
	}
	idx, ds := evalExpr(n.Index, env, line)
	if len(ds) > 0 {
		return Value{}, ds
	}
	if idx.Type != parser.TypeInt || idx.Int < 0 || int(idx.Int) >= len(arr.Elems) {
		col, end := n.Index.Span()
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeIndex, line, col, end, "index out of bounds", "Use an index from 0 through len(array)-1.")}
	}
	return arr.Elems[idx.Int], nil
}

// ExpectationHook evaluates a named observable. The simulator registers it.
// A nil hook means expectation cannot run in this process.
var ExpectationHook func(c *parser.Circuit, name string) (float64, error)

func (st *fnState) inferExpectation(n *parser.CallExpr, line int) (string, []qerr.Diagnostic) {
	if len(n.Args) != 1 {
		return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+11, "expectation expects an observable name", "Write expectation(h).")}
	}
	id, ok := n.Args[0].(*parser.IdentExpr)
	if !ok {
		return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+11, "expectation expects an observable name", "Pass the observable's name, not an expression.")}
	}
	if st.c != nil {
		for _, ob := range st.c.Observables {
			if ob.Name == id.Name {
				return parser.TypeFloat, nil
			}
		}
	}
	return "", []qerr.Diagnostic{diag(qerr.CodeUnknownIdent, line, id.Col, id.EndCol, fmt.Sprintf("unknown observable %q", id.Name), "Declare it with observable name = Z(0).")}
}

func evalExpectation(n *parser.CallExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if ExpectationHook == nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+11, "expectation is not connected to a simulator", "Run this program through the local simulator.")}
	}
	id, ok := n.Args[0].(*parser.IdentExpr)
	if !ok {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+11, "expectation expects an observable name", "Pass the observable's name.")}
	}
	v, err := ExpectationHook(env.circ, id.Name)
	if err != nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+11, err.Error(), "Use an observable the local simulator can evaluate.")}
	}
	return Value{Type: parser.TypeFloat, Float: v}, nil
}

func evalLen(n *parser.CallExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if len(n.Args) != 1 {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeArrayType, line, n.Col, n.Col+3, "len expects one array", "Write len(xs).")}
	}
	v, ds := evalExpr(n.Args[0], env, line)
	if len(ds) > 0 {
		return Value{}, ds
	}
	return Value{Type: parser.TypeInt, Int: int64(len(v.Elems))}, nil
}

func assignIndex(cell *Value, stmt parser.FnStmt, v Value, env *evalEnv) (Value, int, []qerr.Diagnostic) {
	idx, ds := evalExpr(stmt.Decl.Index, env, stmt.Line)
	if len(ds) > 0 {
		return Value{}, execFall, ds
	}
	if idx.Type != parser.TypeInt || idx.Int < 0 || int(idx.Int) >= len(cell.Elems) {
		return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeIndex, stmt.Line, stmt.Col, stmt.EndCol, "index out of bounds", "Use an index from 0 through len(array)-1.")}
	}
	elem, _ := parser.ElemType(cell.Type)
	if v.Type != elem {
		return Value{}, execFall, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, stmt.Line, stmt.Col, stmt.EndCol, "element type mismatch", "Assign the element type.")}
	}
	cell.Elems[idx.Int] = v
	return Value{}, execFall, nil
}
