// Copyright 2026 Magnobit, Inc. All rights reserved.

package livevalidate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Budget is a hard cap for paid/live provider calls. Exhaustion is a
// failed admission, not a bypass.
type Budget struct {
	MaxJobs     int
	MaxCost     float64
	MaxDuration time.Duration
	MaxShots    int
	Providers   map[string]bool
	AllowSubmit bool
	AllowSoak   bool
	Approved    bool
}

func budgetFromEnv() Budget {
	b := Budget{
		MaxJobs:     envInt("QUELL_LIVE_MAX_JOBS", 2),
		MaxCost:     envFloat("QUELL_LIVE_MAX_COST", 1.0),
		MaxDuration: envDuration("QUELL_LIVE_MAX_DURATION", 15*time.Minute),
		MaxShots:    envInt("QUELL_LIVE_MAX_SHOTS", 16),
		Providers:   map[string]bool{},
		AllowSubmit: os.Getenv("QUELL_LIVE_SUBMIT") == "1",
		AllowSoak:   os.Getenv("QUELL_LIVE_SOAK") == "1",
		Approved:    os.Getenv("QUELL_LIVE_BUDGET_APPROVED") == "1",
	}
	if b.MaxShots > 32 {
		b.MaxShots = 32
	}
	if b.MaxJobs > 8 {
		b.MaxJobs = 8
	}
	allow := strings.ToLower(strings.TrimSpace(os.Getenv("QUELL_LIVE_PROVIDERS")))
	if allow == "" {
		allow = "ibm,ionq"
	}
	for _, p := range strings.Split(allow, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			b.Providers[p] = true
		}
	}
	return b
}

func (b Budget) providerAllowed(id string) bool {
	return b.Providers[strings.ToLower(id)]
}

func (b Budget) admitSubmit(jobsUsed int, estimatedCost float64, started time.Time) error {
	if !b.AllowSubmit {
		return fmt.Errorf("live submit disabled (set QUELL_LIVE_SUBMIT=1)")
	}
	if !b.Approved {
		return fmt.Errorf("live budget not approved (set QUELL_LIVE_BUDGET_APPROVED=1)")
	}
	if jobsUsed >= b.MaxJobs {
		return fmt.Errorf("live budget exhausted: max_jobs=%d", b.MaxJobs)
	}
	if estimatedCost > b.MaxCost {
		return fmt.Errorf("live budget exhausted: estimated cost %.4f exceeds max_cost %.4f", estimatedCost, b.MaxCost)
	}
	if time.Since(started) > b.MaxDuration {
		return fmt.Errorf("live budget exhausted: max_duration=%s", b.MaxDuration)
	}
	return nil
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func envOr(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
