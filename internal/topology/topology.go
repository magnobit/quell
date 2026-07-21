// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package topology defines coupling maps for hardware-aware routing.
package topology

import "fmt"

// CouplingMap is an undirected qubit connectivity graph.
type CouplingMap struct {
	Name  string
	Edges [][2]int
	adj   map[int][]int
}

// Preset returns a named map: "linear-N", "heavyhex-toy", or "fully-connected-N".
func Preset(name string) (*CouplingMap, error) {
	switch name {
	case "", "none":
		return nil, nil
	case "heavyhex-toy", "ibm-heavyhex-toy":
		return HeavyHexToy(), nil
	case "linear-5":
		return Linear(5), nil
	case "linear-7":
		return Linear(7), nil
	case "fully-connected-5":
		return FullyConnected(5), nil
	default:
		return nil, fmt.Errorf("unknown coupling map %q — try heavyhex-toy, linear-5, linear-7", name)
	}
}

// Linear is a 1D chain 0—1—…—(n-1).
func Linear(n int) *CouplingMap {
	if n < 2 {
		n = 2
	}
	edges := make([][2]int, 0, n-1)
	for i := 0; i < n-1; i++ {
		edges = append(edges, [2]int{i, i + 1})
	}
	return New("linear-"+itoa(n), edges)
}

// HeavyHexToy is a small heavy-hex fragment (12 qubits) for teaching routing.
// Layout (approx):
//
//	0—1—2
//	|  |  |
//	3—4—5—6
//	|  |  |
//	7—8—9—10—11
func HeavyHexToy() *CouplingMap {
	edges := [][2]int{
		{0, 1}, {1, 2},
		{0, 3}, {1, 4}, {2, 5},
		{3, 4}, {4, 5}, {5, 6},
		{3, 7}, {4, 8}, {5, 9}, {6, 10},
		{7, 8}, {8, 9}, {9, 10}, {10, 11},
	}
	return New("heavyhex-toy", edges)
}

// FullyConnected allows any pair (no SWAPs needed).
func FullyConnected(n int) *CouplingMap {
	if n < 2 {
		n = 2
	}
	var edges [][2]int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			edges = append(edges, [2]int{i, j})
		}
	}
	return New("fully-connected-"+itoa(n), edges)
}

// New builds a CouplingMap and adjacency list.
func New(name string, edges [][2]int) *CouplingMap {
	m := &CouplingMap{Name: name, Edges: edges, adj: map[int][]int{}}
	for _, e := range edges {
		a, b := e[0], e[1]
		m.adj[a] = append(m.adj[a], b)
		m.adj[b] = append(m.adj[b], a)
	}
	return m
}

// Connected reports whether a and b share an edge.
func (m *CouplingMap) Connected(a, b int) bool {
	if m == nil {
		return true
	}
	for _, n := range m.adj[a] {
		if n == b {
			return true
		}
	}
	return false
}

// ShortestPath returns a path a → … → b (inclusive), or nil if unreachable.
func (m *CouplingMap) ShortestPath(a, b int) []int {
	if m == nil || a == b {
		return []int{a}
	}
	if m.Connected(a, b) {
		return []int{a, b}
	}
	type node struct{ q, prev int }
	queue := []int{a}
	prev := map[int]int{a: -1}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nxt := range m.adj[cur] {
			if _, seen := prev[nxt]; seen {
				continue
			}
			prev[nxt] = cur
			if nxt == b {
				path := []int{b}
				for x := b; prev[x] != -1; x = prev[x] {
					path = append([]int{prev[x]}, path...)
				}
				return path
			}
			queue = append(queue, nxt)
		}
	}
	return nil
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
