//go:build live

// Copyright 2026 Magnobit, Inc. All rights reserved.

package livevalidate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/magnobit/quell/internal/backends"
	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/telemetry"
)

func TestLiveProviders(t *testing.T) {
	if os.Getenv("QUELL_LIVE") != "1" {
		t.Skip("live tests skipped (set QUELL_LIVE=1 and -tags=live)")
	}
	budget := budgetFromEnv()
	started := time.Now()
	jobsUsed := 0
	var rows []Evidence

	if budget.providerAllowed("ibm") {
		rows = append(rows, runIBMLive(t, budget, &jobsUsed, started)...)
	} else {
		rows = append(rows, Evidence{Provider: "ibm", Status: "NOT_RUN", ResultSummary: "provider not in allow-list"})
	}
	if budget.providerAllowed("ionq") {
		rows = append(rows, runIonQLive(t, budget, &jobsUsed, started)...)
	} else {
		rows = append(rows, Evidence{Provider: "ionq", Status: "NOT_RUN", ResultSummary: "provider not in allow-list"})
	}

	for _, p := range []string{"aws", "google", "rigetti", "azure", "dwave"} {
		reason := "credentials/provider access unavailable"
		if !budget.providerAllowed(p) {
			reason = "not in P5A primary allow-list"
		}
		rows = append(rows, Evidence{Provider: p, Status: "NOT_RUN", ResultSummary: reason})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	summary := map[string]any{"evidence": rows, "jobsUsed": jobsUsed}
	if err := enc.Encode(summary); err != nil {
		t.Fatalf("encode summary: %v", err)
	}
	raw, _ := json.Marshal(summary)
	if containsSecret(string(raw)) {
		t.Fatal("live summary contained secret-shaped text")
	}
	for _, row := range rows {
		if row.Status == "FAIL" {
			t.Errorf("%s %s failed: %s %s", row.Provider, row.ValidationKind, row.FailureClass, row.ResultSummary)
		}
	}
}

func runIBMLive(t *testing.T, budget Budget, jobsUsed *int, started time.Time) []Evidence {
	t.Helper()
	var out []Evidence
	token := envOr("IBM_QUANTUM_TOKEN", "IBM_TELEMETRY_TOKEN")
	device := envOr("QUELL_LIVE_IBM_BACKEND", "IBM_TELEMETRY_DEVICE")
	instance := envOr("QUELL_LIVE_IBM_INSTANCE", "IBM_QUANTUM_INSTANCE")

	out = append(out, ibmMissingToken())
	out = append(out, ibmInvalidToken(device, instance))

	missing := ""
	switch {
	case token == "":
		missing = "credentials unavailable"
	case instance == "":
		missing = "instance CRN unavailable (set QUELL_LIVE_IBM_INSTANCE)"
	case device == "":
		missing = "backend unavailable (set QUELL_LIVE_IBM_BACKEND)"
	}
	if missing != "" {
		for _, kind := range []string{"auth", "telemetry", "submit"} {
			out = append(out, Evidence{Provider: "ibm", ValidationKind: kind, Status: "NOT_RUN", ResultSummary: missing})
		}
		return out
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := &telemetry.IBMTelemetryClient{Token: token, Instance: instance, Device: device}
	tel, err := client.FetchTelemetry(ctx)
	ev := Evidence{
		Provider:         "ibm",
		Backend:          device,
		ValidationKind:   "telemetry",
		AdapterVersion:   AdapterVersion,
		AccountScopeHash: scopeHash(instance),
	}
	if err != nil {
		ev.Status = "FAIL"
		ev.FailureClass = tel.FailKind
		if ev.FailureClass == "" {
			ev.FailureClass = backends.ClassOf(err)
		}
		ev.ResultSummary = "telemetry fetch failed"
		if tel.FailKind == "auth" || tel.FailKind == "permission" {
			ev.ValidationKind = "auth"
		}
	} else {
		ev.Status = "PASS"
		ev.EvidenceLevel = ModeSandboxVerified
		ev.ResultSummary = fmt.Sprintf("qubits=%v gates=%d coupling=%d readout=%v assignment=%d t1t2=%d",
			ptrInt(tel.Qubits), len(tel.NativeGates), len(tel.CouplingMap), tel.ReadoutError != nil,
			assignmentCount(tel), t1t2Count(tel))
		out = append(out, Evidence{
			Provider: "ibm", Backend: device, ValidationKind: "auth", Status: "PASS",
			EvidenceLevel: ModeSandboxVerified, AdapterVersion: AdapterVersion,
			AccountScopeHash: scopeHash(instance), ResultSummary: "valid token accepted by IBM telemetry API",
		})
	}
	out = append(out, ev)

	if err := budget.admitSubmit(*jobsUsed, 0.05, started); err != nil {
		out = append(out, Evidence{
			Provider: "ibm", Backend: device, ValidationKind: "submit",
			Status: "NOT_RUN", ResultSummary: err.Error(),
		})
		return out
	}

	cfg := &config.IBMConfig{Token: token, Device: device, Instance: instance, Shots: budget.MaxShots}
	result, subErr := backends.RunIBM(cfg, MinimalCircuit, 1)
	*jobsUsed++
	sub := Evidence{
		Provider: "ibm", Backend: device, ValidationKind: "submit",
		AdapterVersion: AdapterVersion, AccountScopeHash: scopeHash(instance),
		EvidenceLevel: evidenceLevelForBackend("ibm", device),
	}
	if subErr != nil {
		sub.Status = "FAIL"
		sub.FailureClass = backends.ClassOf(subErr)
		sub.ResultSummary = "submit/poll/result failed"
		out = append(out, sub)
		return out
	}
	sub.Status = "PASS"
	sub.ProviderJobID = result.JobID
	sub.ResultSummary = fmt.Sprintf("shots=%d outcomes=%d", result.Shots, len(result.Counts))
	out = append(out, sub)
	return out
}

func runIonQLive(t *testing.T, budget Budget, jobsUsed *int, started time.Time) []Evidence {
	t.Helper()
	var out []Evidence
	key := envOr("IONQ_API_KEY", "IONQ_TELEMETRY_KEY")
	device := envOr("QUELL_LIVE_IONQ_BACKEND", "IONQ_TELEMETRY_DEVICE")
	if device == "" {
		device = "simulator"
	}

	out = append(out, ionqMissingKey())
	out = append(out, ionqInvalidKey(device))

	if key == "" {
		out = append(out, Evidence{Provider: "ionq", ValidationKind: "auth", Status: "NOT_RUN", ResultSummary: "credentials unavailable"})
		out = append(out, Evidence{Provider: "ionq", ValidationKind: "telemetry", Status: "NOT_RUN", ResultSummary: "credentials unavailable"})
		out = append(out, Evidence{Provider: "ionq", ValidationKind: "submit", Status: "NOT_RUN", ResultSummary: "credentials unavailable"})
		return out
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := &telemetry.IonQTelemetryClient{APIKey: key, Device: device}
	tel, err := client.FetchTelemetry(ctx)
	ev := Evidence{
		Provider: "ionq", Backend: device, ValidationKind: "telemetry",
		AdapterVersion: AdapterVersion, AccountScopeHash: scopeHash("ionq", device),
	}
	if err != nil {
		ev.Status = "FAIL"
		ev.FailureClass = tel.FailKind
		if ev.FailureClass == "" {
			ev.FailureClass = backends.ClassOf(err)
		}
		ev.ResultSummary = "telemetry fetch failed"
	} else {
		derived := "absent"
		if tel.TwoQubitErrorDerived {
			derived = tel.TwoQubitErrorDerivation
		}
		q := "unknown"
		if tel.QueueWaitSeconds != nil {
			q = fmt.Sprintf("%d", *tel.QueueWaitSeconds)
		}
		ev.Status = "PASS"
		ev.EvidenceLevel = ModeSandboxVerified
		ev.ResultSummary = fmt.Sprintf("qubits=%v gates=%d queue=%s fidelity=%v derived=%s",
			ptrInt(tel.Qubits), len(tel.NativeGates), q, tel.AverageFidelity != nil, derived)
		out = append(out, Evidence{
			Provider: "ionq", Backend: device, ValidationKind: "auth", Status: "PASS",
			EvidenceLevel: ModeSandboxVerified, AdapterVersion: AdapterVersion,
			ResultSummary: "valid key accepted by IonQ backend API",
		})
	}
	out = append(out, ev)

	if err := budget.admitSubmit(*jobsUsed, 0.05, started); err != nil {
		out = append(out, Evidence{
			Provider: "ionq", Backend: device, ValidationKind: "submit",
			Status: "NOT_RUN", ResultSummary: err.Error(),
		})
		return out
	}

	cfg := &config.IonQConfig{APIKey: key, Device: device, Shots: budget.MaxShots}
	result, subErr := backends.RunIonQ(cfg, MinimalCircuit, 1)
	*jobsUsed++
	sub := Evidence{
		Provider: "ionq", Backend: device, ValidationKind: "submit",
		AdapterVersion: AdapterVersion, EvidenceLevel: evidenceLevelForBackend("ionq", device),
	}
	if subErr != nil {
		sub.Status = "FAIL"
		sub.FailureClass = backends.ClassOf(subErr)
		sub.ResultSummary = "submit/poll/result failed"
		out = append(out, sub)
		return out
	}
	sub.Status = "PASS"
	sub.ProviderJobID = result.JobID
	sub.ResultSummary = fmt.Sprintf("shots=%d outcomes=%d", result.Shots, len(result.Counts))
	out = append(out, sub)
	return out
}

func ibmMissingToken() Evidence {
	_, err := backends.RunIBM(&config.IBMConfig{Device: "ibm_test"}, MinimalCircuit, 1)
	status := "FAIL"
	summary := "missing token did not error"
	if err != nil {
		status = "PASS"
		summary = "missing token rejected before HTTP"
	}
	return Evidence{Provider: "ibm", ValidationKind: "auth_missing", Status: status, ResultSummary: summary}
}

func ibmInvalidToken(device, instance string) Evidence {
	ev := Evidence{Provider: "ibm", Backend: device, ValidationKind: "auth_invalid"}
	if device == "" || instance == "" {
		ev.Status, ev.ResultSummary = "NOT_RUN", "needs a backend and instance CRN to reach IAM"
		return ev
	}
	_, err := backends.RunIBM(&config.IBMConfig{Token: "p5a-invalid-token", Device: device, Instance: instance}, MinimalCircuit, 1)
	if err == nil {
		ev.Status = "FAIL"
		ev.ResultSummary = "invalid token was accepted"
		return ev
	}
	if containsSecret(err.Error()) {
		ev.Status = "FAIL"
		ev.FailureClass = "secret_leak"
		ev.ResultSummary = "error text looked secret-shaped"
		return ev
	}
	ev.Status = "PASS"
	ev.FailureClass = backends.ClassOf(err)
	ev.ResultSummary = "invalid token classified without secret leak"
	return ev
}

func ionqMissingKey() Evidence {
	_, err := backends.RunIonQ(&config.IonQConfig{Device: "simulator"}, MinimalCircuit, 1)
	status := "FAIL"
	summary := "missing key did not error"
	if err != nil {
		status = "PASS"
		summary = "missing key rejected before HTTP"
	}
	return Evidence{Provider: "ionq", ValidationKind: "auth_missing", Status: status, ResultSummary: summary}
}

func ionqInvalidKey(device string) Evidence {
	_, err := backends.RunIonQ(&config.IonQConfig{APIKey: "p5a-invalid-key", Device: device}, MinimalCircuit, 1)
	ev := Evidence{Provider: "ionq", Backend: device, ValidationKind: "auth_invalid"}
	if err == nil {
		ev.Status = "FAIL"
		ev.ResultSummary = "invalid key was accepted"
		return ev
	}
	if containsSecret(err.Error()) {
		ev.Status = "FAIL"
		ev.FailureClass = "secret_leak"
		ev.ResultSummary = "error text looked secret-shaped"
		return ev
	}
	ev.Status = "PASS"
	ev.FailureClass = backends.ClassOf(err)
	ev.ResultSummary = "invalid key classified without secret leak"
	return ev
}

func ptrInt(v *int) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *v)
}

func assignmentCount(t telemetry.BackendTelemetry) int {
	n := 0
	for _, q := range t.QubitsCal {
		if q.HasAssignmentMatrix {
			n++
		}
	}
	return n
}

func t1t2Count(t telemetry.BackendTelemetry) int {
	n := 0
	for _, q := range t.QubitsCal {
		if q.T1Seconds != nil || q.T2Seconds != nil {
			n++
		}
	}
	return n
}
