// Copyright 2026 Magnobit, Inc. All rights reserved.

package livevalidate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	ModeOffline         = "OFFLINE"
	ModeContractTested  = "CONTRACT_TESTED"
	ModeSandboxVerified = "SANDBOX_VERIFIED"
	ModeLiveVerified    = "LIVE_VERIFIED"

	AdapterVersion = "quell-backends"
)

// Evidence is a secret-free record of one live validation step.
type Evidence struct {
	Provider         string `json:"provider"`
	Backend          string `json:"backend,omitempty"`
	ValidationKind   string `json:"validationKind"`
	Status           string `json:"status"`
	EvidenceLevel    string `json:"evidenceLevel,omitempty"`
	ProviderJobID    string `json:"providerJobId,omitempty"`
	AccountScopeHash string `json:"accountScopeHash,omitempty"`
	AdapterVersion   string `json:"adapterVersion,omitempty"`
	ResultSummary    string `json:"resultSummary,omitempty"`
	FailureClass     string `json:"failureClass,omitempty"`
}

// MinimalCircuit is a 1-qubit H+measure program for bounded live spend.
const MinimalCircuit = `OPENQASM 3.0;
include "stdgates.inc";
qubit[1] q;
bit[1] c;
h q[0];
c[0] = measure q[0];
`

func evidenceLevelForBackend(provider, backend string) string {
	b := strings.ToLower(strings.TrimSpace(backend))
	if strings.Contains(b, "simulator") || strings.Contains(b, "sim.") {
		return ModeSandboxVerified
	}
	if provider == "ionq" && (b == "simulator" || b == "") {
		return ModeSandboxVerified
	}
	return ModeLiveVerified
}

func scopeHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8])
}

func containsSecret(s string) bool {
	l := strings.ToLower(s)
	needles := []string{
		"bearer ", "authorization:", "client_secret", "secret_access_key",
		"session_token", `"token":`, `"api_key":`, `"apikey":`,
	}
	for _, n := range needles {
		if strings.Contains(l, n) {
			return true
		}
	}
	return false
}
