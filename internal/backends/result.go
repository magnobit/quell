// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package backends

import (
	"fmt"
	"sort"
	"strings"
)

// RunResult holds measurement counts from any hardware backend.
type RunResult struct {
	JobID   string
	Backend string
	Shots   int
	Counts  map[string]int // bit-string → count, e.g. "00" → 512
}

// Print writes a terminal histogram to stdout.
func (r *RunResult) Print() {
	fmt.Printf("\nResults — %s  (%d shots)\n", r.Backend, r.Shots)
	fmt.Printf("Job ID : %s\n\n", r.JobID)

	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(r.Counts))
	for k, v := range r.Counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})

	maxCount := 0
	if len(pairs) > 0 {
		maxCount = pairs[0].v
	}
	for _, p := range pairs {
		barLen := 0
		if maxCount > 0 {
			barLen = p.v * 30 / maxCount
		}
		bar := strings.Repeat("█", barLen)
		pct := float64(p.v) / float64(r.Shots) * 100
		fmt.Printf("  |%s⟩  %6.2f%%  %s\n", p.k, pct, bar)
	}
	fmt.Println()
}
