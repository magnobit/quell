// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package anneal is the QUBO / Ising surface for quantum annealers
// (D-Wave and similar). Gate-model Quell (.quell files) cannot run on
// annealers — this package is the separate program representation those
// devices need.
//
// Status: foundation only. Types and a minimal text format exist so
// Control Plane / Cloud / CLI can point at a real API shape; submission
// to D-Wave Leap is not wired yet.
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
//	# comment
//	n <num_vars>
//	h <i> <bias>
//	q <i> <j> <coupling>
//
// This is intentionally small so tooling and docs can share one shape
// before a richer .quell-anneal language lands.
func ParseQUBO(src string) (*Problem, error) {
	p := &Problem{
		Linear:    map[int]float64{},
		Quadratic: map[[2]int]float64{},
	}
	for n, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		switch strings.ToLower(parts[0]) {
		case "n":
			if len(parts) != 2 {
				return nil, fmt.Errorf("anneal: line %d: n expects 1 int", n+1)
			}
			v, err := strconv.Atoi(parts[1])
			if err != nil || v < 1 {
				return nil, fmt.Errorf("anneal: line %d: invalid n", n+1)
			}
			p.NumVars = v
		case "h":
			if len(parts) != 3 {
				return nil, fmt.Errorf("anneal: line %d: h expects i bias", n+1)
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
				return nil, fmt.Errorf("anneal: line %d: q expects i j coupling", n+1)
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
			return nil, fmt.Errorf("anneal: line %d: unknown directive %q", n+1, parts[0])
		}
	}
	if p.NumVars < 1 {
		return nil, fmt.Errorf("anneal: empty QUBO — declare n or at least one h/q term")
	}
	return p, nil
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
