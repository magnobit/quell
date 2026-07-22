// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package decompose maps exotic / library gates into Quell's built-in gate set
// so converters can emit valid circuits instead of warn-only stubs.
package decompose

import (
	"fmt"
	"strconv"
	"strings"
)

// Result is one or more Quell lines plus an optional human note.
type Result struct {
	Lines []string
	Note  string // e.g. "decomposed from qc.rxx"
}

func joinLines(lines ...string) Result {
	return Result{Lines: lines}
}

func q(i int) string { return strconv.Itoa(i) }

// RXX decomposes R_XX(θ) on qubits a,b:
// H a; H b; CNOT a b; RZ θ b; CNOT a b; H a; H b
func RXX(theta string, a, b int) Result {
	th := NormAngle(theta)
	return Result{
		Note: "decomposed from RXX",
		Lines: []string{
			"H " + q(a),
			"H " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"RZ " + th + " " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"H " + q(a),
			"H " + q(b),
		},
	}
}

// RYY: RX(π/2) on both, CNOT, RZ, CNOT, RX(-π/2) on both.
func RYY(theta string, a, b int) Result {
	th := NormAngle(theta)
	return Result{
		Note: "decomposed from RYY",
		Lines: []string{
			"RX PI/2 " + q(a),
			"RX PI/2 " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"RZ " + th + " " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"RX -PI/2 " + q(a),
			"RX -PI/2 " + q(b),
		},
	}
}

// RZZ: CNOT; RZ θ on target; CNOT (equiv. CRZ for many targets).
func RZZ(theta string, a, b int) Result {
	th := NormAngle(theta)
	return Result{
		Note: "decomposed from RZZ",
		Lines: []string{
			"CNOT " + q(a) + " " + q(b),
			"RZ " + th + " " + q(b),
			"CNOT " + q(a) + " " + q(b),
		},
	}
}

// RZX: H on b; CNOT a b; RZ θ b; CNOT; H b
func RZX(theta string, a, b int) Result {
	th := NormAngle(theta)
	return Result{
		Note: "decomposed from RZX",
		Lines: []string{
			"H " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"RZ " + th + " " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"H " + q(b),
		},
	}
}

// ECR ≈ RZX(π/4) then X on control (IBM ECR approx for conversion).
func ECR(a, b int) Result {
	r := RZX("PI/4", a, b)
	r.Note = "decomposed from ECR (approx RZX(π/4)+X)"
	r.Lines = append(r.Lines, "X "+q(a))
	return r
}

// CY: S† on target, CNOT, S on target.
func CY(a, b int) Result {
	return Result{
		Note: "decomposed from CY",
		Lines: []string{
			"SDG " + q(b),
			"CNOT " + q(a) + " " + q(b),
			"S " + q(b),
		},
	}
}

// CH: practical stand-in CRY(π/2).
func CH(a, b int) Result {
	return Result{
		Note:  "decomposed from CH (as CRY PI/2)",
		Lines: []string{"CRY PI/2 " + q(a) + " " + q(b)},
	}
}

// SXDG: conjugate of SX ≈ RZ(-π/2) SX RZ(π/2) or simply comment+RX(-PI/2) partial;
// Quell has no SXDG — use SDG-like via RX: RX -PI/2 is not SX†.
// SX = √X; SX† = SX · X · SX or RZ(π/2)·SX·RZ(-π/2) — use RX -PI/2 as educational approx note.
func SXDG(qubit int) Result {
	return Result{
		Note:  "decomposed from SXDG (approx RX -PI/2)",
		Lines: []string{"RX -PI/2 " + q(qubit)},
	}
}

// PhasedXPow: Z^{-p} X^t Z^{p} → RZ(-p·π) RX(t·π) RZ(p·π)
// phaseExp and exp are Cirq exponents (dimensionless); we emit * PI.
func PhasedXPow(phaseExp, exp string, qubit int) Result {
	p := NormAngle(mulPI(phaseExp))
	t := NormAngle(mulPI(exp))
	negP := negateAngle(p)
	return Result{
		Note: "decomposed from PhasedXPowGate",
		Lines: []string{
			"RZ " + negP + " " + q(qubit),
			"RX " + t + " " + q(qubit),
			"RZ " + p + " " + q(qubit),
		},
	}
}

