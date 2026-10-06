// Copyright 2026 Magnobit, Inc. All rights reserved.

package engine

import (
	"fmt"
	"math/rand"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

// stabilizerMaxQubits is the tableau size this build will allocate.
// Reliable gates are H, S, SDG, CNOT, CZ, X, Y, Z, and computational measurement.
const stabilizerMaxQubits = 32

// Tableau is an Aaronson-Gottesman stabilizer tableau for Clifford circuits.
type Tableau struct {
	n int
	x [][]byte
	z [][]byte
	r []byte
}

func NewTableau(n int) *Tableau {
	if n < 1 {
		n = 1
	}
	t := &Tableau{
		n: n,
		x: make([][]byte, 2*n+1),
		z: make([][]byte, 2*n+1),
		r: make([]byte, 2*n+1),
	}
	for i := range t.x {
		t.x[i] = make([]byte, n)
		t.z[i] = make([]byte, n)
	}
	for i := 0; i < n; i++ {
		t.x[i][i] = 1
		t.z[n+i][i] = 1
	}
	return t
}

func (t *Tableau) H(a int) {
	for i := 0; i < 2*t.n; i++ {
		t.r[i] ^= t.x[i][a] & t.z[i][a]
		t.x[i][a], t.z[i][a] = t.z[i][a], t.x[i][a]
	}
}

func (t *Tableau) S(a int) {
	for i := 0; i < 2*t.n; i++ {
		t.r[i] ^= t.x[i][a] & t.z[i][a]
		t.z[i][a] ^= t.x[i][a]
	}
}

func (t *Tableau) CNOT(a, b int) {
	for i := 0; i < 2*t.n; i++ {
		t.r[i] ^= t.x[i][a] & t.z[i][b] & (t.x[i][b] ^ t.z[i][a] ^ 1)
		t.x[i][b] ^= t.x[i][a]
		t.z[i][a] ^= t.z[i][b]
	}
}

func (t *Tableau) X(a int) { t.H(a); t.S(a); t.S(a); t.H(a) }
func (t *Tableau) Z(a int) { t.S(a); t.S(a) }
func (t *Tableau) Y(a int) { t.Z(a); t.X(a) }

func (t *Tableau) rowsum(h, i int) {
	var g int
	for j := 0; j < t.n; j++ {
		if t.x[i][j] == 1 && t.z[h][j] == 1 {
			g++
		}
		if t.x[h][j] == 1 && t.z[i][j] == 1 {
			g--
		}
		t.x[h][j] ^= t.x[i][j]
		t.z[h][j] ^= t.z[i][j]
	}
	switch ((g % 4) + 4) % 4 {
	case 0:
		t.r[h] ^= t.r[i]
	case 2:
		t.r[h] ^= t.r[i] ^ 1
	default:
		t.r[h] ^= t.r[i]
	}
}

// MeasureZ returns a computational-basis bit for qubit a and updates the tableau.
func (t *Tableau) MeasureZ(a int, rng *rand.Rand) int {
	p := -1
	for i := t.n; i < 2*t.n; i++ {
		if t.x[i][a] == 1 {
			p = i
			break
		}
	}
	if p < 0 {
		for j := 0; j < t.n; j++ {
			t.x[2*t.n][j] = 0
			t.z[2*t.n][j] = 0
		}
		t.r[2*t.n] = 0
		for i := 0; i < t.n; i++ {
			if t.x[i][a] == 1 {
				t.rowsum(2*t.n, i+t.n)
			}
		}
		return int(t.r[2*t.n])
	}
	for i := 0; i < 2*t.n; i++ {
		if i != p && t.x[i][a] == 1 {
			t.rowsum(i, p)
		}
	}
	copy(t.x[p-t.n], t.x[p])
	copy(t.z[p-t.n], t.z[p])
	t.r[p-t.n] = t.r[p]
	for j := 0; j < t.n; j++ {
		t.x[p][j] = 0
		t.z[p][j] = 0
	}
	t.z[p][a] = 1
	outcome := 0
	if rng.Intn(2) == 1 {
		outcome = 1
	}
	t.r[p] = byte(outcome)
	return outcome
}

func cliffordKind(k ir.OpKind) bool {
	switch k {
	case ir.OpH, ir.OpX, ir.OpY, ir.OpZ, ir.OpS, ir.OpSDG, ir.OpCNOT, ir.OpCZ, ir.OpMEASURE, ir.OpBARRIER:
		return true
	default:
		return false
	}
}

// RunStabilizer samples a Clifford circuit. Non-Clifford programs return an error.
func RunStabilizer(src string, shots int, seed int64) (map[string]int, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	p := ir.Lower(c)
	for _, op := range p.Ops {
		if !cliffordKind(op.Kind) || len(op.Then) > 0 || len(op.Else) > 0 {
			return nil, fmt.Errorf("engine stabilizer is partial: %s is outside the Clifford subset", op.Kind)
		}
	}
	if shots <= 0 {
		shots = 1
	}
	n := p.NumQubits
	if n < 1 {
		n = 1
	}
	if n > stabilizerMaxQubits {
		return nil, fmt.Errorf("engine stabilizer supports at most %d qubits", stabilizerMaxQubits)
	}
	rng := rand.New(rand.NewSource(seed))
	counts := map[string]int{}
	for s := 0; s < shots; s++ {
		tab := NewTableau(n)
		var bits []byte
		for _, op := range p.Ops {
			if err := applyClifford(tab, op, rng, &bits); err != nil {
				return nil, err
			}
		}
		if bits == nil {
			bits = make([]byte, n)
			for q := 0; q < n; q++ {
				bits[q] = byte('0' + tab.MeasureZ(q, rng))
			}
		}
		counts[string(bits)]++
	}
	return counts, nil
}

func applyClifford(t *Tableau, op ir.Op, rng *rand.Rand, bits *[]byte) error {
	q := func(i int) int {
		if i < len(op.Qubits) {
			return op.Qubits[i]
		}
		return 0
	}
	switch op.Kind {
	case ir.OpH:
		t.H(q(0))
	case ir.OpX:
		t.X(q(0))
	case ir.OpY:
		t.Y(q(0))
	case ir.OpZ:
		t.Z(q(0))
	case ir.OpS:
		t.S(q(0))
	case ir.OpSDG:
		t.S(q(0))
		t.S(q(0))
		t.S(q(0))
	case ir.OpCNOT:
		t.CNOT(q(0), q(1))
	case ir.OpCZ:
		t.H(q(1))
		t.CNOT(q(0), q(1))
		t.H(q(1))
	case ir.OpMEASURE:
		if len(*bits) == 0 {
			*bits = make([]byte, t.n)
			for i := range *bits {
				(*bits)[i] = '0'
			}
		}
		if len(op.Qubits) == 0 {
			for i := 0; i < t.n; i++ {
				(*bits)[i] = byte('0' + t.MeasureZ(i, rng))
			}
			return nil
		}
		for _, qubit := range op.Qubits {
			if qubit >= 0 && qubit < t.n {
				(*bits)[qubit] = byte('0' + t.MeasureZ(qubit, rng))
			}
		}
	case ir.OpBARRIER:
	default:
		return fmt.Errorf("engine stabilizer cannot apply %s", op.Kind)
	}
	return nil
}
