// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"
	"math"
	"strings"
)

// ionqGate is one entry in an IonQ ionq.circuit.v1 QIS circuit.
// Rotation is in turns (1 turn = 2π radians), which is what the IonQ job API expects.
type ionqGate struct {
	Gate     string   `json:"gate"`
	Target   *int     `json:"target,omitempty"`
	Targets  []int    `json:"targets,omitempty"`
	Control  *int     `json:"control,omitempty"`
	Rotation *float64 `json:"rotation,omitempty"`
}

// toIonQQIS turns an OpenQASM 2 or 3 circuit into the JSON gate list IonQ
// accepts. OpenQASM text is not an allowed input type. Measurement is omitted
// because IonQ measures every qubit when the job finishes. Gates that cannot
// be represented in the QIS gateset are rejected.
func toIonQQIS(src string, numQubits int) (int, []ionqGate, error) {
	qubits := numQubits
	if qubits < 1 {
		qubits = 1
	}
	base := map[string]int{}
	size := map[string]int{}
	total := 0
	var gates []ionqGate

	use := func(q int) error {
		if q < 0 {
			return fmt.Errorf("qubit index %d is negative", q)
		}
		if q >= qubits {
			qubits = q + 1
		}
		return nil
	}
	resolve := func(token string) (int, error) {
		token = strings.TrimSpace(token)
		m := qasmArgRe.FindStringSubmatch(token)
		if m == nil {
			return 0, fmt.Errorf("qubit %q is not a register index", token)
		}
		idx := 0
		fmt.Sscanf(m[2], "%d", &idx)
		origin, ok := base[m[1]]
		if !ok {
			origin = 0
		}
		if n, known := size[m[1]]; known && idx >= n {
			return 0, fmt.Errorf("qubit %s is outside register %s[%d]", token, m[1], n)
		}
		return origin + idx, nil
	}

	for n, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" || strings.HasPrefix(line, "OPENQASM") || strings.HasPrefix(line, "include") {
			continue
		}
		if m := qasmDeclRe.FindStringSubmatch(line); m != nil {
			nSize := 1
			if m[3] != "" {
				fmt.Sscanf(m[3], "%d", &nSize)
			}
			if m[1] == "qubit" {
				base[m[4]], size[m[4]] = total, nSize
				total += nSize
				if total > qubits {
					qubits = total
				}
			}
			continue
		}
		if m := qasmRegRe.FindStringSubmatch(line); m != nil {
			nSize := 0
			fmt.Sscanf(m[3], "%d", &nSize)
			if m[1] == "qreg" {
				base[m[2]], size[m[2]] = total, nSize
				total += nSize
				if total > qubits {
					qubits = total
				}
			}
			continue
		}
		if strings.Contains(line, "measure") || strings.HasPrefix(line, "barrier") {
			continue
		}
		if strings.HasPrefix(line, "reset") {
			return 0, nil, fmt.Errorf("line %d: reset is not part of the IonQ QIS gateset", n+1)
		}
		m := qasmOpRe.FindStringSubmatch(line)
		if m == nil {
			return 0, nil, fmt.Errorf("line %d: %q is not supported for IonQ", n+1, line)
		}
		name := strings.ToLower(m[1])
		var params []float64
		if m[2] != "" {
			for _, p := range splitTopLevel(m[3]) {
				v, err := evalAngle(p)
				if err != nil {
					return 0, nil, fmt.Errorf("line %d: angle %q: %w", n+1, p, err)
				}
				params = append(params, v)
			}
		}
		var qs []int
		for _, tok := range splitTopLevel(m[4]) {
			q, err := resolve(tok)
			if err != nil {
				return 0, nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			if err := use(q); err != nil {
				return 0, nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			qs = append(qs, q)
		}
		translated, err := ionqTranslate(name, params, qs)
		if err != nil {
			return 0, nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		gates = append(gates, translated...)
	}
	if gates == nil {
		gates = []ionqGate{}
	}
	return qubits, gates, nil
}

func ionqTranslate(name string, params []float64, qs []int) ([]ionqGate, error) {
	one := func(gate string) ([]ionqGate, error) {
		if len(qs) != 1 || len(params) != 0 {
			return nil, fmt.Errorf("%s needs one qubit", gate)
		}
		t := qs[0]
		return []ionqGate{{Gate: gate, Target: &t}}, nil
	}
	rot := func(gate string) ([]ionqGate, error) {
		if len(qs) != 1 || len(params) != 1 {
			return nil, fmt.Errorf("%s needs one qubit and one angle", gate)
		}
		t := qs[0]
		turns := params[0] / (2 * math.Pi)
		return []ionqGate{{Gate: gate, Target: &t, Rotation: &turns}}, nil
	}
	switch name {
	case "id", "i":
		return nil, nil
	case "h", "x", "y", "z", "s", "t":
		return one(name)
	case "sdg":
		return one("si")
	case "tdg":
		return one("ti")
	case "sx":
		return one("v")
	case "sxdg":
		return one("vi")
	case "rx", "ry", "rz":
		return rot(name)
	case "p", "phase", "u1":
		return rot("rz")
	case "cx", "cnot":
		if len(qs) != 2 || len(params) != 0 {
			return nil, fmt.Errorf("cnot needs a control and a target")
		}
		c, t := qs[0], qs[1]
		return []ionqGate{{Gate: "cnot", Control: &c, Target: &t}}, nil
	case "swap":
		if len(qs) != 2 || len(params) != 0 {
			return nil, fmt.Errorf("swap needs two qubits")
		}
		return []ionqGate{{Gate: "swap", Targets: []int{qs[0], qs[1]}}}, nil
	case "cz":
		if len(qs) != 2 || len(params) != 0 {
			return nil, fmt.Errorf("cz needs a control and a target")
		}
		c, t := qs[0], qs[1]
		return []ionqGate{
			{Gate: "h", Target: &t},
			{Gate: "cnot", Control: &c, Target: &t},
			{Gate: "h", Target: &t},
		}, nil
	default:
		return nil, fmt.Errorf("gate %q is not in the IonQ QIS gateset", name)
	}
}
