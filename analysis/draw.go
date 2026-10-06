// Copyright 2026 Magnobit, Inc. All rights reserved.

package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/magnobit/quell/internal/ir"
)

// Draw renders the program as an ASCII circuit, one row per qubit. Qubit 0 is
// the top row. The output is ASCII only so it is safe on any terminal code
// page. Symbols: [H] single-qubit gate, * control, (+) CNOT/CCX target,
// x SWAP end, [M] measurement, || barrier, | wire crossing a gate.
// Control-flow blocks are drawn as one labelled box on the qubits they touch;
// their bodies are not expanded.
func Draw(p *ir.Program) string {
	if p == nil || p.NumQubits < 1 {
		return ""
	}
	n := p.NumQubits
	type item struct {
		cells    map[int]string
		vertical bool
	}
	var items []item
	var add func(op ir.Op)
	add = func(op ir.Op) {
		if op.Kind == ir.OpPAR {
			for _, b := range op.Then {
				add(b)
			}
			return
		}
		cells, vertical := opCells(op, n)
		if len(cells) > 0 {
			items = append(items, item{cells, vertical})
		}
	}
	for _, op := range p.Ops {
		add(op)
	}

	next := make([]int, n)
	var cols []map[int]string
	var spans [][2]int
	var vert []bool
	for _, it := range items {
		lo, hi := n, -1
		for q := range it.cells {
			if q < 0 || q >= n {
				continue
			}
			if q < lo {
				lo = q
			}
			if q > hi {
				hi = q
			}
		}
		if hi < 0 {
			continue
		}
		occupied := []int{}
		if it.vertical {
			for q := lo; q <= hi; q++ {
				occupied = append(occupied, q)
			}
		} else {
			for q := range it.cells {
				if q >= 0 && q < n {
					occupied = append(occupied, q)
				}
			}
		}
		col := 0
		for _, q := range occupied {
			if next[q] > col {
				col = next[q]
			}
		}
		for _, q := range occupied {
			next[q] = col + 1
		}
		for len(cols) <= col {
			cols = append(cols, map[int]string{})
			spans = append(spans, [2]int{n, -1})
			vert = append(vert, false)
		}
		for q, l := range it.cells {
			cols[col][q] = l
		}
		if it.vertical {
			vert[col] = true
			if lo < spans[col][0] {
				spans[col][0] = lo
			}
			if hi > spans[col][1] {
				spans[col][1] = hi
			}
		}
	}

	digits := len(fmt.Sprint(n - 1))
	prefix := func(q int) string { return fmt.Sprintf("q%-*d: ", digits, q) }
	blank := strings.Repeat(" ", len(prefix(0)))

	widths := make([]int, len(cols))
	for c, cells := range cols {
		w := 3
		for _, l := range cells {
			if len(l) > w {
				w = len(l)
			}
		}
		w += 2
		if w%2 == 0 {
			w++
		}
		widths[c] = w
	}

	var b strings.Builder
	for q := 0; q < n; q++ {
		b.WriteString(prefix(q))
		for c, cells := range cols {
			label, ok := cells[q]
			if !ok && vert[c] && q > spans[c][0] && q < spans[c][1] {
				label, ok = "|", true
			}
			b.WriteString(center(label, ok, widths[c]))
		}
		b.WriteString("--")
		b.WriteString("\n")
		if q == n-1 {
			break
		}
		b.WriteString(blank)
		var line strings.Builder
		for c := range cols {
			cell := []byte(strings.Repeat(" ", widths[c]))
			if vert[c] && q >= spans[c][0] && q < spans[c][1] {
				cell[widths[c]/2] = '|'
			}
			line.Write(cell)
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteString("\n")
	}
	return b.String()
}

func center(label string, ok bool, w int) string {
	if !ok {
		return strings.Repeat("-", w)
	}
	left := (w - len(label)) / 2
	return strings.Repeat("-", left) + label + strings.Repeat("-", w-len(label)-left)
}

func fmtArgs(op ir.Op) string {
	parts := make([]string, len(op.Args))
	for i, a := range op.Args {
		if i < len(op.ArgNames) && op.ArgNames[i] != "" {
			parts[i] = op.ArgNames[i]
			continue
		}
		parts[i] = fmt.Sprintf("%.4g", a)
	}
	return strings.Join(parts, ",")
}

func gateLabel(op ir.Op) string {
	if len(op.Args) == 0 {
		return "[" + string(op.Kind) + "]"
	}
	return "[" + string(op.Kind) + "(" + fmtArgs(op) + ")]"
}

// opCells maps qubit -> label for one operation. The second result is true
// when a vertical connector should join the qubits.
func opCells(op ir.Op, n int) (map[int]string, bool) {
	q := op.Qubits
	cells := map[int]string{}
	switch op.Kind {
	case ir.OpH, ir.OpX, ir.OpY, ir.OpZ, ir.OpS, ir.OpT, ir.OpSDG, ir.OpTDG, ir.OpSX,
		ir.OpRX, ir.OpRY, ir.OpRZ, ir.OpP, ir.OpU:
		for _, t := range q {
			cells[t] = gateLabel(op)
		}
		return cells, false
	case ir.OpCNOT:
		if len(q) == 2 {
			cells[q[0]], cells[q[1]] = "*", "(+)"
			return cells, true
		}
	case ir.OpCZ:
		if len(q) == 2 {
			cells[q[0]], cells[q[1]] = "*", "*"
			return cells, true
		}
	case ir.OpSWAP:
		if len(q) == 2 {
			cells[q[0]], cells[q[1]] = "x", "x"
			return cells, true
		}
	case ir.OpISWAP:
		if len(q) == 2 {
			cells[q[0]], cells[q[1]] = "[iSWAP]", "[iSWAP]"
			return cells, true
		}
	case ir.OpCRX, ir.OpCRY, ir.OpCRZ:
		if len(q) == 2 {
			cells[q[0]] = "*"
			cells[q[1]] = "[" + strings.TrimPrefix(string(op.Kind), "C") + "(" + fmtArgs(op) + ")]"
			return cells, true
		}
	case ir.OpCCX:
		if len(q) == 3 {
			cells[q[0]], cells[q[1]], cells[q[2]] = "*", "*", "(+)"
			return cells, true
		}
	case ir.OpCSWAP:
		if len(q) == 3 {
			cells[q[0]], cells[q[1]], cells[q[2]] = "*", "x", "x"
			return cells, true
		}
	case ir.OpMEASURE:
		targets := q
		if len(targets) == 0 {
			for i := 0; i < n; i++ {
				targets = append(targets, i)
			}
		}
		for _, t := range targets {
			cells[t] = "[M]"
		}
		return cells, false
	case ir.OpBARRIER:
		targets := q
		if len(targets) == 0 {
			for i := 0; i < n; i++ {
				targets = append(targets, i)
			}
		}
		for _, t := range targets {
			cells[t] = "||"
		}
		return cells, false
	case ir.OpRESET:
		for _, t := range q {
			cells[t] = "[|0>]"
		}
		return cells, false
	case ir.OpIF, ir.OpWHILE, ir.OpSWITCH, ir.OpASSERT:
		touched := map[int]bool{}
		collectQubits(op, touched)
		if len(touched) == 0 {
			touched[0] = true
		}
		keys := make([]int, 0, len(touched))
		for t := range touched {
			keys = append(keys, t)
		}
		sort.Ints(keys)
		for _, t := range keys {
			cells[t] = "[" + string(op.Kind) + "]"
		}
		return cells, false
	}
	for _, t := range q {
		cells[t] = "[" + string(op.Kind) + "]"
	}
	return cells, len(q) > 1
}

func collectQubits(op ir.Op, into map[int]bool) {
	for _, t := range op.Qubits {
		into[t] = true
	}
	if op.Body != nil {
		collectQubits(*op.Body, into)
	}
	for _, o := range op.Then {
		collectQubits(o, into)
	}
	for _, o := range op.Else {
		collectQubits(o, into)
	}
	for _, arm := range op.Cases {
		for _, o := range arm.Body {
			collectQubits(o, into)
		}
	}
}
