// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package quell embeds the project's own docs so the CLI's `quell ask` can
// answer common questions locally, with no ANTHROPIC_API_KEY and no network
// call — the same doc-search fallback the platform API's /api/v1/ai/ask
// endpoint uses when no key is configured.
package quell

import _ "embed"

//go:embed README.md
var readmeMD string

//go:embed SPEC.md
var specMD string

// Docs returns the embedded docs keyed by filename, for askdocs.Answer.
func Docs() map[string]string {
	return map[string]string{
		"README.md": readmeMD,
		"SPEC.md":   specMD,
	}
}
