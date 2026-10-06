// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import "testing"

func TestZipAndGrid(t *testing.T) {
	zip, err := Zip([]string{"theta"}, [][]float64{{0, 0.1, 0.2}})
	if err != nil || len(zip) != 3 || zip[1]["theta"] != 0.1 {
		t.Fatal(zip, err)
	}
	grid, err := Grid([]string{"a", "b"}, [][]float64{{0, 1}, {2, 3}})
	if err != nil || len(grid) != 4 {
		t.Fatal(grid, err)
	}
	src := "PARAM theta\nRX theta 0\nMEASURE\n"
	bound, err := BindSweep(src, zip)
	if err != nil || len(bound) != 3 {
		t.Fatal(err, bound)
	}
}
