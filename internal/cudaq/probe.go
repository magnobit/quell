// Copyright 2026 Magnobit, Inc. All rights reserved.

package cudaq

import (
	"bytes"
	"os/exec"
	"strings"
)

// Installation is what this process can actually run.
// qpp-cpu is a CPU target. It is never reported as a GPU.
type Installation struct {
	Installed bool
	CPU       bool
	GPU       bool
	Selected  string
	Engine    string
}

// Probe asks the installed CUDA-Q module which targets it can run.
// A missing module leaves every flag false. A CPU sample that succeeds
// sets Engine to cudaq and Selected to qpp-cpu.
func Probe() Installation {
	var inst Installation
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return inst
		}
	}
	cmd := exec.Command(py, "-c", "import cudaq\nprint('installed')\nnames=[]\ntry:\n    names=[getattr(t,'name',str(t)) for t in cudaq.get_targets()]\nexcept Exception:\n    pass\nprint('TARGETS '+','.join(names))\n")
	out, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(out, []byte("installed")) {
		return inst
	}
	inst.Installed = true
	names := ""
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "TARGETS ") {
			names = strings.TrimPrefix(line, "TARGETS ")
		}
	}
	if strings.Contains(names, "qpp-cpu") || names == "" {
		if _, err := Run("H 0\nMEASURE\n", 8); err == nil {
			inst.CPU = true
			inst.Selected = "qpp-cpu"
			inst.Engine = "cudaq"
		}
	}
	if gpuTarget(names) && nvidiaPresent() {
		if runGPU(py) == nil {
			inst.GPU = true
		}
	}
	return inst
}

func gpuTarget(names string) bool {
	low := strings.ToLower(names)
	return strings.Contains(low, "gpu") || strings.Contains(low, "nvidia")
}

func nvidiaPresent() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}

func runGPU(py string) error {
	script := "import cudaq\nnames=[getattr(t,'name',str(t)) for t in cudaq.get_targets()]\npick=''\nfor n in names:\n    if 'gpu' in n.lower() or n.lower().startswith('nvidia'):\n        pick=n\n        break\nif not pick:\n    raise SystemExit('no gpu target')\ncudaq.set_target(pick)\n@cudaq.kernel\ndef quell():\n    q=cudaq.qvector(1)\n    h(q[0])\n    mz(q[0])\nprint(cudaq.sample(quell, shots_count=4))\n"
	cmd := exec.Command(py, "-c", script)
	_, err := cmd.CombinedOutput()
	return err
}
