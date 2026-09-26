// Copyright 2026 Magnobit, Inc. All rights reserved.

package targetfmt

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Payload is one provider job body. Direct connections and Azure both use it.
type Payload struct {
	Provider     string
	InputFormat  string
	OutputFormat string
	Body         []byte
}

// Azure chooses the format from the target id (quantinuum.sim.h2-1e, rigetti.sim.qvm, pasqal.sim.emu-free).
func Azure(target, openQASM string) (Payload, error) {
	provider := target
	if i := strings.IndexByte(target, '.'); i > 0 {
		provider = target[:i]
	}
	switch provider {
	case "quantinuum":
		body, err := QuantinuumOpenQASM(openQASM)
		if err != nil {
			return Payload{}, err
		}
		return Payload{
			Provider:     provider,
			InputFormat:  "honeywell.openqasm.v1",
			OutputFormat: "honeywell.quantum-results.v1",
			Body:         []byte(body),
		}, nil
	case "rigetti":
		body, err := RigettiQuil(openQASM)
		if err != nil {
			return Payload{}, err
		}
		return Payload{
			Provider:     provider,
			InputFormat:  "rigetti.quil.v1",
			OutputFormat: "rigetti.quil-results.v1",
			Body:         []byte(body),
		}, nil
	case "pasqal":
		body, err := PasqalPulser(openQASM)
		if err != nil {
			return Payload{}, err
		}
		return Payload{
			Provider:     provider,
			InputFormat:  "pasqal.pulser.v1",
			OutputFormat: "pasqal.pulser-results.v1",
			Body:         body,
		}, nil
	default:
		return Payload{}, fmt.Errorf("azure target %q has no Quell translation; set extra inputDataFormat to send a prepared body", target)
	}
}

// QuantinuumOpenQASM emits OpenQASM 2.0 for honeywell.openqasm.v1.
// A direct Quantinuum connection sends this same text.
func QuantinuumOpenQASM(openQASM string) (string, error) {
	c, err := ParseOpenQASM(openQASM)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[%d];\ncreg c[%d];\n", c.Qubits, c.Qubits)
	for _, g := range c.Gates {
		line, err := qasm2(g)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("measure q -> c;\n")
	return b.String(), nil
}

func qasm2(g Gate) (string, error) {
	q := func(i int) string { return fmt.Sprintf("q[%d]", g.Qubits[i]) }
	switch g.Name {
	case "h", "x", "y", "z", "s", "t", "sdg", "tdg":
		return fmt.Sprintf("%s %s;", g.Name, q(0)), nil
	case "sx":
		return fmt.Sprintf("rx(%s) %s;", strconvAngle(math.Pi/2), q(0)), nil
	case "sxdg":
		return fmt.Sprintf("rx(%s) %s;", strconvAngle(-math.Pi/2), q(0)), nil
	case "rx", "ry", "rz":
		return fmt.Sprintf("%s(%s) %s;", g.Name, strconvAngle(g.Angle), q(0)), nil
	case "cx":
		return fmt.Sprintf("cx %s, %s;", q(0), q(1)), nil
	case "cz":
		return fmt.Sprintf("cz %s, %s;", q(0), q(1)), nil
	case "swap":
		return fmt.Sprintf("swap %s, %s;", q(0), q(1)), nil
	default:
		return "", fmt.Errorf("quantinuum: gate %q is not translated", g.Name)
	}
}

// Rigetti program formats. Quil is the default and the Azure Quantum input.
// OpenQASM 3 is the direct Rigetti program format, kept so a connection can
// switch without another translator.
const (
	RigettiFormatQuil  = "quil"
	RigettiFormatQASM3 = "openqasm3"
)

// RigettiProgram returns the program text and the format name to send.
// format "" or "quil" emits Quil. "openqasm3" and "qasm3" return the
// OpenQASM source unchanged.
func RigettiProgram(openQASM, format string) (programFormat, source string, err error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", RigettiFormatQuil:
		source, err = RigettiQuil(openQASM)
		if err != nil {
			return "", "", err
		}
		return RigettiFormatQuil, source, nil
	case RigettiFormatQASM3, "qasm3", "qasm":
		return RigettiFormatQASM3, openQASM, nil
	default:
		return "", "", fmt.Errorf("program format %q must be %s or %s", format, RigettiFormatQuil, RigettiFormatQASM3)
	}
}

// RigettiQuil emits Quil. Azure Quantum uploads it as rigetti.quil.v1.
// RunRigetti sends this same text when program_format is quil.
func RigettiQuil(openQASM string) (string, error) {
	c, err := ParseOpenQASM(openQASM)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DECLARE ro BIT[%d]\n", c.Qubits)
	for _, g := range c.Gates {
		line, err := quil(g)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for i := 0; i < c.Qubits; i++ {
		fmt.Fprintf(&b, "MEASURE %d ro[%d]\n", i, i)
	}
	return b.String(), nil
}

func quil(g Gate) (string, error) {
	switch g.Name {
	case "h", "x", "y", "z", "s", "t":
		return fmt.Sprintf("%s %d", strings.ToUpper(g.Name), g.Qubits[0]), nil
	case "sdg":
		return fmt.Sprintf("RZ(%s) %d", strconvAngle(-math.Pi/2), g.Qubits[0]), nil
	case "tdg":
		return fmt.Sprintf("RZ(%s) %d", strconvAngle(-math.Pi/4), g.Qubits[0]), nil
	case "sx", "sxdg":
		angle := math.Pi / 2
		if g.Name == "sxdg" {
			angle = -angle
		}
		return fmt.Sprintf("RX(%s) %d", strconvAngle(angle), g.Qubits[0]), nil
	case "rx", "ry", "rz":
		return fmt.Sprintf("%s(%s) %d", strings.ToUpper(g.Name), strconvAngle(g.Angle), g.Qubits[0]), nil
	case "cx":
		return fmt.Sprintf("CNOT %d %d", g.Qubits[0], g.Qubits[1]), nil
	case "cz":
		return fmt.Sprintf("CZ %d %d", g.Qubits[0], g.Qubits[1]), nil
	case "swap":
		return fmt.Sprintf("SWAP %d %d", g.Qubits[0], g.Qubits[1]), nil
	default:
		return "", fmt.Errorf("rigetti: gate %q is not translated", g.Name)
	}
}

// PasqalPulser emits the pasqal.pulser.v1 JSON envelope.
// A direct Pasqal connection sends this same document.
//
// Single-qubit gates become local Raman pulses. H is RY(π/2) then RX(π),
// which equals H up to a global phase. CX is H on the target, a local
// Rydberg CZ, then H again. Atoms sit 5 µm apart so a pairwise Rydberg
// pulse blockades that pair and not the next atom in the line.
func PasqalPulser(openQASM string) ([]byte, error) {
	c, err := ParseOpenQASM(openQASM)
	if err != nil {
		return nil, err
	}
	seq, err := pulserSequence(c)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"sequence_builder": seq})
	if err != nil {
		return nil, err
	}
	return body, nil
}

func strconvAngle(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", v), "0"), ".")
}
