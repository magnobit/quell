// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
)

// These pin down real, verified gate/construct parity between what the
// parser accepts (parser.GateArity — the single source of truth also used
// by the LSP's hover/completion) and what each compile target can actually
// emit. This matters specifically because every one of the 9 live hardware
// backends (IBM, IonQ, AWS Braket, Google, Rigetti, Azure, D-Wave, NVIDIA,
// Intel — see quell/internal/backends) receives the *same* compiled
// OpenQASM 3 output; there is no per-backend gate translation anywhere in
// this codebase. So "does hardware backend X support gate Y" is genuinely
// a question about that provider's own transpiler, external to Quell and
// unverifiable from this repo — but "does Quell's own OpenQASM 3 compiler
// (target OpenQASM, the one actually used for hardware submission) support
// every gate its own parser accepts" is fully verifiable, and exactly the
// gap that actually mattered: a gate that parsed fine but wasn't handled
// by the live target would only surface as "unsupported gate" at compile
// time, with zero warning at submission time.
//
// The other five targets (OpenQASM2, Qiskit, Cirq, Braket, Q#) are export/
// interop formats, not the hardware-submission path, and some of them have
// real, deliberate limitations the compiler already reports clearly rather
// than emitting broken output for (OpenQASM 2.0 genuinely has no classical
// control-flow syntax; Cirq/Braket source export doesn't implement it yet).
// Those are covered by knownLimitations below so a *regression* — the
// error message changing, or a case that used to fail silently succeeding
// with wrong output — would still be caught, without the test wrongly
// treating an intentional, documented limitation as a bug.

// quellSource builds a minimal, valid one-line Quell program invoking name
// with placeholder qubit indices (0, 1, 2, ...) and a fixed angle (1.0) for
// any float arguments, per GateArity's arity and the parser's "angle
// arguments come before qubit indices" convention.
func quellSource(name string, spec parser.GateSpec) string {
	var parts []string
	parts = append(parts, name)
	for i := 0; i < spec.Args; i++ {
		parts = append(parts, "1.0")
	}
	n := spec.Qubits
	if n < 0 {
		n = 1 // variadic (MEASURE, BARRIER) — one qubit is a valid call
	}
	for i := 0; i < n; i++ {
		parts = append(parts, strconv.Itoa(i))
	}
	line := strings.Join(parts, " ")
	// Every program needs a MEASURE for the parser's own validation to be
	// satisfied without an unrelated warning masking a real compile error.
	if name == "MEASURE" {
		return line + "\n"
	}
	return line + "\nMEASURE\n"
}

// knownLimitation records a construct/target pair that's expected to fail,
// and a substring its error must contain — so a regression (a different,
// unexpected failure reason, e.g. the construct silently breaking in some
// new way) still fails the test instead of being masked by a blanket skip.
type knownLimitation struct {
	construct, target, errSubstring string
}

func limitationFor(list []knownLimitation, construct string, target Target) *knownLimitation {
	for _, l := range list {
		if l.construct == construct && l.target == string(target) {
			return &l
		}
	}
	return nil
}

// gateLimitations: OpenQASM 2.0's real, published gate library doesn't
// include these — the compiler correctly refuses to emit invalid QASM2
// rather than silently producing something a real QASM2 consumer would
// reject. None of this touches the live hardware path (target OpenQASM,
// i.e. QASM3), which supports all of them.
var gateLimitations = []knownLimitation{
	{"ISWAP", "openqasm2", "not representable in OpenQASM 2.0"},
	{"CRX", "openqasm2", "not representable in OpenQASM 2.0"},
	{"CRY", "openqasm2", "not representable in OpenQASM 2.0"},
	{"CSWAP", "openqasm2", "not representable in OpenQASM 2.0"},
}

