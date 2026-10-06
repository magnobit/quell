// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Point is one sweep coordinate and the source BindMany produced for it.
type Point struct {
	Index      int
	Params     map[string]float64
	Bound      string
	Err        string
	Group      string
	Provenance string
	Cancelled  bool
}

// Run binds points until cancel is closed. BindMany is the only binder.
// A closed cancel channel keeps the points already bound and marks the rest cancelled.
func Run(src, kind, group string, points []map[string]float64, cancel <-chan struct{}) ([]Point, error) {
	if kind != "zip" && kind != "grid" && kind != "list" {
		return nil, fmt.Errorf("sweep kind %q is not zip, grid, or list", kind)
	}
	var out []Point
	for i, params := range points {
		if cancelled(cancel) {
			out = append(out, Point{
				Index: i, Params: params, Group: group, Cancelled: true,
				Provenance: pointHash(kind, group, i, params),
			})
			continue
		}
		bound, err := BindMany(src, []map[string]float64{params})
		pt := Point{Index: i, Params: params, Group: group, Provenance: pointHash(kind, group, i, params)}
		if err != nil {
			pt.Err = err.Error()
			out = append(out, pt)
			continue
		}
		if len(bound) == 1 {
			pt.Bound = bound[0]
		}
		out = append(out, pt)
	}
	return out, nil
}

func cancelled(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func pointHash(kind, group string, index int, params map[string]float64) string {
	b, _ := json.Marshal(struct {
		Kind   string             `json:"kind"`
		Group  string             `json:"group"`
		Index  int                `json:"index"`
		Params map[string]float64 `json:"params"`
	}{kind, group, index, params})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ParameterShift is the two-point shift rule. eval must bind through BindMany.
func ParameterShift(point map[string]float64, name string, shift float64, eval func(map[string]float64) (float64, error)) (float64, error) {
	if shift == 0 {
		return 0, fmt.Errorf("parameter shift requires a non-zero shift")
	}
	plus := clone(point)
	minus := clone(point)
	plus[name] = point[name] + shift
	minus[name] = point[name] - shift
	a, err := eval(plus)
	if err != nil {
		return 0, err
	}
	b, err := eval(minus)
	if err != nil {
		return 0, err
	}
	return (a - b) / (2 * shift), nil
}

// FiniteDifference is the central difference. eval must bind through BindMany.
func FiniteDifference(point map[string]float64, name string, step float64, eval func(map[string]float64) (float64, error)) (float64, error) {
	return ParameterShift(point, name, step, eval)
}

func clone(in map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
