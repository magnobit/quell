// Copyright 2026 Magnobit, Inc. All rights reserved.

package targetfmt

import (
	"fmt"
	"math"
)

const (
	pasqalSpacingUM = 5.0
	pasqalPulseNS   = 200
)

type pulserSeq struct {
	Version    string            `json:"version"`
	Name       string            `json:"name"`
	Register   []pulserQubit     `json:"register"`
	Channels   map[string]string `json:"channels"`
	Variables  map[string]any    `json:"variables"`
	Operations []map[string]any  `json:"operations"`
}

type pulserQubit struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type pulserBuilder struct {
	seq           pulserSeq
	digitalTarget string
	rydbergTarget string
}

func pulserSequence(c *Circuit) (*pulserSeq, error) {
	b := &pulserBuilder{
		digitalTarget: "q0",
		rydbergTarget: "",
		seq: pulserSeq{
			Version:   "1",
			Name:      "quell",
			Channels:  map[string]string{"digital": "raman_local", "rydberg": "rydberg_local"},
			Variables: map[string]any{},
		},
	}
	for i := 0; i < c.Qubits; i++ {
		b.seq.Register = append(b.seq.Register, pulserQubit{
			Name: qName(i),
			X:    float64(i) * pasqalSpacingUM,
			Y:    0,
		})
	}
	for _, g := range c.Gates {
		if err := b.gate(g); err != nil {
			return nil, err
		}
	}
	b.align()
	return &b.seq, nil
}

func (b *pulserBuilder) gate(g Gate) error {
	switch g.Name {
	case "h":
		b.ry(g.Qubits[0], math.Pi/2)
		b.rx(g.Qubits[0], math.Pi)
	case "x":
		b.rx(g.Qubits[0], math.Pi)
	case "y":
		b.ry(g.Qubits[0], math.Pi)
	case "z":
		b.rz(g.Qubits[0], math.Pi)
	case "s":
		b.rz(g.Qubits[0], math.Pi/2)
	case "t":
		b.rz(g.Qubits[0], math.Pi/4)
	case "sdg":
		b.rz(g.Qubits[0], -math.Pi/2)
	case "tdg":
		b.rz(g.Qubits[0], -math.Pi/4)
	case "sx":
		b.rx(g.Qubits[0], math.Pi/2)
	case "sxdg":
		b.rx(g.Qubits[0], -math.Pi/2)
	case "rx":
		b.rx(g.Qubits[0], g.Angle)
	case "ry":
		b.ry(g.Qubits[0], g.Angle)
	case "rz":
		b.rz(g.Qubits[0], g.Angle)
	case "cz":
		b.cz(g.Qubits[0], g.Qubits[1])
	case "cx":
		b.ry(g.Qubits[1], math.Pi/2)
		b.rx(g.Qubits[1], math.Pi)
		b.cz(g.Qubits[0], g.Qubits[1])
		b.ry(g.Qubits[1], math.Pi/2)
		b.rx(g.Qubits[1], math.Pi)
	case "swap":
		b.gate(Gate{Name: "cx", Qubits: []int{g.Qubits[0], g.Qubits[1]}})
		b.gate(Gate{Name: "cx", Qubits: []int{g.Qubits[1], g.Qubits[0]}})
		b.gate(Gate{Name: "cx", Qubits: []int{g.Qubits[0], g.Qubits[1]}})
	default:
		return errGate(g.Name)
	}
	return nil
}

func errGate(name string) error {
	return fmt.Errorf("pasqal: gate %q is not translated", name)
}

func (b *pulserBuilder) rx(q int, angle float64) { b.raman(q, angle, 0) }
func (b *pulserBuilder) ry(q int, angle float64) { b.raman(q, angle, math.Pi/2) }

func (b *pulserBuilder) rz(q int, angle float64) {
	b.align()
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op":      "phase_shift",
		"channel": "digital",
		"phase":   angle,
		"targets": []string{qName(q)},
	})
}

func (b *pulserBuilder) raman(q int, area, phase float64) {
	if math.Abs(area) < 1e-12 {
		return
	}
	b.align()
	name := qName(q)
	if b.digitalTarget != name {
		b.seq.Operations = append(b.seq.Operations, map[string]any{
			"op": "target", "channel": "digital", "target": name,
		})
		b.digitalTarget = name
	}
	b.pulse("digital", area, phase)
}

func (b *pulserBuilder) cz(c, t int) {
	b.align()
	b.rydberg(c, math.Pi)
	b.rydberg(t, 2*math.Pi)
	b.rydberg(c, math.Pi)
}

func (b *pulserBuilder) rydberg(q int, area float64) {
	name := qName(q)
	if b.rydbergTarget != name {
		b.seq.Operations = append(b.seq.Operations, map[string]any{
			"op": "target", "channel": "rydberg", "target": name,
		})
		b.rydbergTarget = name
	}
	b.pulse("rydberg", area, 0)
}

func (b *pulserBuilder) pulse(channel string, area, phase float64) {
	amp := area / (float64(pasqalPulseNS) / 1000)
	wf := func(value float64) map[string]any {
		return map[string]any{"kind": "constant", "duration": pasqalPulseNS, "value": value}
	}
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op":               "pulse",
		"channel":          channel,
		"protocol":         "const",
		"amplitude":        wf(amp),
		"detuning":         wf(0),
		"phase":            phase,
		"post_phase_shift": 0,
	})
}

func (b *pulserBuilder) align() {
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op":       "align",
		"channels": []string{"digital", "rydberg"},
	})
}

func qName(i int) string { return "q" + itoa(i) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d [8]byte
	n := len(d)
	for i > 0 {
		n--
		d[n] = byte('0' + i%10)
		i /= 10
	}
	return string(d[n:])
}
