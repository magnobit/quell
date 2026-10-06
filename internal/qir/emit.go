// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qir emits LLVM IR for the QIR base-profile subset.
// It is not a second quantum IR. Operations outside that subset are
// reported as semantic losses and are not emitted as comments that
// look like successful QIR.
package qir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/magnobit/quell/internal/ir"
)

// Loss is one operation that was not emitted.
type Loss struct {
	Kind string
	Line int
}

// EmitLLVM writes LLVM IR for the supported base-profile subset.
// A non-empty loss list means the text is not a complete image of p.
func EmitLLVM(p *ir.Program) (string, []Loss) {
	if p == nil {
		return "", []Loss{{Kind: "empty"}}
	}
	n := p.NumQubits
	if n < 1 {
		n = 1
	}
	var losses []Loss
	var body []string
	var meas []string
	used := map[string]bool{}
	seenMeasure := false
	results := 0
	for _, op := range p.Ops {
		if len(op.Then) > 0 || len(op.Else) > 0 || op.Body != nil {
			losses = append(losses, Loss{Kind: string(op.Kind) + "-control"})
			continue
		}
		if unbound(op) {
			losses = append(losses, Loss{Kind: "unbound-param"})
			continue
		}
		switch op.Kind {
		case ir.OpBARRIER:
			continue
		case ir.OpRESET:
			losses = append(losses, Loss{Kind: "reset"})
		case ir.OpMEASURE:
			seenMeasure = true
			qs := op.Qubits
			if len(qs) == 0 {
				qs = []int{0}
			}
			for _, q := range qs {
				if q < 0 || q >= n {
					losses = append(losses, Loss{Kind: "qubit"})
					continue
				}
				meas = append(meas, fmt.Sprintf("  call void @__quantum__qis__mz__body(%s, %s)\n", staticPtr(q), staticPtrWrite(results)))
				results++
			}
			used["mz"] = true
		default:
			if seenMeasure {
				losses = append(losses, Loss{Kind: "gate-after-measure"})
				continue
			}
			line, key, ok := unitary(op)
			if !ok {
				losses = append(losses, Loss{Kind: string(op.Kind)})
				continue
			}
			for _, q := range op.Qubits {
				if q < 0 || q >= n {
					losses = append(losses, Loss{Kind: "qubit"})
					ok = false
				}
			}
			if ok {
				body = append(body, line)
				used[key] = true
			}
		}
	}
	if len(losses) > 0 {
		return "", losses
	}
	return module(n, results, body, meas, used), nil
}

// Emit returns LLVM IR when every operation mapped.
// Incomplete programs return an error instead of a fake dialect.
func Emit(p *ir.Program) (string, error) {
	text, losses := EmitLLVM(p)
	if len(losses) > 0 {
		kinds := make([]string, len(losses))
		for i, l := range losses {
			kinds[i] = l.Kind
		}
		return "", fmt.Errorf("qir export incomplete: %s", strings.Join(kinds, ", "))
	}
	return text, nil
}

func unbound(op ir.Op) bool {
	for _, n := range op.ArgNames {
		if n != "" {
			return true
		}
	}
	return false
}

func unitary(op ir.Op) (line, key string, ok bool) {
	q0 := 0
	q1 := 0
	if len(op.Qubits) > 0 {
		q0 = op.Qubits[0]
	}
	if len(op.Qubits) > 1 {
		q1 = op.Qubits[1]
	}
	switch op.Kind {
	case ir.OpH:
		return fmt.Sprintf("  call void @__quantum__qis__h__body(%s)\n", staticPtr(q0)), "h", true
	case ir.OpX:
		return fmt.Sprintf("  call void @__quantum__qis__x__body(%s)\n", staticPtr(q0)), "x", true
	case ir.OpY:
		return fmt.Sprintf("  call void @__quantum__qis__y__body(%s)\n", staticPtr(q0)), "y", true
	case ir.OpZ:
		return fmt.Sprintf("  call void @__quantum__qis__z__body(%s)\n", staticPtr(q0)), "z", true
	case ir.OpS:
		return fmt.Sprintf("  call void @__quantum__qis__s__body(%s)\n", staticPtr(q0)), "s", true
	case ir.OpT:
		return fmt.Sprintf("  call void @__quantum__qis__t__body(%s)\n", staticPtr(q0)), "t", true
	case ir.OpSDG:
		return fmt.Sprintf("  call void @__quantum__qis__s__adj(%s)\n", staticPtr(q0)), "s_adj", true
	case ir.OpTDG:
		return fmt.Sprintf("  call void @__quantum__qis__t__adj(%s)\n", staticPtr(q0)), "t_adj", true
	case ir.OpRX:
		return fmt.Sprintf("  call void @__quantum__qis__rx__body(double %s, %s)\n", angle(op), staticPtr(q0)), "rx", true
	case ir.OpRY:
		return fmt.Sprintf("  call void @__quantum__qis__ry__body(double %s, %s)\n", angle(op), staticPtr(q0)), "ry", true
	case ir.OpRZ:
		return fmt.Sprintf("  call void @__quantum__qis__rz__body(double %s, %s)\n", angle(op), staticPtr(q0)), "rz", true
	case ir.OpCNOT:
		return fmt.Sprintf("  call void @__quantum__qis__cnot__body(%s, %s)\n", staticPtr(q0), staticPtr(q1)), "cnot", true
	case ir.OpCZ:
		return fmt.Sprintf("  call void @__quantum__qis__cz__body(%s, %s)\n", staticPtr(q0), staticPtr(q1)), "cz", true
	case ir.OpSWAP:
		return fmt.Sprintf("  call void @__quantum__qis__swap__body(%s, %s)\n", staticPtr(q0), staticPtr(q1)), "swap", true
	default:
		return "", "", false
	}
}