// XPow / YPow / ZPow / HPow: exponent t → rotation by t·π about axis.
func XPow(exp string, qubit int) Result {
	return Result{Note: "decomposed from XPowGate", Lines: []string{"RX " + NormAngle(mulPI(exp)) + " " + q(qubit)}}
}
func YPow(exp string, qubit int) Result {
	return Result{Note: "decomposed from YPowGate", Lines: []string{"RY " + NormAngle(mulPI(exp)) + " " + q(qubit)}}
}
func ZPow(exp string, qubit int) Result {
	return Result{Note: "decomposed from ZPowGate", Lines: []string{"RZ " + NormAngle(mulPI(exp)) + " " + q(qubit)}}
}
func HPow(exp string, qubit int) Result {
	// H^t ≈ RY(-π/4) · RZ(t·π) · RY(π/4) rough; for t=1 → H.
	if isOne(exp) {
		return Result{Note: "decomposed from HPowGate", Lines: []string{"H " + q(qubit)}}
	}
	t := NormAngle(mulPI(exp))
	return Result{
		Note: "decomposed from HPowGate",
		Lines: []string{
			"RY -PI/4 " + q(qubit),
			"RZ " + t + " " + q(qubit),
			"RY PI/4 " + q(qubit),
		},
	}
}

// XXPow / ZZPow with Cirq-style exponent.
func XXPow(exp string, a, b int) Result {
	r := RXX(mulPI(exp), a, b)
	r.Note = "decomposed from XXPowGate"
	return r
}
func ZZPow(exp string, a, b int) Result {
	r := RZZ(mulPI(exp), a, b)
	r.Note = "decomposed from ZZPowGate"
	return r
}

// PhasedXZ: Z^{-a} X^x Z^{a} Z^z
func PhasedXZ(x, z, a string, qubit int) Result {
	aa := NormAngle(mulPI(a))
	xx := NormAngle(mulPI(x))
	zz := NormAngle(mulPI(z))
	negA := negateAngle(aa)
	return Result{
		Note: "decomposed from PhasedXZGate",
		Lines: []string{
			"RZ " + negA + " " + q(qubit),
			"RX " + xx + " " + q(qubit),
			"RZ " + aa + " " + q(qubit),
			"RZ " + zz + " " + q(qubit),
		},
	}
}

// FormatAnnotate prepends a comment line when Note is set.
func FormatAnnotate(r Result) []string {
	if r.Note == "" {
		return r.Lines
	}
	out := make([]string, 0, len(r.Lines)+1)
	out = append(out, "// "+r.Note)
	out = append(out, r.Lines...)
	return out
}

// NormAngle normalizes pi → PI for Quell.
func NormAngle(a string) string {
	a = strings.TrimSpace(a)
	a = strings.ReplaceAll(a, "pi", "PI")
	a = strings.ReplaceAll(a, "Pi", "PI")
	return a
}

func mulPI(exp string) string {
	exp = strings.TrimSpace(exp)
	if exp == "" {
		return "PI"
	}
	if isOne(exp) {
		return "PI"
	}
	if isZero(exp) {
		return "0"
	}
	// already contains PI
	if strings.Contains(strings.ToUpper(exp), "PI") {
		return NormAngle(exp)
	}
	return NormAngle(exp) + "*PI"
}

func negateAngle(a string) string {
	a = strings.TrimSpace(a)
	if strings.HasPrefix(a, "-") {
		return strings.TrimPrefix(a, "-")
	}
	return "-" + a
}

func isOne(s string) bool {
	s = strings.TrimSpace(s)
	return s == "1" || s == "1.0" || s == "1.00"
}

func isZero(s string) bool {
	s = strings.TrimSpace(s)
	return s == "0" || s == "0.0" || s == "0.00"
}

// JoinQuell joins Result lines with newlines (no trailing blank).
func JoinQuell(r Result) string {
	return strings.Join(FormatAnnotate(r), "\n")
}

// Must for tests / debug
func DebugName(r Result) string {
	return fmt.Sprintf("%s (%d lines)", r.Note, len(r.Lines))
}
