// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "github.com/magnobit/quell/internal/topology"

// RoutingAssessment is the placement-side view of how a workload's
// required connectivity sits on a backend coupling map. Uses the same
// undirected CouplingMap / shortest-path model as
// optimizer.OptimizeWithOptions — one formula, not a second scorer.
type RoutingAssessment struct {
	RequiredPairs  int
	DirectPairs    int
	Fit            float64 // DirectPairs / RequiredPairs, 0 if none required
	EstimatedSWAPs int     // approximate SWAP insertions (path length − 2)
}

// AssessRouting reports native edge coverage and an approximate SWAP
// overhead. Empty coupling means unknown topology — callers must treat
// that as neutral, not as fit 0 or fit 1. Disconnected required pairs
// add a small SWAP penalty and never hard-reject.
func AssessRouting(required, coupling [][2]int) RoutingAssessment {
	out := RoutingAssessment{RequiredPairs: len(required)}
	if len(required) == 0 {
		return out
	}
	edges := make(map[[2]int]bool, len(coupling)*2)
	for _, e := range coupling {
		edges[e] = true
		edges[[2]int{e[1], e[0]}] = true
	}
	for _, pair := range required {
		if edges[pair] {
			out.DirectPairs++
		}
	}
	out.Fit = float64(out.DirectPairs) / float64(out.RequiredPairs)

	cm := topology.FromEdges("placement", coupling)
	if cm == nil {
		return out
	}
	for _, pair := range required {
		if edges[pair] {
			continue
		}
		path := cm.ShortestPath(pair[0], pair[1])
		if path == nil || len(path) < 2 {
			// No path: still executable after a larger remap; count a
			// modest penalty instead of treating the backend as impossible.
			out.EstimatedSWAPs += 2
			continue
		}
		if extra := len(path) - 2; extra > 0 {
			out.EstimatedSWAPs += extra
		}
	}
	return out
}

// AllToAllEdges is the canonical complete-graph edge list for a backend
// whose execution model is all-to-all. Not a teaching Preset.
func AllToAllEdges(n int) [][2]int {
	if n < 2 {
		return nil
	}
	return topology.AllToAll("all-to-all", n).Edges
}
