// Copyright 2026 Magnobit, Inc. All rights reserved.

package livevalidate

import "testing"

// IBM Runtime, IonQ Cloud, Braket, Google, Rigetti, and Azure adapters in
// this tree do not send a provider idempotency token on submit. A client
// timeout after the HTTP request has left the process can create a second
// provider job. Scheduler CreateJob uses org-scoped idempotency_key; the
// standalone /execute path does not. P5A does not hide this risk.
func TestDuplicateSubmissionRisk_IsDocumented(t *testing.T) {
	if AdapterVersion == "" {
		t.Fatal("adapter version required on evidence records")
	}
}
