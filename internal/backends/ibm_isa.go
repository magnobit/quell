// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ibmTarget is what the ISA translation needs from a backend configuration.
type ibmTarget struct {
	NumQubits int
	Basis     map[string]bool
	Coupling  map[[2]int]bool // empty means unchecked
}

func (t ibmTarget) coupled(a, b int) bool {
	return len(t.Coupling) == 0 || t.Coupling[[2]int{a, b}] || t.Coupling[[2]int{b, a}]
}

var defaultIBMTarget = ibmTarget{Basis: map[string]bool{"cz": true, "rz": true, "sx": true, "x": true, "id": true, "measure": true, "reset": true, "delay": true}}

var (
	qasmDeclRe = regexp.MustCompile(`^(qubit|bit)\s*(\[\s*(\d+)\s*\])?\s+([A-Za-z_]\w*)\s*;$`)
	qasmRegRe  = regexp.MustCompile(`^(qreg|creg)\s+([A-Za-z_]\w*)\s*\[\s*(\d+)\s*\]\s*;$`)
	qasmOpRe   = regexp.MustCompile(`^([A-Za-z_]\w*)\s*(\((.*)\))?\s+(.+);$`)
	qasmArgRe  = regexp.MustCompile(`^([A-Za-z_]\w*)\s*\[\s*(\d+)\s*\]$`)
)

// toIBMISA rewrites an OpenQASM 3 circuit into the rz/sx/x/cz instruction set
// IBM primitives require, with qubits mapped trivially to physical qubits.
// It rejects what it cannot translate faithfully (control flow, custom gates,
// two-qubit gates on uncoupled qubits) instead of letting the job fail on
// the QPU.
func toIBMISA(src string, t ibmTarget) (string, error) {
	if !t.Basis["cz"] || !t.Basis["rz"] || !t.Basis["sx"] {
		return "", fmt.Errorf("backend basis %v is not rz/sx/cz; native translation for it is not supported", sortedBasis(t.Basis))
	}
	qubitBase := map[string]int{}
	qubitSize := map[string]int{}
	total := 0
	var out []string
	hasInclude := false
	emit := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	for n, rawLine := range strings.Split(src, "\n") {
		line := strings.TrimSpace(rawLine)
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "OPENQASM"):
			out = append(out, line)
			continue
		case strings.HasPrefix(line, "include"):
			hasInclude = hasInclude || strings.Contains(line, "stdgates.inc")
			out = append(out, line)
			continue
		}
		if m := qasmDeclRe.FindStringSubmatch(line); m != nil {
			if m[1] == "qubit" {
				size := 1
				if m[3] != "" {
					size, _ = strconv.Atoi(m[3])
				}
				qubitBase[m[4]], qubitSize[m[4]] = total, size
				total += size
			}
			out = append(out, line)
			continue
		}
		if m := qasmRegRe.FindStringSubmatch(line); m != nil {
			if m[1] == "qreg" {
				size, _ := strconv.Atoi(m[3])
				qubitBase[m[2]], qubitSize[m[2]] = total, size
				total += size
			}
			out = append(out, line)
			continue
		}
		if strings.Contains(line, "measure") || strings.HasPrefix(line, "barrier") || strings.HasPrefix(line, "reset") {
			out = append(out, line)
			continue
		}
		m := qasmOpRe.FindStringSubmatch(line)
		if m == nil {
			return "", fmt.Errorf("line %d: %q is not supported by IBM native translation (control flow and custom gates must be unrolled first)", n+1, line)
		}
		name := strings.ToLower(m[1])
		var params []float64
		if m[2] != "" {
			for _, p := range splitTopLevel(m[3]) {
				v, err := evalAngle(p)
				if err != nil {
					return "", fmt.Errorf("line %d: angle %q: %w", n+1, p, err)
				}
				params = append(params, v)
			}
		}
		var qs []string
		var phys []int
		for _, a := range strings.Split(m[4], ",") {
			am := qasmArgRe.FindStringSubmatch(strings.TrimSpace(a))
			if am == nil {
				return "", fmt.Errorf("line %d: operand %q must be an indexed qubit like q[0]", n+1, strings.TrimSpace(a))
			}
			base, ok := qubitBase[am[1]]
			idx, _ := strconv.Atoi(am[2])
			if !ok || idx >= qubitSize[am[1]] {
				return "", fmt.Errorf("line %d: unknown qubit %s[%d]", n+1, am[1], idx)
			}
			qs = append(qs, am[1]+"["+am[2]+"]")
			phys = append(phys, base+idx)
		}
		ops, err := decomposeForIBM(name, params, len(qs))
		if err != nil {
			return "", fmt.Errorf("line %d: %w", n+1, err)
		}
		for _, op := range ops {
			if len(op.q) == 2 && !t.coupled(phys[op.q[0]], phys[op.q[1]]) {
				return "", fmt.Errorf("line %d: qubits %d and %d are not coupled on this backend (routing is not supported yet; use adjacent qubits)", n+1, phys[op.q[0]], phys[op.q[1]])
			}
			switch {
			case op.name == "rz":
				emit("rz(%s) %s;", formatAngle(op.angle), qs[op.q[0]])
			case len(op.q) == 2:
				emit("%s %s, %s;", op.name, qs[op.q[0]], qs[op.q[1]])
			default:
				emit("%s %s;", op.name, qs[op.q[0]])
			}
		}
	}
	if t.NumQubits > 0 && total > t.NumQubits {
		return "", fmt.Errorf("circuit uses %d qubits, backend has %d", total, t.NumQubits)
	}
	if !hasInclude {
		at := 0
		if len(out) > 0 && strings.HasPrefix(out[0], "OPENQASM") {
			at = 1
		}
		out = append(out[:at], append([]string{`include "stdgates.inc";`}, out[at:]...)...)
	}
	return strings.Join(out, "\n") + "\n", nil
}

