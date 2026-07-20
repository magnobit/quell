// Copyright 2026 Magnobit, Inc. All rights reserved.

package anneal

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// SampleLocal runs classical simulated annealing on a QUBO — always available
// with no Leap credentials. Used for CI, demos, and as a fallback when
// DWAVE_API_TOKEN is unset. Not a quantum annealer.
func SampleLocal(p *Problem, numReads int, seed int64) (*Result, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if numReads < 1 {
		numReads = 100
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	type acc struct {
		bits  []int
		e     float64
		count int
	}
	bucket := map[string]*acc{}

	for r := 0; r < numReads; r++ {
		bits := make([]int, p.NumVars)
		for i := range bits {
			bits[i] = rng.Intn(2)
		}
		e := energy(p, bits)
		// cool from T=2 → ~0
		t := 2.0
		for step := 0; step < 200*p.NumVars; step++ {
			i := rng.Intn(p.NumVars)
			bits[i] = 1 - bits[i]
			ne := energy(p, bits)
			de := ne - e
			if de <= 0 || rng.Float64() < math.Exp(-de/t) {
				e = ne
			} else {
				bits[i] = 1 - bits[i]
			}
			t *= 0.995
			if t < 1e-4 {
				t = 1e-4
			}
		}
		key := bitsKey(bits)
		if a, ok := bucket[key]; ok {
			a.count++
		} else {
			cp := append([]int(nil), bits...)
			bucket[key] = &acc{bits: cp, e: e, count: 1}
		}
	}

	out := &Result{Info: fmt.Sprintf("local simulated annealing (%d reads)", numReads), Samples: make([]Sample, 0, len(bucket))}
	for _, a := range bucket {
		out.Samples = append(out.Samples, Sample{Bits: a.bits, Energy: a.e, NumOccur: a.count})
	}
	sort.Slice(out.Samples, func(i, j int) bool {
		if out.Samples[i].Energy != out.Samples[j].Energy {
			return out.Samples[i].Energy < out.Samples[j].Energy
		}
		return out.Samples[i].NumOccur > out.Samples[j].NumOccur
	})
	return out, nil
}

func energy(p *Problem, bits []int) float64 {
	var e float64
	for i, b := range p.Linear {
		e += b * float64(bits[i])
	}
	for k, c := range p.Quadratic {
		e += c * float64(bits[k[0]]*bits[k[1]])
	}
	return e
}

func bitsKey(bits []int) string {
	b := make([]byte, len(bits))
	for i, v := range bits {
		if v != 0 {
			b[i] = '1'
		} else {
			b[i] = '0'
		}
	}
	return string(b)
}

// CountsMap flattens samples into bitstring→count for Cloud UI compatibility.
func (r *Result) CountsMap() map[string]int {
	m := map[string]int{}
	if r == nil {
		return m
	}
	for _, s := range r.Samples {
		m[bitsKey(s.Bits)] += s.NumOccur
	}
	return m
}
