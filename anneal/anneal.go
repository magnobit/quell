// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package anneal is the QUBO / Ising surface for quantum annealers
// (D-Wave and similar). Gate-model Quell (.quell files) cannot run on
// annealers — this package is the separate program representation those
// devices need.
//
// Parse a .qubo text file, then SampleLeap (Ocean + token) or SampleLocal
// (classical SA). Cloud submits with kind=qubo; CLI uses `quell anneal run`.
package anneal

import (
	"fmt"
	"strconv"
	"strings"
)

// Problem is a quadratic unconstrained binary optimization (QUBO) problem:
// minimize xᵀ Q x over binary variables x ∈ {0,1}ⁿ.
type Problem struct {
	// Linear biases h[i] for variable i (optional; may be empty).
	Linear map[int]float64
	// Quadratic couplings Q[i][j] for i < j (and optionally diagonal).
	Quadratic map[[2]int]float64
	// NumVars is inferred from the highest index if zero.
	NumVars int
}

// Result is a sampled annealer solution.
type Result struct {
	Samples []Sample
	Info    string
}

// Sample is one bitstring with energy and occurrence count.
type Sample struct {
	Bits     []int
	Energy   float64
	NumOccur int
}

// ParseQUBO reads a minimal line-oriented QUBO format:
//
//	# comment   (also // … like Quell)
//	n <num_vars>
//	h <i> <bias>
//	q <i> <j> <coupling>
//
// This is intentionally small so tooling and docs can share one shape
// before a richer .quell-anneal language lands.
func ParseQUBO(src string) (*Problem, error) {
	if looksLikeGateQuell(src) {
		return nil, fmt.Errorf("anneal: this looks like gate Quell (H/CNOT/MEASURE) — D-Wave needs QUBO lines: n / h i bias / q i j coupling (not H 0)")
	}
	p := &Problem{
		Linear:    map[int]float64{},
		Quadratic: map[[2]int]float64{},
	}
	for n, line := range strings.Split(src, "\n") {
		line = stripQUBOComment(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		switch strings.ToLower(parts[0]) {
		case "n":
			if len(parts) != 2 {
				return nil, fmt.Errorf("anneal: line %d: n expects 1 int (example: n 2)", n+1)
			}
			v, err := strconv.Atoi(parts[1])
			if err != nil || v < 1 {
				return nil, fmt.Errorf("anneal: line %d: invalid n", n+1)
			}
			p.NumVars = v
		case "h":
			if len(parts) != 3 {
				return nil, fmt.Errorf("anneal: line %d: h expects \"h <i> <bias>\" (example: h 0 -1) — not gate Quell \"H 0\"", n+1)
			}
			i, err1 := strconv.Atoi(parts[1])
			b, err2 := strconv.ParseFloat(parts[2], 64)
			if err1 != nil || err2 != nil || i < 0 {
				return nil, fmt.Errorf("anneal: line %d: invalid h", n+1)
			}
			p.Linear[i] = b
			if i+1 > p.NumVars {
				p.NumVars = i + 1
			}
		case "q":
			if len(parts) != 4 {
				return nil, fmt.Errorf("anneal: line %d: q expects \"q <i> <j> <coupling>\" (example: q 0 1 2)", n+1)
			}
			i, err1 := strconv.Atoi(parts[1])
			j, err2 := strconv.Atoi(parts[2])
			c, err3 := strconv.ParseFloat(parts[3], 64)
			if err1 != nil || err2 != nil || err3 != nil || i < 0 || j < 0 {
				return nil, fmt.Errorf("anneal: line %d: invalid q", n+1)
			}
			if i > j {
				i, j = j, i
			}
			p.Quadratic[[2]int{i, j}] = c
			if j+1 > p.NumVars {
				p.NumVars = j + 1
			}
		default:
			return nil, fmt.Errorf("anneal: line %d: unknown directive %q — use n / h / q (QUBO), not gate names", n+1, parts[0])
		}
	}
	if p.NumVars < 1 {
		return nil, fmt.Errorf("anneal: empty QUBO — declare n or at least one h/q term")
	}
	return p, nil
}

// looksLikeGateQuell detects common gate-model source mistakenly submitted as QUBO.
func looksLikeGateQuell(src string) bool {
	upper := strings.ToUpper(src)
	gateHits := 0
	for _, g := range []string{"\nCNOT ", "\nCX ", "\nMEASURE", "\nRX ", "\nRY ", "\nRZ "} {
		if strings.Contains(upper, g) || strings.HasPrefix(strings.TrimSpace(upper), strings.TrimSpace(g)) {
			gateHits++
		}
	}
	// Lone "H 0" / "X 1" lines without QUBO n/h/q structure
	hasQUBO := regexpHasQUBODirective(src)
	if hasQUBO {
		return false
	}
	for _, line := range strings.Split(src, "\n") {
		line = stripQUBOComment(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		op := strings.ToUpper(fields[0])
		switch op {
		case "H", "X", "Y", "Z", "S", "T", "CNOT", "CX", "CZ", "SWAP", "MEASURE", "M", "RX", "RY", "RZ":
			return true
		}
	}
	return gateHits > 0
}

func regexpHasQUBODirective(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = stripQUBOComment(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "n", "h", "q":
			// Real QUBO h needs 3 fields; "h 0" alone is usually gate H 0
			if strings.ToLower(fields[0]) == "h" && len(fields) < 3 {
				continue
			}
			if strings.ToLower(fields[0]) == "n" || strings.ToLower(fields[0]) == "q" || len(fields) >= 3 {
				return true
			}
		}
	}
	return false
}

// stripQUBOComment removes # … and // … comments. Full-line comments become "".
// Inline comments after h/q terms are allowed so Quell-style editors feel familiar.
func stripQUBOComment(line string) string {
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return ""
	}
	if i := strings.Index(line, "//"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if i := strings.Index(line, "#"); i >= 0 {
		// Keep # inside numbers? Not needed for this format; treat as comment.
		line = strings.TrimSpace(line[:i])
	}
	return line
}

// Validate checks index ranges.
func (p *Problem) Validate() error {
	if p == nil || p.NumVars < 1 {
		return fmt.Errorf("anneal: empty problem")
	}
	for i := range p.Linear {
		if i < 0 || i >= p.NumVars {
			return fmt.Errorf("anneal: linear index %d out of range [0,%d)", i, p.NumVars)
		}
	}
	for k := range p.Quadratic {
		if k[0] < 0 || k[1] >= p.NumVars || k[0] > k[1] {
			return fmt.Errorf("anneal: quadratic index %v invalid for n=%d", k, p.NumVars)
		}
	}
	return nil
}
