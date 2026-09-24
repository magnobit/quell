// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package livevalidate is the opt-in P5A live-provider harness.
//
// Declared adapter readiness in quell/provider stays CONTRACT_TESTED
// (or whatever the registry records). Successful live runs produce
// ValidationEvidence at SANDBOX_VERIFIED or LIVE_VERIFIED — they do
// not overwrite architectural readiness.
//
// Standard `go test ./...` never talks to the internet. Live tests
// compile only with `-tags=live` and still skip unless QUELL_LIVE=1.
package livevalidate
