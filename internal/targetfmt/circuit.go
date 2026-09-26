// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package targetfmt turns one OpenQASM circuit into the text or JSON each
// provider accepts. Azure Quantum calls it today. A direct Quantinuum,
// Rigetti, or Pasqal connection uses the same functions.
package targetfmt

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Gate is one operation in a provider-neutral circuit. Angles are radians.
type Gate struct {
	Name   string
	Qubits []int
	Angle  float64
}

// Circuit is the gate list shared by every provider emitter.
type Circuit struct {
	Qubits int
	Gates  []Gate
}

var (
	declRe = regexp.MustCompile(`^(qubit|bit)\s*(\[\s*(\d+)\s*\])?\s+([A-Za-z_]\w*)\s*;$`)
	regRe  = regexp.MustCompile(`^(qreg|creg)\s+([A-Za-z_]\w*)\s*\[\s*(\d+)\s*\]\s*;$`)
	opRe   = regexp.MustCompile(`^([A-Za-z_]\w*)\s*(\((.*)\))?\s+(.+);$`)
	argRe  = regexp.MustCompile(`^([A-Za-z_]\w*)\s*\[\s*(\d+)\s*\]$`)
)

// ParseOpenQASM reads an OpenQASM 2 or 3 circuit. Measurement is recorded as
// "measure every qubit at the end", which is what these providers do.
// Control flow and unknown gates are rejected.
func ParseOpenQASM(src string) (*Circuit, error) {
	src = strings.ReplaceAll(src, ";", ";\n")
	base := map[string]int{}
	size := map[string]int{}
	total := 0
	c := &Circuit{}
	use := func(q int) {
		if q+1 > c.Qubits {
			c.Qubits = q + 1
		}
	}
	resolve := func(token string) (int, error) {
		m := argRe.FindStringSubmatch(strings.TrimSpace(token))
		if m == nil {
			return 0, fmt.Errorf("qubit %q is not a register index", token)
		}
		idx, _ := strconv.Atoi(m[2])
		origin := base[m[1]]
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
		if m := declRe.FindStringSubmatch(line); m != nil {
			nSize := 1
			if m[3] != "" {
				nSize, _ = strconv.Atoi(m[3])
			}
			if m[1] == "qubit" {
				base[m[4]], size[m[4]] = total, nSize
				total += nSize
				if total > c.Qubits {
					c.Qubits = total
				}
			}
			continue
		}
		if m := regRe.FindStringSubmatch(line); m != nil {
			nSize, _ := strconv.Atoi(m[3])
			if m[1] == "qreg" {
				base[m[2]], size[m[2]] = total, nSize
				total += nSize
				if total > c.Qubits {
					c.Qubits = total
				}
			}
			continue
		}
		if strings.Contains(line, "measure") || strings.HasPrefix(line, "barrier") {
			continue
		}
		if strings.HasPrefix(line, "reset") || strings.HasPrefix(line, "if") || strings.HasPrefix(line, "for") {
			return nil, fmt.Errorf("line %d: %q cannot be translated", n+1, line)
		}
		m := opRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("line %d: %q is not supported", n+1, line)
		}
		name := strings.ToLower(m[1])
		var angle float64
		hasAngle := false
		if m[2] != "" {
			parts := splitComma(m[3])
			if len(parts) != 1 {
				return nil, fmt.Errorf("line %d: gate %s has %d angles; only one is translated", n+1, name, len(parts))
			}
			v, err := evalAngle(parts[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			angle, hasAngle = v, true
		}
		var qs []int
		for _, tok := range splitComma(m[4]) {
			q, err := resolve(tok)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			use(q)
			qs = append(qs, q)
		}
		g, err := normalize(name, hasAngle, angle, qs)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		if g.Name != "" {
			c.Gates = append(c.Gates, g)
		}
	}
	if c.Qubits < 1 {
		c.Qubits = 1
	}
	return c, nil
}

func normalize(name string, hasAngle bool, angle float64, qs []int) (Gate, error) {
	one := func(n string) (Gate, error) {
		if len(qs) != 1 || hasAngle {
			return Gate{}, fmt.Errorf("%s needs one qubit", n)
		}
		return Gate{Name: n, Qubits: qs}, nil
	}
	rot := func(n string) (Gate, error) {
		if len(qs) != 1 || !hasAngle {
			return Gate{}, fmt.Errorf("%s needs one qubit and one angle", n)
		}
		return Gate{Name: n, Qubits: qs, Angle: angle}, nil
	}
	switch name {
	case "id", "i":
		return Gate{}, nil
	case "h", "x", "y", "z", "s", "t", "sdg", "tdg", "sx", "sxdg":
		return one(name)
	case "rx", "ry", "rz":
		return rot(name)
	case "p", "phase", "u1":
		return rot("rz")
	case "cx", "cnot":
		if len(qs) != 2 || hasAngle {
			return Gate{}, fmt.Errorf("cx needs a control and a target")
		}
		return Gate{Name: "cx", Qubits: qs}, nil
	case "cz":
		if len(qs) != 2 || hasAngle {
			return Gate{}, fmt.Errorf("cz needs two qubits")
		}
		return Gate{Name: "cz", Qubits: qs}, nil
	case "swap":
		if len(qs) != 2 || hasAngle {
			return Gate{}, fmt.Errorf("swap needs two qubits")
		}
		return Gate{Name: "swap", Qubits: qs}, nil
	default:
		return Gate{}, fmt.Errorf("gate %q is not translated", name)
	}
}

func splitComma(s string) []string {
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

func evalAngle(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	s = strings.ReplaceAll(s, "π", "pi")
	s = strings.ReplaceAll(s, "pi", fmt.Sprintf("(%g)", math.Pi))
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v, nil
	}
	// number(op)number after pi substitution, one operator.
	for _, op := range []string{"*", "/"} {
		if i := strings.Index(s, op); i > 0 {
			left, right := s[:i], s[i+1:]
			a, err1 := parseParen(left)
			b, err2 := parseParen(right)
			if err1 != nil || err2 != nil {
				return 0, fmt.Errorf("angle %q is not a number or a pi expression", s)
			}
			if op == "*" {
				return a * b, nil
			}
			if b == 0 {
				return 0, fmt.Errorf("division by zero in angle")
			}
			return a / b, nil
		}
	}
	if strings.HasPrefix(s, "-") {
		v, err := parseParen(s[1:])
		if err != nil {
			return 0, err
		}
		return -v, nil
	}
	return parseParen(s)
}

func parseParen(s string) (float64, error) {
	s = strings.Trim(s, "()")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("angle %q is not a number or a pi expression", s)
	}
	return v, nil
}
