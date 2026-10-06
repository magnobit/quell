// Copyright 2026 Magnobit, Inc. All rights reserved.

package analysis

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseParams parses --param name=value flags. Values may be a float or a
// PI expression such as PI/2 or 2*PI. Empty input yields an empty map, not nil.
func ParseParams(flags []string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, s := range flags {
		eq := strings.IndexByte(s, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --param %q — expected name=radians (e.g. --param theta=1.5708)", s)
		}
		name := strings.TrimSpace(s[:eq])
		val := strings.TrimSpace(s[eq+1:])
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			if a, ok := evalAngleLite(val); ok {
				f = a
			} else {
				return nil, fmt.Errorf("invalid --param value %q: %w", val, err)
			}
		}
		out[name] = f
	}
	return out, nil
}

func evalAngleLite(tok string) (float64, bool) {
	upper := strings.ToUpper(tok)
	s := strings.ReplaceAll(upper, "PI", fmt.Sprintf("%g", 3.141592653589793))
	if i := strings.Index(s, "*"); i > 0 {
		a, e1 := strconv.ParseFloat(s[:i], 64)
		b, e2 := strconv.ParseFloat(s[i+1:], 64)
		if e1 == nil && e2 == nil {
			return a * b, true
		}
	}
	if i := strings.LastIndex(s, "/"); i > 0 {
		a, e1 := strconv.ParseFloat(s[:i], 64)
		b, e2 := strconv.ParseFloat(s[i+1:], 64)
		if e1 == nil && e2 == nil && b != 0 {
			return a / b, true
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}
