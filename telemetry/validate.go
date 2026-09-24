// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import (
	"math"
	"strings"
	"time"
)

const (
	maxPersistedQubits = 256
	maxPersistedGates  = 4096
)

// validProbability accepts a finite value in [0, 1]. Explicit zero is
// valid. NaN/Inf and values outside the unit interval are rejected.
func validProbability(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}

// validCoherence accepts a finite T1/T2 > 0 after unit normalization.
func validCoherence(seconds float64) bool {
	return !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds > 0
}

func validQubitIndex(i int) bool {
	return i >= 0 && i < maxPersistedQubits
}

// assignmentMinDet matches P2C's invertibility floor. A 2x2 built from
// IBM P(obs|true) off-diagonals is column-stochastic by construction
// (P(0|0)=1-P(1|0), P(1|1)=1-P(0|1)) and is only trusted when both
// directional assignment probabilities are present and the matrix is
// invertible.
const assignmentMinDet = 1e-4

// ValidAssignmentPair accepts IBM prob_meas0_prep1 (P0Given1) and
// prob_meas1_prep0 (P1Given0) from the same qubit. One-sided values,
// scalars, NaN/Inf, and singular matrices are rejected. Never derive
// these from readout_error.
func ValidAssignmentPair(p0given1, p1given0 float64) bool {
	if !validProbability(p0given1) || !validProbability(p1given0) {
		return false
	}
	if p0given1 >= 1 || p1given0 >= 1 {
		return false
	}
	return math.Abs(1-p0given1-p1given0) >= assignmentMinDet
}

// normalizeError converts a provider error figure to a probability in
// [0, 1] when the unit is known. Percent units are divided by 100.
// A missing unit with a value in [0, 1] is treated as probability.
// A missing unit with a value > 1 is rejected — we do not guess.
func normalizeError(value float64, unit string) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	u := strings.ToLower(strings.TrimSpace(unit))
	switch u {
	case "%", "percent", "percentage":
		value = value / 100
	case "", "probability", "frac", "fraction", "1":
	default:
		return nil
	}
	if !validProbability(value) {
		return nil
	}
	v := value
	return &v
}

// normalizeTimeToSeconds converts provider coherence times to seconds.
// IBM BackendProperties typically report T1/T2 in microseconds ("us").
func normalizeTimeToSeconds(value float64, unit string) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return nil
	}
	u := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(unit, "µ", "u")))
	var seconds float64
	switch u {
	case "", "s", "sec", "second", "seconds":
		seconds = value
	case "ms", "millisecond", "milliseconds":
		seconds = value / 1e3
	case "us", "µs", "microsecond", "microseconds":
		seconds = value / 1e6
	case "ns", "nanosecond", "nanoseconds":
		seconds = value / 1e9
	default:
		return nil
	}
	if !validCoherence(seconds) {
		return nil
	}
	return &seconds
}

func parseRFC3339(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

func classifyHTTPStatus(code int) string {
	switch {
	case code == 401:
		return "auth"
	case code == 403:
		return "permission"
	case code == 404:
		return "backend_unavailable"
	case code == 429:
		return "rate_limit"
	case code == 408 || code == 504:
		return "timeout"
	case code >= 500:
		return "provider_unavailable"
	case code >= 400:
		return "invalid_request"
	default:
		return ""
	}
}

func classifyNetErr(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(strings.ToLower(err.Error()), "timeout") ||
		strings.Contains(err.Error(), "deadline exceeded") ||
		strings.Contains(err.Error(), "context canceled") {
		return "timeout"
	}
	return "network"
}
