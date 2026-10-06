// Copyright 2026 Magnobit, Inc. All rights reserved.

package qir

import (
	"strings"
	"testing"
)

func TestToQuellSubset(t *testing.T) {
	text := "" +
		"call void @__quantum__qis__h__body(ptr null)\n" +
		"call void @__quantum__qis__cnot__body(ptr null, ptr inttoptr (i64 1 to ptr))\n" +
		"call void @__quantum__qis__mz__body(ptr null)\n"
	got, err := ToQuell(text)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "H 0") || !strings.Contains(got, "CNOT 0 1") || !strings.Contains(got, "MEASURE 0") {
		t.Fatalf("got %q", got)
	}
}

func TestToQuellRejectsUnknown(t *testing.T) {
	_, err := ToQuell("call void @__quantum__qis__reset__body(ptr null)\n")
	if err == nil {
		t.Fatal("reset is outside the documented subset")
	}
}
