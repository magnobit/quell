// Copyright 2026 Magnobit, Inc. All rights reserved.

package ir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CanonicalVersion identifies the IR text schema hashed by Hash.
// Changing the format requires bumping this so old hashes stay meaningful.
const CanonicalVersion = "quell-ir-v1"

// CanonicalBytes is a deterministic, order-stable encoding of p.
// It does not use Go's struct dump or map iteration: field order is fixed,
// floats use strconv 'g'/-1 (shortest unique IEEE-754 form), and Params
// are sorted. Two programs that lower to the same operations therefore
// produce identical bytes regardless of PARAM declaration order.
func CanonicalBytes(p *Program) []byte {
	if p == nil {
		return []byte(CanonicalVersion + "\n<nil>\n")
	}
	var b strings.Builder
	b.Grow(256 + len(p.Ops)*32)
	fmt.Fprintf(&b, "%s\n", CanonicalVersion)
	fmt.Fprintf(&b, "num_qubits=%d\n", p.NumQubits)
	params := append([]string(nil), p.Params...)
	sort.Strings(params)
	fmt.Fprintf(&b, "params=%s\n", strings.Join(params, ","))
	writeFloatLine(&b, "noise_depolarizing", p.NoiseDepolarizing)
	writeFloatLine(&b, "noise_amplitude_damping", p.NoiseAmplitudeDamping)
	writeFloatLine(&b, "noise_phase_damping", p.NoisePhaseDamping)
	writeFloatLine(&b, "noise_readout", p.NoiseReadout)
	b.WriteString("ops\n")
	writeOps(&b, p.Ops, 0)
	return []byte(b.String())
}

// Hash returns sha256:<hex> of CanonicalBytes(p).
func Hash(p *Program) string {
	sum := sha256.Sum256(CanonicalBytes(p))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeFloatLine(b *strings.Builder, name string, v float64) {
	fmt.Fprintf(b, "%s=%s\n", name, formatFloat(v))
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func writeOps(b *strings.Builder, ops []Op, depth int) {
	pad := strings.Repeat("  ", depth)
	for _, op := range ops {
		fmt.Fprintf(b, "%s%s", pad, op.Kind)
		if len(op.Qubits) > 0 {
			fmt.Fprintf(b, " q=%s", joinInts(op.Qubits))
		}
		if len(op.Args) > 0 {
			fmt.Fprintf(b, " args=%s", joinFloats(op.Args))
		}
		if hasNamedArgs(op.ArgNames) {
			fmt.Fprintf(b, " argNames=%s", strings.Join(op.ArgNames, ","))
		}
		if op.Kind == OpIF || op.Kind == OpWHILE || op.Kind == OpASSERT || op.Kind == OpSWITCH {
			fmt.Fprintf(b, " cond=%d,%d,%d", op.CondCbit, op.CondEq, op.CondRightBit)
		}
		if op.MaxIter != 0 {
			fmt.Fprintf(b, " maxIter=%d", op.MaxIter)
		}
		if len(op.MeasTargets) > 0 {
			fmt.Fprintf(b, " meas=%s", joinInts(op.MeasTargets))
		}
		b.WriteByte('\n')
		if op.Body != nil {
			fmt.Fprintf(b, "%sbody\n", pad)
			writeOps(b, []Op{*op.Body}, depth+1)
		}
		if len(op.Then) > 0 {
			fmt.Fprintf(b, "%sthen\n", pad)
			writeOps(b, op.Then, depth+1)
		}
		if len(op.Else) > 0 {
			fmt.Fprintf(b, "%selse\n", pad)
			writeOps(b, op.Else, depth+1)
		}
		for _, arm := range op.Cases {
			if arm.Default {
				fmt.Fprintf(b, "%scase default\n", pad)
			} else {
				fmt.Fprintf(b, "%scase %d\n", pad, arm.Value)
			}
			writeOps(b, arm.Body, depth+1)
		}
	}
}

func hasNamedArgs(names []string) bool {
	for _, n := range names {
		if n != "" {
			return true
		}
	}
	return false
}

func joinInts(vs []int) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}

func joinFloats(vs []float64) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = formatFloat(v)
	}
	return strings.Join(parts, ",")
}
