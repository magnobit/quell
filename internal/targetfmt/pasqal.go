// Copyright 2026 Magnobit, Inc. All rights reserved.

package targetfmt

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	pasqalSpacingUM = 5.0
	pasqalPulseNS   = 224
)

// pulserDigitalAnalogDevice is Sequence.to_abstract_repr() for
// pulser.devices.DigitalAnalogDevice (pulser 1.9.1). Azure emu-free
// rejected MockDevice at execution because that virtual device exposes
// three eigenstates (r, g, h). DigitalAnalogDevice is the device the
// official Pulser Bell example uses.
const pulserDigitalAnalogDevice = `{"name":"DigitalAnalogDevice","dimensions":2,"rydberg_level":70,"min_atom_distance":4,"max_atom_num":100,"max_radial_distance":50,"supports_slm_mask":true,"max_layout_filling":0.5,"reusable_channels":false,"pre_calibrated_layouts":[],"version":"1","pulser_version":"1.9.1","channels":[{"id":"rydberg_global","basis":"ground-rydberg","addressing":"Global","max_abs_detuning":125.66370614359172,"max_amp":15.707963267948966,"min_retarget_interval":null,"fixed_retarget_t":null,"max_targets":null,"clock_period":4,"min_duration":16,"max_duration":67108864,"mod_bandwidth":null,"eom_config":null},{"id":"rydberg_local","basis":"ground-rydberg","addressing":"Local","max_abs_detuning":125.66370614359172,"max_amp":62.83185307179586,"min_retarget_interval":220,"fixed_retarget_t":0,"max_targets":1,"clock_period":4,"min_duration":16,"max_duration":67108864,"mod_bandwidth":null,"eom_config":null},{"id":"raman_local","basis":"digital","addressing":"Local","max_abs_detuning":125.66370614359172,"max_amp":62.83185307179586,"min_retarget_interval":220,"fixed_retarget_t":0,"max_targets":1,"clock_period":4,"min_duration":16,"max_duration":67108864,"mod_bandwidth":null,"eom_config":null}],"dmm_objects":[{"id":"dmm_0","basis":"ground-rydberg","addressing":"Global","max_abs_detuning":null,"max_amp":0,"min_retarget_interval":null,"fixed_retarget_t":null,"max_targets":null,"clock_period":4,"min_duration":16,"max_duration":67108864,"mod_bandwidth":null,"eom_config":null,"bottom_detuning":-125.66370614359172,"total_bottom_detuning":-12566.370614359172}],"interaction_coeff_xy":36288.3559282823,"is_virtual":false}`

// pulserSeq is Sequence.to_abstract_repr() for pasqal.pulser.v1.
// Azure validates this against the Pulser schema: device, measurement,
// and pulse protocol must be present. Target atoms are register indices.
type pulserSeq struct {
	Version     string            `json:"version"`
	Name        string            `json:"name"`
	Register    []pulserQubit     `json:"register"`
	Channels    map[string]string `json:"channels"`
	Variables   map[string]any    `json:"variables"`
	Operations  []map[string]any  `json:"operations"`
	Measurement string            `json:"measurement"`
	Device      any               `json:"device"`
	PulserVer   string            `json:"pulser_version,omitempty"`
}

type pulserQubit struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type pulserBuilder struct {
	seq           pulserSeq
	digitalTarget int
	rydbergTarget int
	// rotChannel is where single-qubit rotations go. Entangling circuits
	// use only the Rydberg channel: emu-free cannot sample a sequence that
	// mixes Raman (g,h) with Rydberg (g,r), because the state is then
	// (r,g,h) and it cannot infer which eigenstate is |1>.
	rotChannel string
	phaseBasis string
}

func pulserSequence(c *Circuit) (*pulserSeq, error) {
	rydbergOnly := needsRydberg(c)
	b := &pulserBuilder{
		digitalTarget: -1,
		rydbergTarget: -1,
		rotChannel:    "digital",
		phaseBasis:    "digital",
		seq: pulserSeq{
			Version:     "1",
			Name:        "quell",
			Channels:    map[string]string{},
			Variables:   map[string]any{},
			Measurement: "digital",
			Device:      pulserDevice(),
			PulserVer:   "1.9.1",
		},
	}
	if rydbergOnly {
		b.rotChannel = "rydberg"
		b.phaseBasis = "ground-rydberg"
		b.seq.Measurement = "ground-rydberg"
		b.seq.Channels["rydberg"] = "rydberg_local"
	} else {
		b.seq.Channels["digital"] = "raman_local"
	}
	for i := 0; i < c.Qubits; i++ {
		b.seq.Register = append(b.seq.Register, pulserQubit{
			Name: qName(i),
			X:    float64(i) * pasqalSpacingUM,
			Y:    0,
		})
	}
	if c.Qubits > 0 {
		b.setTarget(b.rotChannel, 0)
	}
	for _, g := range c.Gates {
		if err := b.gate(g); err != nil {
			return nil, err
		}
	}
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
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op":      "phase_shift",
		"basis":   b.phaseBasis,
		"phi":     angle,
		"targets": []int{q},
	})
}

func (b *pulserBuilder) raman(q int, area, phase float64) {
	if math.Abs(area) < 1e-12 {
		return
	}
	b.setTarget(b.rotChannel, q)
	b.pulse(b.rotChannel, area, phase)
}

func (b *pulserBuilder) cz(c, t int) {
	b.align()
	b.rydberg(c, math.Pi)
	b.rydberg(t, 2*math.Pi)
	b.rydberg(c, math.Pi)
	b.align()
}

func (b *pulserBuilder) rydberg(q int, area float64) {
	b.setTarget("rydberg", q)
	b.pulse("rydberg", area, 0)
}

func (b *pulserBuilder) setTarget(channel string, q int) {
	cur := &b.digitalTarget
	if channel == "rydberg" {
		cur = &b.rydbergTarget
	}
	if *cur == q {
		return
	}
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op": "target", "channel": channel, "target": q,
	})
	*cur = q
}

func (b *pulserBuilder) pulse(channel string, area, phase float64) {
	amp := area / (float64(pasqalPulseNS) / 1000)
	wf := func(value float64) map[string]any {
		return map[string]any{"kind": "constant", "duration": pasqalPulseNS, "value": value}
	}
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op":               "pulse",
		"channel":          channel,
		"protocol":         "min-delay",
		"amplitude":        wf(amp),
		"detuning":         wf(0),
		"phase":            phase,
		"post_phase_shift": 0,
	})
}

func needsRydberg(c *Circuit) bool {
	for _, g := range c.Gates {
		switch g.Name {
		case "cx", "cz", "swap":
			return true
		}
	}
	return false
}

func (b *pulserBuilder) align() {
	if _, digital := b.seq.Channels["digital"]; !digital {
		return
	}
	if _, rydberg := b.seq.Channels["rydberg"]; !rydberg {
		return
	}
	b.seq.Operations = append(b.seq.Operations, map[string]any{
		"op":       "align",
		"channels": []string{"digital", "rydberg"},
	})
}

func pulserDevice() any {
	var device any
	if err := json.Unmarshal([]byte(pulserDigitalAnalogDevice), &device); err != nil {
		return "MockDevice"
	}
	return device
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
