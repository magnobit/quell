// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"
	"sort"
	"strings"
)

// RunResult holds measurement counts from any hardware or simulation backend.
// JSON tags are camelCase for Cloud / Labs API clients.
type RunResult struct {
	JobID     string         `json:"jobId"`
	Backend   string         `json:"backend"`   // human-readable label shown in CLI/UI
	Requested string         `json:"requested"` // catalog id: ibm, nvidia, dwave, …
	Engine    string         `json:"engine"`    // actual engine: cudaq, local-statevector, leap, local-sa, …
	FellBack  bool           `json:"fellBack"`  // true when Engine is a local substitute for Requested
	Shots     int            `json:"shots"`
	Counts    map[string]int `json:"counts"` // bit-string → count, e.g. "00" → 512
}

// NewResult builds a successful run that used the intended engine.
func NewResult(requested, engine, display, jobID string, shots int, counts map[string]int) *RunResult {
	return &RunResult{
		JobID:     jobID,
		Backend:   display,
		Requested: requested,
		Engine:    engine,
		FellBack:  false,
		Shots:     shots,
		Counts:    counts,
	}
}

// NewFallback builds a run that substituted a local engine for the requested backend.
func NewFallback(requested, engine, display, jobID string, shots int, counts map[string]int) *RunResult {
	r := NewResult(requested, engine, display, jobID, shots, counts)
	r.FellBack = true
	return r
}

// Print writes a terminal histogram to stdout.
func (r *RunResult) Print() {
	label := r.Backend
	if r.FellBack {
		label += " ⚠ local fallback"
	}
	fmt.Printf("\nResults — %s  (%d shots)\n", label, r.Shots)
	if r.Requested != "" || r.Engine != "" {
		fmt.Printf("Engine : requested=%s actual=%s fellBack=%v\n", r.Requested, r.Engine, r.FellBack)
	}
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
