// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"crypto/rand"
	"fmt"
	"strconv"
)

// mergeExtra writes cfg.Extra into body, so a provider-added job parameter
// that doesn't have a typed config field yet can still be sent — set it via
// `extra:` in quell.config.yml or `quell run --set <backend>.<key>=<value>`.
// Each value is coerced to bool/int64/float64 when it parses as one,
// otherwise sent as a string, so numeric/boolean provider fields come
// through as real JSON types rather than quoted strings.
//
// This only reaches parameters that belong in the request body a typed
// field already writes into (e.g. a new IBM Sampler option, a new Braket
// top-level field) — a provider introducing a wholly new endpoint or auth
// flow still needs a real code change, not just a new --set value.
func mergeExtra(body map[string]any, extra map[string]string) {
	for k, v := range extra {
		body[k] = coerceExtraValue(v)
	}
}

// notifySubmitted fires a hosted-caller's submit hook so the provider job
// id can be persisted before poll starts — cancel needs that id while the
// job is still running.
func notifySubmitted(fn func(string), jobID string) {
	if fn != nil && jobID != "" {
		fn(jobID)
	}
}

// newUUID returns a random RFC 4122 version 4 UUID. Braket needs one as a
// clientToken and Azure Quantum requires job ids in this form.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func coerceExtraValue(s string) any {
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
