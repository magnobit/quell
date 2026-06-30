// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// Instruction represents a single parsed gate operation.
type Instruction struct {
	Gate      string
	Qubits    []int
	Args      []float64
	QubitNames []string // original names if named qubits were used
}

// Circuit is the parsed representation of a .quell source file.
type Circuit struct {
	Instructions []Instruction
	NumQubits    int
	QubitNames   map[string]int // named qubit registry (name → index)
}

// Parse parses Quell source code and returns a Circuit.
//
// Named qubit declarations:
//
//	qubit alice          → alice=0
//	qubit bob, charlie   → bob=1, charlie=2
//
// Gates can then reference qubits by name or by integer index.
func Parse(src string) (*Circuit, error) {
	var instructions []Instruction
	maxQubit := -1
	namedQubits := map[string]int{}
	nextNamedIdx := 0

	scanner := bufio.NewScanner(strings.NewReader(src))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		// strip inline comment
		raw := scanner.Text()
		if ci := strings.Index(raw, "//"); ci >= 0 {
			raw = raw[:ci]
		}
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		tokens := strings.Fields(line)
		keyword := strings.ToUpper(tokens[0])

		// ── Named qubit declaration ────────────────────────────────────────
		if keyword == "QUBIT" {
			for _, tok := range tokens[1:] {
				name := strings.Trim(tok, ", \t")
				if name == "" {
					continue
				}
				if _, exists := namedQubits[name]; !exists {
					namedQubits[name] = nextNamedIdx
					if nextNamedIdx > maxQubit {
						maxQubit = nextNamedIdx
					}
					nextNamedIdx++
				}
			}
			continue
		}

		gate := keyword
		var qubits []int
		var qubitNames []string
		var args []float64

		for _, tok := range tokens[1:] {
			// strip trailing commas (allow: CNOT alice, bob)
			tok = strings.TrimRight(tok, ",")
			if tok == "" {
				continue
			}
			// named qubit reference
			if idx, ok := namedQubits[tok]; ok {
				if idx > maxQubit {
					maxQubit = idx
				}
				qubits = append(qubits, idx)
				qubitNames = append(qubitNames, tok)
				continue
			}
			// integer qubit index
			if isInt(tok) {
				n, err := strconv.Atoi(tok)
				if err != nil {
					return nil, fmt.Errorf("line %d: invalid qubit index %q", lineNum, tok)
				}
				if n > maxQubit {
					maxQubit = n
				}
				qubits = append(qubits, n)
				qubitNames = append(qubitNames, tok)
				continue
			}
			// float angle
			if isFloat(tok) {
				f, _ := strconv.ParseFloat(tok, 64)
				args = append(args, f)
				continue
			}
			return nil, fmt.Errorf("line %d: unexpected token %q", lineNum, tok)
		}

		instructions = append(instructions, Instruction{
			Gate:       gate,
			Qubits:     qubits,
			Args:       args,
			QubitNames: qubitNames,
		})
	}

	nq := maxQubit + 1
	if nq < nextNamedIdx {
		nq = nextNamedIdx
	}

	return &Circuit{
		Instructions: instructions,
		NumQubits:    nq,
		QubitNames:   namedQubits,
	}, nil
}

func isInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func isFloat(s string) bool {
	if isInt(s) {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
