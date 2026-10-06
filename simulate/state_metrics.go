// Copyright 2026 Magnobit, Inc. All rights reserved.

package simulate

import (
	"fmt"
	"math"
	"math/cmplx"
)

// Probability returns the computational-basis probability of bits.
// bits is a left-to-right bit string whose width is the qubit count.
func (sv *StateVector) Probability(bits string) (float64, error) {
	idx, err := basisIndex(sv.N, bits)
	if err != nil {
		return 0, err
	}
	a := sv.amp[idx]
	return real(a)*real(a) + imag(a)*imag(a), nil
}

// Amplitude returns the complex amplitude of bits.
func (sv *StateVector) Amplitude(bits string) (complex128, error) {
	idx, err := basisIndex(sv.N, bits)
	if err != nil {
		return 0, err
	}
	return sv.amp[idx], nil
}

// Fidelity is |<a|b>|². Global phase does not change it.
func Fidelity(a, b *StateVector) (float64, error) {
	if a == nil || b == nil || a.dim != b.dim {
		return 0, fmt.Errorf("fidelity: state sizes differ")
	}
	var dot complex128
	for i := range a.amp {
		dot += cmplx.Conj(a.amp[i]) * b.amp[i]
	}
	return cmplx.Abs(dot) * cmplx.Abs(dot), nil
}

// TVD is half the L1 distance between the two computational distributions.
func TVD(a, b *StateVector) (float64, error) {
	if a == nil || b == nil || a.dim != b.dim {
		return 0, fmt.Errorf("tvd: state sizes differ")
	}
	var acc float64
	for i := range a.amp {
		pa := real(a.amp[i])*real(a.amp[i]) + imag(a.amp[i])*imag(a.amp[i])
		pb := real(b.amp[i])*real(b.amp[i]) + imag(b.amp[i])*imag(b.amp[i])
		acc += math.Abs(pa - pb)
	}
	return 0.5 * acc, nil
}

func basisIndex(n int, bits string) (int, error) {
	if len(bits) != n {
		return 0, fmt.Errorf("bit string width %d, circuit has %d qubits", len(bits), n)
	}
	idx := 0
	for i := 0; i < n; i++ {
		switch bits[i] {
		case '0':
		case '1':
			idx |= 1 << (n - 1 - i)
		default:
			return 0, fmt.Errorf("bit string %q", bits)
		}
	}
	return idx, nil
}
