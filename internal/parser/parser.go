package parser

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

type Instruction struct {
	Gate   string
	Qubits []int
	Args   []float64
}

type Circuit struct {
	Instructions []Instruction
	NumQubits    int
}

func Parse(src string) (*Circuit, error) {
	var instructions []Instruction
	maxQubit := -1

	scanner := bufio.NewScanner(strings.NewReader(src))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		tokens := strings.Fields(line)
		gate := strings.ToUpper(tokens[0])

		var qubits []int
		var args []float64

		for _, tok := range tokens[1:] {
			if isFloat(tok) {
				f, _ := strconv.ParseFloat(tok, 64)
				args = append(args, f)
			} else if isInt(tok) {
				n, err := strconv.Atoi(tok)
				if err != nil {
					return nil, fmt.Errorf("line %d: invalid qubit index %q", lineNum, tok)
				}
				if n > maxQubit {
					maxQubit = n
				}
				qubits = append(qubits, n)
			} else {
				return nil, fmt.Errorf("line %d: unexpected token %q", lineNum, tok)
			}
		}

		instructions = append(instructions, Instruction{
			Gate:   gate,
			Qubits: qubits,
			Args:   args,
		})
	}

	return &Circuit{
		Instructions: instructions,
		NumQubits:    maxQubit + 1,
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
