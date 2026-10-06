// Copyright 2026 Magnobit, Inc. All rights reserved.

package optimizer

import (
	"time"

	"github.com/magnobit/quell/internal/ir"
)

// PassStat is one conservative pass. It does not change what Optimize returns.
type PassStat struct {
	Name    string
	Version string
	In      int
	Out     int
	Ns      int64
}

// Profile runs the same passes as Optimize and records counts and latency.
func Profile(p *ir.Program) (*ir.Program, []PassStat) {
	if p == nil {
		p = &ir.Program{}
	}
	cur := withOps(p, append([]ir.Op(nil), p.Ops...))
	var stats []PassStat
	for _, name := range []string{
		"zero_angle_elimination",
		"self_inverse_cancellation",
		"rotation_fusion",
		"zero_angle_elimination",
	} {
		start := time.Now()
		in := len(cur.Ops)
		next, _ := ApplyPass(cur, name)
		stats = append(stats, PassStat{
			Name: name, Version: "quell-optimizer-v1",
			In: in, Out: len(next.Ops), Ns: time.Since(start).Nanoseconds(),
		})
		cur = next
	}
	return cur, stats
}
