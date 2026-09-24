// Copyright 2026 Magnobit, Inc. All rights reserved.

package execute

import "testing"

func TestCancel_EmptyProviderJobIDIsNoOp(t *testing.T) {
	if err := Cancel(IBM, &IBMCredentials{Token: "tok"}, ""); err != nil {
		t.Fatalf("empty id should be a no-op, got %v", err)
	}
}

func TestCancel_LocalBackendsAreNoOp(t *testing.T) {
	for _, b := range []string{NVIDIA, Intel, DWave, Local} {
		if err := Cancel(b, nil, "job-1"); err != nil {
			t.Fatalf("Cancel(%s) = %v, want nil", b, err)
		}
	}
}

func TestCancel_IBMRequiresTypedCredentials(t *testing.T) {
	if err := Cancel(IBM, "not-creds", "job-1"); err == nil {
		t.Fatal("expected an error when IBM credentials are the wrong type")
	}
}