// TestGateParity_EveryBuiltinGateCompilesOnEveryTarget is the actual
// parity check for fixed-arity gates: every name in parser.GateArity,
// compiled against every compile target, must succeed — except the
// specific, deliberate OpenQASM 2.0 gate-set gaps in gateLimitations,
// which must fail with that documented reason specifically (not some other,
// unexpected error).
func TestGateParity_EveryBuiltinGateCompilesOnEveryTarget(t *testing.T) {
	for name, spec := range parser.GateArity {
		src := quellSource(name, spec)
		for _, target := range Targets {
			t.Run(fmt.Sprintf("%s/%s", name, target), func(t *testing.T) {
				_, err := Compile(src, target)
				want := limitationFor(gateLimitations, name, target)
				if want == nil {
					if err != nil {
						t.Errorf("gate %s unsupported on target %s: %v\nsource:\n%s", name, target, err, src)
					}
					return
				}
				if err == nil {
					t.Errorf("gate %s on target %s started compiling successfully — remove it from gateLimitations if that's real support now, but verify the OUTPUT is actually correct first", name, target)
					return
				}
				if !strings.Contains(err.Error(), want.errSubstring) {
					t.Errorf("gate %s on target %s failed for a different reason than expected: got %q, want it to contain %q — this may be a real regression, not the known limitation", name, target, err.Error(), want.errSubstring)
				}
			})
		}
	}
}

var controlFlowLimitations = []knownLimitation{
	{"WHILE", "openqasm2", "not representable in OpenQASM 2.0"},
	{"SWITCH", "openqasm2", "not representable in OpenQASM 2.0"},
	{"PAR", "openqasm2", "not representable in OpenQASM 2.0"},
	{"ASSERT", "openqasm2", "not representable in OpenQASM 2.0"},
	{"IF", "braket", "not exported to Braket"},
	{"WHILE", "braket", "not exported to Braket"},
	{"WHILE", "cirq", "not exported to Cirq"},
	{"SWITCH", "cirq", "not exported to Cirq"},
	{"SWITCH", "braket", "not exported to Braket"},
	{"SWITCH", "qiskit", "not exported to Qiskit"},
	{"ASSERT", "cirq", "not exported to Cirq"},
	// Note: SWITCH/qsharp is intentionally absent — it actually compiles
	// successfully (confirmed by running this test), unlike the sibling
	// Cirq/Braket/Qiskit export targets.
}

// TestGateParity_ControlFlowCompilesOnEveryTarget covers the constructs
// GateArity doesn't (they're keywords, not fixed-arity gates): IF, WHILE,
// SWITCH, PAR, ASSERT, BARRIER, RESET. MEASURE is covered above via
// GateArity directly. Critically, every one of these must succeed on
// target OpenQASM specifically — that's the one every live hardware
// backend actually receives.
func TestGateParity_ControlFlowCompilesOnEveryTarget(t *testing.T) {
	cases := map[string]string{
		"BARRIER": "H 0\nBARRIER 0\nMEASURE\n",
		"RESET":   "H 0\nRESET 0\nMEASURE\n",
		"IF":      "H 0\nMEASURE\nIF c[0]==1 { X 1 }\nMEASURE\n",
		"WHILE":   "H 0\nMEASURE\nWHILE c[0]==0 MAX 3 { H 0\nMEASURE }\n",
		"SWITCH":  "H 0\nMEASURE\nSWITCH c {\nCASE 0: X 1\nDEFAULT: Y 1\n}\nMEASURE\n",
		"PAR":     "PAR { H 0\nH 1 }\nMEASURE\n",
		"ASSERT":  "H 0\nMEASURE\nASSERT c[0]==0\n",
	}
	for name, src := range cases {
		for _, target := range Targets {
			t.Run(fmt.Sprintf("%s/%s", name, target), func(t *testing.T) {
				_, err := Compile(src, target)
				want := limitationFor(controlFlowLimitations, name, target)
				if want == nil {
					if err != nil {
						t.Errorf("construct %s unsupported on target %s: %v\nsource:\n%s", name, target, err, src)
					}
					return
				}
				if err == nil {
					t.Errorf("construct %s on target %s started compiling successfully — remove it from controlFlowLimitations if that's real support now, but verify the OUTPUT is actually correct first, not just that it no longer errors", name, target)
					return
				}
				if !strings.Contains(err.Error(), want.errSubstring) {
					t.Errorf("construct %s on target %s failed for a different reason than expected: got %q, want it to contain %q — this may be a real regression, not the known limitation", name, target, err.Error(), want.errSubstring)
				}
			})
		}
	}
}
