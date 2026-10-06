// Copyright 2026 Magnobit, Inc. All rights reserved.

package cudaq

import "testing"

func TestProbeDoesNotInventGPU(t *testing.T) {
	inst := Probe()
	if inst.GPU && inst.Selected == "qpp-cpu" {
		t.Fatal("qpp-cpu was labeled GPU")
	}
	if !inst.Installed && (inst.CPU || inst.GPU || inst.Engine != "") {
		t.Fatalf("missing CUDA-Q looked runnable: %+v", inst)
	}
	if inst.CPU && inst.Engine != "cudaq" {
		t.Fatalf("cpu engine %+v", inst)
	}
	t.Logf("cudaq-installed=%v cpu-target-available=%v gpu-target-available=%v selected-target=%s actual-engine=%s",
		inst.Installed, inst.CPU, inst.GPU, inst.Selected, inst.Engine)
}
