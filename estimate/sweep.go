// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "fmt"

// Zip pairs axes by index. The axes must have the same length.
// Names are PARAM names. This is not a cartesian product.
func Zip(names []string, axes [][]float64) ([]map[string]float64, error) {
	if len(names) != len(axes) {
		return nil, fmt.Errorf("sweep zip: %d names and %d axes", len(names), len(axes))
	}
	if len(axes) == 0 {
		return nil, nil
	}
	n := len(axes[0])
	for _, ax := range axes {
		if len(ax) != n {
			return nil, fmt.Errorf("sweep zip: axes have different lengths")
		}
	}
	out := make([]map[string]float64, n)
	for i := 0; i < n; i++ {
		point := map[string]float64{}
		for a, name := range names {
			point[name] = axes[a][i]
		}
		out[i] = point
	}
	return out, nil
}

// Grid is the cartesian product of axes, in the order of names.
func Grid(names []string, axes [][]float64) ([]map[string]float64, error) {
	if len(names) != len(axes) {
		return nil, fmt.Errorf("sweep grid: %d names and %d axes", len(names), len(axes))
	}
	if len(axes) == 0 {
		return nil, nil
	}
	points := []map[string]float64{{}}
	for a, ax := range axes {
		next := make([]map[string]float64, 0, len(points)*len(ax))
		for _, base := range points {
			for _, v := range ax {
				point := map[string]float64{}
				for k, val := range base {
					point[k] = val
				}
				point[names[a]] = v
				next = append(next, point)
			}
		}
		points = next
	}
	return points, nil
}

// BindSweep parses and lowers once, then binds every point with BindMany.
func BindSweep(src string, points []map[string]float64) ([]string, error) {
	return BindMany(src, points)
}
