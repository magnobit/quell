// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"fmt"
	"strings"

	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/qerr"
)

// Tasks is the QubitLabs job bridge. A nil hook means async cannot run.
// Implementations must call the existing scheduler and must not invent a job table.
type Tasks interface {
	Submit(program string) (string, error)
	Status(id string) (string, error)
	Cancel(id string) error
}

// TaskHook is set by the process that owns scheduler credentials.
var TaskHook Tasks

func isTaskCall(name string) bool {
	switch name {
	case "async", "await", "all", "race", "timeout", "cancel":
		return true
	default:
		return false
	}
}

func (st *fnState) inferTask(n *parser.CallExpr, types map[string]string, from string, line int) (string, []qerr.Diagnostic) {
	switch n.Name {
	case "async":
		if len(n.Args) != 1 {
			return "", []qerr.Diagnostic{diag(qerr.CodeArgCount, line, n.Col, n.Col+5, "async expects the program text", "Write async(program).")}
		}
		t, ds := st.infer(n.Args[0], types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if t != parser.TypeString {
			return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+5, "async expects a string", "Pass the Quell program as a string.")}
		}
		return "task", nil
	case "await", "cancel":
		if len(n.Args) != 1 {
			return "", []qerr.Diagnostic{diag(qerr.CodeArgCount, line, n.Col, n.Col+len(n.Name), n.Name+" expects one task", "")}
		}
		t, ds := st.infer(n.Args[0], types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if t != "task" && t != parser.TypeString {
			return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+len(n.Name), n.Name+" expects a task", "")}
		}
		return parser.TypeString, nil
	case "timeout":
		if len(n.Args) != 2 {
			return "", []qerr.Diagnostic{diag(qerr.CodeArgCount, line, n.Col, n.Col+7, "timeout expects a task and an int", "Write timeout(task, milliseconds).")}
		}
		t, ds := st.infer(n.Args[0], types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if t != "task" && t != parser.TypeString {
			return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+7, "timeout expects a task", "")}
		}
		u, ds := st.infer(n.Args[1], types, from, line)
		if len(ds) > 0 {
			return "", ds
		}
		if u != parser.TypeInt {
			return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+7, "timeout duration must be int", "")}
		}
		return "task", nil
	case "all":
		if len(n.Args) < 1 {
			return "", []qerr.Diagnostic{diag(qerr.CodeArgCount, line, n.Col, n.Col+3, "all expects tasks", "")}
		}
		for _, a := range n.Args {
			t, ds := st.infer(a, types, from, line)
			if len(ds) > 0 {
				return "", ds
			}
			if t != "task" && t != parser.TypeString {
				return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+3, "all expects tasks", "")}
			}
		}
		return parser.TypeString, nil
	case "race":
		if len(n.Args) < 1 {
			return "", []qerr.Diagnostic{diag(qerr.CodeArgCount, line, n.Col, n.Col+4, "race expects tasks", "")}
		}
		for _, a := range n.Args {
			t, ds := st.infer(a, types, from, line)
			if len(ds) > 0 {
				return "", ds
			}
			if t != "task" && t != parser.TypeString {
				return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+4, "race expects tasks", "")}
			}
		}
		return "task", nil
	default:
		return "", []qerr.Diagnostic{diag(qerr.CodeUnknownFunc, line, n.Col, n.Col+len(n.Name), "unknown task call", "")}
	}
}

func evalTask(n *parser.CallExpr, env *evalEnv, line int) (Value, []qerr.Diagnostic) {
	if TaskHook == nil {
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+len(n.Name),
			"task bridge is not connected to QubitLabs",
			"Configure the scheduler client. Quell does not schedule jobs itself.")}
	}
	idOf := func(a parser.Expr) (string, []qerr.Diagnostic) {
		v, ds := evalExpr(a, env, line)
		if len(ds) > 0 {
			return "", ds
		}
		if v.Type != "task" && v.Type != parser.TypeString {
			return "", []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+len(n.Name), "expected a task id", "")}
		}
		return v.Str, nil
	}
	switch n.Name {
	case "async":
		v, ds := evalExpr(n.Args[0], env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		id, err := TaskHook.Submit(v.Str)
		if err != nil {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+5, err.Error(), "")}
		}
		return Value{Type: "task", Str: id}, nil
	case "await":
		id, ds := idOf(n.Args[0])
		if len(ds) > 0 {
			return Value{}, ds
		}
		status, err := TaskHook.Status(id)
		if err != nil {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+5, err.Error(), "")}
		}
		return Value{Type: parser.TypeString, Str: status}, nil
	case "cancel":
		id, ds := idOf(n.Args[0])
		if len(ds) > 0 {
			return Value{}, ds
		}
		if err := TaskHook.Cancel(id); err != nil {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+6, err.Error(), "")}
		}
		return Value{Type: parser.TypeString, Str: "cancel requested"}, nil
	case "timeout":
		id, ds := idOf(n.Args[0])
		if len(ds) > 0 {
			return Value{}, ds
		}
		dur, ds := evalExpr(n.Args[1], env, line)
		if len(ds) > 0 {
			return Value{}, ds
		}
		if dur.Int < 0 {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+7, "timeout milliseconds must be >= 0", "")}
		}
		status, err := TaskHook.Status(id)
		if err != nil {
			return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+7, err.Error(), "")}
		}
		if !terminalStatus(status) {
			if err := TaskHook.Cancel(id); err != nil {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+7, err.Error(), "")}
			}
		}
		return Value{Type: "task", Str: id}, nil
	case "all":
		var parts []string
		for _, a := range n.Args {
			id, ds := idOf(a)
			if len(ds) > 0 {
				return Value{}, ds
			}
			status, err := TaskHook.Status(id)
			if err != nil {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+3, err.Error(), "")}
			}
			parts = append(parts, id+"="+status)
		}
		return Value{Type: parser.TypeString, Str: strings.Join(parts, ",")}, nil
	case "race":
		var first string
		for _, a := range n.Args {
			id, ds := idOf(a)
			if len(ds) > 0 {
				return Value{}, ds
			}
			if first == "" {
				first = id
			}
			status, err := TaskHook.Status(id)
			if err != nil {
				return Value{}, []qerr.Diagnostic{diag(qerr.CodeTypeMismatch, line, n.Col, n.Col+4, err.Error(), "")}
			}
			if terminalStatus(status) {
				return Value{Type: "task", Str: id}, nil
			}
		}
		return Value{Type: "task", Str: first}, nil
	default:
		return Value{}, []qerr.Diagnostic{diag(qerr.CodeUnknownFunc, line, n.Col, n.Col+len(n.Name), fmt.Sprintf("unknown function %q", n.Name), "")}
	}
}

func terminalStatus(s string) bool {
	switch strings.ToLower(s) {
	case "succeeded", "success", "completed", "failed", "cancelled", "canceled", "error":
		return true
	default:
		return false
	}
}
