// Copyright 2026 Magnobit, Inc. All rights reserved.

package estimate

import (
	"strings"
	"testing"
)

func TestBindSource_ProducesConcreteCircuit(t *testing.T) {
	src := "PARAM theta : angle\nRX theta 0\nMEASURE"
	out, err := BindSource(src, map[string]float64{"theta": 1.5707963267948966})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "theta") {
		t.Errorf("bound source still contains theta: %q", out)
	}
	if !strings.Contains(out, "RX") {
		t.Errorf("bound source = %q, want concrete RX", out)
	}
}

func TestBindSource_MissingParamErrors(t *testing.T) {
	_, err := BindSource("PARAM theta : angle\nRX theta 0\nMEASURE", nil)
	if err == nil {
		t.Fatal("unbound PARAM without values must error")
	}
}
