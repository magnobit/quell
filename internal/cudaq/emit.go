// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package cudaq lowers Quell IR to a CUDA-Q Python program.
// It does not parse generated provider text back into Quell.
// Execution is attempted only when the cudaq module imports.
package cudaq

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// Emit writes a CUDA-Q kernel from canonical IR.
// Dynamic control and unbound parameters are errors.
func Emit(p *ir.Program) (string, error) {
	if p == nil {
		return "", fmt.Errorf("cudaq: nil program")
	}
	n := p.NumQubits
	if n < 1 {
		n = 1
	}
	var body strings.Builder
	if err := writeOps(&body, p.Ops); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("import cudaq\n")
	b.WriteString("@cudaq.kernel\n")
	b.WriteString("def quell():\n")
	fmt.Fprintf(&b, "    q = cudaq.qvector(%d)\n", n)
	if body.Len() == 0 {
		b.WriteString("    pass\n")
	} else {
		b.WriteString(body.String())
	}
	return b.String(), nil
}

func writeOps(b *strings.Builder, ops []ir.Op) error {
	for _, op := range ops {
		if len(op.Then) > 0 || len(op.Else) > 0 || op.Body != nil {
			return fmt.Errorf("cudaq adapter: %s is not lowered", op.Kind)
		}
		for _, name := range op.ArgNames {
			if name != "" {
				return fmt.Errorf("cudaq adapter: unbound parameter %s", name)
			}
		}
		q := func(i int) string {
			if i < len(op.Qubits) {
				return strconv.Itoa(op.Qubits[i])
			}
			return "0"
		}
		switch op.Kind {
		case ir.OpH:
			fmt.Fprintf(b, "    h(q[%s])\n", q(0))
		case ir.OpX:
			fmt.Fprintf(b, "    x(q[%s])\n", q(0))
		case ir.OpY:
			fmt.Fprintf(b, "    y(q[%s])\n", q(0))
		case ir.OpZ:
			fmt.Fprintf(b, "    z(q[%s])\n", q(0))
		case ir.OpRX:
			fmt.Fprintf(b, "    rx(%g, q[%s])\n", angle(op), q(0))
		case ir.OpRY:
			fmt.Fprintf(b, "    ry(%g, q[%s])\n", angle(op), q(0))
		case ir.OpRZ:
			fmt.Fprintf(b, "    rz(%g, q[%s])\n", angle(op), q(0))
		case ir.OpCNOT:
			fmt.Fprintf(b, "    cx(q[%s], q[%s])\n", q(0), q(1))
		case ir.OpMEASURE:
			if len(op.Qubits) == 0 {
				b.WriteString("    mz(q)\n")
			} else {
				for _, qubit := range op.Qubits {
					fmt.Fprintf(b, "    mz(q[%d])\n", qubit)
				}
			}
		case ir.OpBARRIER:
		default:
			return fmt.Errorf("cudaq adapter: unsupported op %s", op.Kind)
		}
	}
	return nil
}

func angle(op ir.Op) float64 {
	if len(op.Args) > 0 {
		return op.Args[0]
	}
	return 0
}

// Run emits a kernel and executes it on CUDA-Q's CPU target.
// It does not fall back to the pure-Go simulator and it does not claim a GPU.
// Counts come from a two-column dump this adapter prints, not from scraping
// a provider compiler listing.
func Run(src string, shots int) (map[string]int, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	kernel, err := Emit(ir.Lower(c))
	if err != nil {
		return nil, err
	}
	if shots <= 0 {
		shots = 100
	}
	script := fmt.Sprintf("import cudaq\ncudaq.set_target('qpp-cpu')\n%s\nresult = cudaq.sample(quell, shots_count=%d)\nfor bits, count in result.items():\n    print(bits, int(count))\n", kernel, shots)
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return nil, fmt.Errorf("python is not installed")
		}
	}
	cmd := exec.Command(py, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	counts := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		counts[fields[0]] += n
	}
	if len(counts) == 0 {
		return nil, fmt.Errorf("cudaq returned no counts: %s", strings.TrimSpace(string(out)))
	}
	return counts, nil
}