type isaOp struct {
	name  string
	q     []int // indexes into the source gate's operands
	angle float64
}

func rz(a float64, q int) isaOp { return isaOp{name: "rz", q: []int{q}, angle: a} }
func sx(q int) isaOp            { return isaOp{name: "sx", q: []int{q}} }
func xg(q int) isaOp            { return isaOp{name: "x", q: []int{q}} }
func cz(a, b int) isaOp         { return isaOp{name: "cz", q: []int{a, b}} }

// uOps is U(θ,φ,λ) up to global phase: rz(λ) · sx · rz(θ+π) · sx · rz(φ+π).
func uOps(theta, phi, lam float64, q int) []isaOp {
	return []isaOp{rz(lam, q), sx(q), rz(theta+math.Pi, q), sx(q), rz(phi+math.Pi, q)}
}

func hOps(q int) []isaOp { return []isaOp{rz(math.Pi/2, q), sx(q), rz(math.Pi/2, q)} }

func cxOps(c, t int) []isaOp {
	ops := hOps(t)
	ops = append(ops, cz(c, t))
	return append(ops, hOps(t)...)
}

func cat(parts ...[]isaOp) []isaOp {
	var out []isaOp
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var ibmGateArity = map[string][2]int{ // name -> {qubits, params}
	"id": {1, 0}, "x": {1, 0}, "y": {1, 0}, "z": {1, 0}, "h": {1, 0},
	"s": {1, 0}, "sdg": {1, 0}, "t": {1, 0}, "tdg": {1, 0}, "sx": {1, 0}, "sxdg": {1, 0},
	"rx": {1, 1}, "ry": {1, 1}, "rz": {1, 1}, "p": {1, 1}, "phase": {1, 1}, "u1": {1, 1},
	"u2": {1, 2}, "u": {1, 3}, "u3": {1, 3},
	"cx": {2, 0}, "cnot": {2, 0}, "cz": {2, 0}, "cy": {2, 0}, "swap": {2, 0},
	"cp": {2, 1}, "cphase": {2, 1}, "crz": {2, 1}, "rzz": {2, 1},
}

func decomposeForIBM(name string, p []float64, nq int) ([]isaOp, error) {
	ar, ok := ibmGateArity[name]
	if !ok {
		return nil, fmt.Errorf("gate %q is not supported by IBM native translation", name)
	}
	if ar[0] != nq || ar[1] != len(p) {
		return nil, fmt.Errorf("gate %s takes %d qubit(s) and %d parameter(s)", name, ar[0], ar[1])
	}
	pi := math.Pi
	switch name {
	case "id":
		return nil, nil
	case "x":
		return []isaOp{xg(0)}, nil
	case "y":
		return []isaOp{rz(pi, 0), xg(0)}, nil
	case "z":
		return []isaOp{rz(pi, 0)}, nil
	case "s":
		return []isaOp{rz(pi/2, 0)}, nil
	case "sdg":
		return []isaOp{rz(-pi/2, 0)}, nil
	case "t":
		return []isaOp{rz(pi/4, 0)}, nil
	case "tdg":
		return []isaOp{rz(-pi/4, 0)}, nil
	case "h":
		return hOps(0), nil
	case "sx":
		return []isaOp{sx(0)}, nil
	case "sxdg":
		return []isaOp{rz(pi, 0), sx(0), rz(pi, 0)}, nil
	case "rz", "p", "phase", "u1":
		return []isaOp{rz(p[0], 0)}, nil
	case "rx":
		return uOps(p[0], -pi/2, pi/2, 0), nil
	case "ry":
		return uOps(p[0], 0, 0, 0), nil
	case "u2":
		return uOps(pi/2, p[0], p[1], 0), nil
	case "u", "u3":
		return uOps(p[0], p[1], p[2], 0), nil
	case "cz":
		return []isaOp{cz(0, 1)}, nil
	case "cx", "cnot":
		return cxOps(0, 1), nil
	case "cy":
		return cat([]isaOp{rz(-pi/2, 1)}, cxOps(0, 1), []isaOp{rz(pi/2, 1)}), nil
	case "swap":
		return cat(cxOps(0, 1), cxOps(1, 0), cxOps(0, 1)), nil
	case "cp", "cphase":
		l := p[0]
		return cat([]isaOp{rz(l/2, 0)}, cxOps(0, 1), []isaOp{rz(-l/2, 1)}, cxOps(0, 1), []isaOp{rz(l/2, 1)}), nil
	case "crz":
		th := p[0]
		return cat([]isaOp{rz(th/2, 1)}, cxOps(0, 1), []isaOp{rz(-th/2, 1)}, cxOps(0, 1)), nil
	case "rzz":
		return cat(cxOps(0, 1), []isaOp{rz(p[0], 1)}, cxOps(0, 1)), nil
	}
	return nil, fmt.Errorf("gate %q is not supported by IBM native translation", name)
}

func formatAngle(a float64) string {
	a = math.Remainder(a, 2*math.Pi)
	if math.Abs(a) < 1e-15 {
		a = 0
	}
	return strconv.FormatFloat(a, 'g', 17, 64)
}

func sortedBasis(b map[string]bool) []string {
	out := make([]string, 0, len(b))
	for k := range b {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

// evalAngle evaluates an OpenQASM angle expression: numbers, pi/π/tau, + - * /
// and parentheses.
func evalAngle(s string) (float64, error) {
	e := &angleParser{s: strings.ReplaceAll(s, " ", "")}
	v, err := e.expr()
	if err != nil {
		return 0, err
	}
	if e.i != len(e.s) {
		return 0, fmt.Errorf("unexpected %q", e.s[e.i:])
	}
	return v, nil
}

type angleParser struct {
	s string
	i int
}

func (p *angleParser) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

func (p *angleParser) expr() (float64, error) {
	v, err := p.term()
	for err == nil && (p.peek() == '+' || p.peek() == '-') {
		op := p.peek()
		p.i++
		var r float64
		if r, err = p.term(); op == '+' {
			v += r
		} else {
			v -= r
		}
	}
	return v, err
}

func (p *angleParser) term() (float64, error) {
	v, err := p.unary()
	for err == nil && (p.peek() == '*' || p.peek() == '/') {
		op := p.peek()
		p.i++
		var r float64
		if r, err = p.unary(); err != nil {
			break
		}
		if op == '*' {
			v *= r
		} else if r == 0 {
			return 0, fmt.Errorf("division by zero")
		} else {
			v /= r
		}
	}
	return v, err
}

func (p *angleParser) unary() (float64, error) {
	switch p.peek() {
	case '-':
		p.i++
		v, err := p.unary()
		return -v, err
	case '+':
		p.i++
		return p.unary()
	case '(':
		p.i++
		v, err := p.expr()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing )")
		}
		p.i++
		return v, nil
	}
	rest := p.s[p.i:]
	for _, c := range []struct {
		name string
		v    float64
	}{{"pi", math.Pi}, {"π", math.Pi}, {"tau", 2 * math.Pi}, {"τ", 2 * math.Pi}} {
		if strings.HasPrefix(rest, c.name) {
			p.i += len(c.name)
			return c.v, nil
		}
	}
	j := p.i
	for j < len(p.s) && (p.s[j] >= '0' && p.s[j] <= '9' || p.s[j] == '.' || p.s[j] == 'e' || p.s[j] == 'E' ||
		(j > p.i && (p.s[j] == '-' || p.s[j] == '+') && (p.s[j-1] == 'e' || p.s[j-1] == 'E'))) {
		j++
	}
	if j == p.i {
		return 0, fmt.Errorf("expected a number at %q", rest)
	}
	v, err := strconv.ParseFloat(p.s[p.i:j], 64)
	if err != nil {
		return 0, err
	}
	p.i = j
	return v, nil
}