func angle(op ir.Op) string {
	v := 0.0
	if len(op.Args) > 0 {
		v = op.Args[0]
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func staticPtr(id int) string {
	if id == 0 {
		return "ptr null"
	}
	return fmt.Sprintf("ptr inttoptr (i64 %d to ptr)", id)
}

func staticPtrWrite(id int) string {
	if id == 0 {
		return "ptr writeonly null"
	}
	return fmt.Sprintf("ptr writeonly inttoptr (i64 %d to ptr)", id)
}

func module(qubits, results int, body, meas []string, used map[string]bool) string {
	var b strings.Builder
	b.WriteString("; ModuleID = 'quell'\n")
	b.WriteString("source_filename = \"quell\"\n\n")
	if results > 0 {
		b.WriteString(fmt.Sprintf("@out = internal constant [4 x i8] c\"out\\00\"\n"))
		for i := 0; i < results; i++ {
			label := fmt.Sprintf("m%d", i)
			b.WriteString(fmt.Sprintf("@%s = internal constant [%d x i8] c\"%s\\00\"\n", label, len(label)+1, label))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "define i64 @main() #0 {\n")
	b.WriteString("entry:\n")
	b.WriteString("  call void @__quantum__rt__initialize(ptr null)\n")
	b.WriteString("  br label %body\n\n")
	b.WriteString("body:\n")
	for _, line := range body {
		b.WriteString(line)
	}
	b.WriteString("  br label %measurements\n\n")
	b.WriteString("measurements:\n")
	for _, line := range meas {
		b.WriteString(line)
	}
	b.WriteString("  br label %output\n\n")
	b.WriteString("output:\n")
	if results > 0 {
		fmt.Fprintf(&b, "  call void @__quantum__rt__tuple_record_output(i64 %d, ptr @out)\n", results)
		for i := 0; i < results; i++ {
			fmt.Fprintf(&b, "  call void @__quantum__rt__result_record_output(%s, ptr @m%d)\n", staticPtr(i), i)
		}
	}
	b.WriteString("  ret i64 0\n")
	b.WriteString("}\n\n")
	for _, spec := range declarations(used) {
		b.WriteString(spec)
	}
	b.WriteString("declare void @__quantum__rt__initialize(ptr)\n")
	if results > 0 {
		b.WriteString("declare void @__quantum__rt__tuple_record_output(i64, ptr)\n")
		b.WriteString("declare void @__quantum__rt__result_record_output(ptr, ptr)\n")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "attributes #0 = { \"entry_point\" \"qir_profiles\"=\"base_profile\" \"output_labeling_schema\"=\"quell.v1\" \"required_num_qubits\"=\"%d\" \"required_num_results\"=\"%d\" }\n", qubits, results)
	if used["mz"] {
		b.WriteString("attributes #1 = { \"irreversible\" }\n")
	}
	b.WriteString("\n")
	b.WriteString("!llvm.module.flags = !{!0, !1, !2, !3}\n\n")
	b.WriteString("!0 = !{i32 1, !\"qir_major_version\", i32 2}\n")
	b.WriteString("!1 = !{i32 7, !\"qir_minor_version\", i32 0}\n")
	b.WriteString("!2 = !{i32 1, !\"dynamic_qubit_management\", i1 false}\n")
	b.WriteString("!3 = !{i32 1, !\"dynamic_result_management\", i1 false}\n")
	return b.String()
}

func declarations(used map[string]bool) []string {
	type decl struct {
		key  string
		text string
	}
	order := []decl{
		{"h", "declare void @__quantum__qis__h__body(ptr)\n"},
		{"x", "declare void @__quantum__qis__x__body(ptr)\n"},
		{"y", "declare void @__quantum__qis__y__body(ptr)\n"},
		{"z", "declare void @__quantum__qis__z__body(ptr)\n"},
		{"s", "declare void @__quantum__qis__s__body(ptr)\n"},
		{"t", "declare void @__quantum__qis__t__body(ptr)\n"},
		{"s_adj", "declare void @__quantum__qis__s__adj(ptr)\n"},
		{"t_adj", "declare void @__quantum__qis__t__adj(ptr)\n"},
		{"rx", "declare void @__quantum__qis__rx__body(double, ptr)\n"},
		{"ry", "declare void @__quantum__qis__ry__body(double, ptr)\n"},
		{"rz", "declare void @__quantum__qis__rz__body(double, ptr)\n"},
		{"cnot", "declare void @__quantum__qis__cnot__body(ptr, ptr)\n"},
		{"cz", "declare void @__quantum__qis__cz__body(ptr, ptr)\n"},
		{"swap", "declare void @__quantum__qis__swap__body(ptr, ptr)\n"},
		{"mz", "declare void @__quantum__qis__mz__body(ptr, ptr writeonly) #1\n"},
	}
	var out []string
	for _, d := range order {
		if used[d.key] {
			out = append(out, d.text)
		}
	}
	return out
}
