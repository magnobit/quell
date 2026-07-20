// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/simulate"
)

// RunNVIDIA executes a Quell circuit (passed as OpenQASM 3 or raw Quell —
// we prefer Quell source via Extra["quell_source"] when present) on NVIDIA
// CUDA-Q when installed. Otherwise falls back to Quell's local statevector
// simulator so Cloud/CLI plumbing works without a GPU (Backend notes the fallback).
func RunNVIDIA(cfg *config.NVIDIAConfig, qasm3 string) (*RunResult, error) {
	if cfg == nil {
		cfg = &config.NVIDIAConfig{}
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1024
	}
	device := cfg.Device
	if device == "" {
		device = "nvidia"
	}

	quellSrc := ""
	if cfg.Extra != nil {
		quellSrc = cfg.Extra["quell_source"]
	}

	if os.Getenv("QUELL_NVIDIA_REQUIRE_CUDAQ") != "1" {
		if counts, err := tryCUDAQ(qasm3, shots, device); err == nil {
			return &RunResult{
				JobID:   "nvidia-cudaq",
				Backend: "NVIDIA / CUDA-Q / " + device,
				Shots:   shots,
				Counts:  counts,
			}, nil
		}
	} else {
		counts, err := tryCUDAQ(qasm3, shots, device)
		if err != nil {
			return nil, err
		}
		return &RunResult{JobID: "nvidia-cudaq", Backend: "NVIDIA / CUDA-Q / " + device, Shots: shots, Counts: counts}, nil
	}

	// Local fallback — same simulator as `quell simulate`
	src := quellSrc
	if src == "" {
		return nil, fmt.Errorf("nvidia: CUDA-Q not available and no quell_source for local fallback — install cuda-quantum or pass Extra quell_source")
	}
	res, err := simulate.Run(src, shots)
	if err != nil {
		return nil, fmt.Errorf("nvidia: local fallback: %w", err)
	}
	return &RunResult{
		JobID:   "nvidia-local-fallback",
		Backend: "NVIDIA / local-statevector-fallback",
		Shots:   shots,
		Counts:  res.Counts,
	}, nil
}

func tryCUDAQ(qasm3 string, shots int, device string) (map[string]int, error) {
	payload, _ := json.Marshal(map[string]any{
		"qasm":   qasm3,
		"shots":  shots,
		"device": device,
	})
	cmd := exec.Command("python", "-c", cudaqPythonScript)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cmd3 := exec.Command("python3", "-c", cudaqPythonScript)
		cmd3.Stdin = bytes.NewReader(payload)
		stdout.Reset()
		stderr.Reset()
		cmd3.Stdout = &stdout
		cmd3.Stderr = &stderr
		if err3 := cmd3.Run(); err3 != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err3.Error()
			}
			return nil, fmt.Errorf("cudaq: %s", msg)
		}
	}
	var parsed struct {
		Error  string         `json:"error"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("cudaq decode: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("cudaq: %s", parsed.Error)
	}
	return parsed.Counts, nil
}

const cudaqPythonScript = `
import json, sys, re
req = json.load(sys.stdin)
try:
    import cudaq
except Exception as e:
    print(json.dumps({"error": "cuda-quantum not installed: %s" % e}))
    sys.exit(0)

qasm = req.get("qasm") or ""
shots = int(req.get("shots") or 1024)
# Minimal OpenQASM 3 → CUDA-Q kernel via gate scrape (H, X, CX/CNOT, MEASURE)
lines = []
for raw in qasm.splitlines():
    line = raw.split("//")[0].strip()
    if not line or line.startswith("OPENQASM") or line.startswith("include") or line.startswith("qubit") or line.startswith("bit"):
        continue
    lines.append(line)

n = 0
ops = []
for line in lines:
    m = re.match(r"(h|x|cx|cnot)\s+.*?\[(\d+)\](?:\s*,\s*.*?\[(\d+)\])?", line, re.I)
    if not m:
        # Quell-emitted style sometimes: h q[0];
        m2 = re.match(r"(h|x)\s+q\[(\d+)\]", line, re.I)
        if m2:
            g, a = m2.group(1).lower(), int(m2.group(2))
            n = max(n, a+1)
            ops.append((g, a, None))
            continue
        m3 = re.match(r"(cx|cnot)\s+q\[(\d+)\]\s*,\s*q\[(\d+)\]", line, re.I)
        if m3:
            a, b = int(m3.group(2)), int(m3.group(3))
            n = max(n, a+1, b+1)
            ops.append(("cx", a, b))
        continue
    g = m.group(1).lower()
    a = int(m.group(2))
    b = int(m.group(3)) if m.group(3) else None
    n = max(n, a+1)
    if b is not None:
        n = max(n, b+1)
    ops.append((g if g != "cnot" else "cx", a, b))

if n < 1:
    n = 1

@cudaq.kernel
def k():
    q = cudaq.qvector(n)
    # ops applied below via generated calls — CUDA-Q kernels need static structure;
    # use dynamic via cudaq.qvector and python-side loop with cudaq.expose is limited,
    # so we build a string kernel instead.
    pass

# Build kernel as string for flexibility
body = ["    q = cudaq.qvector(%d)" % n]
for g, a, b in ops:
    if g == "h":
        body.append("    h(q[%d])" % a)
    elif g == "x":
        body.append("    x(q[%d])" % a)
    elif g == "cx":
        body.append("    cx(q[%d], q[%d])" % (a, b))
src = "import cudaq\n@cudaq.kernel\ndef circuit():\n" + "\n".join(body) + "\n    mz(q)\n"
try:
    ns = {}
    exec(src, ns)
    circuit = ns["circuit"]
    counts = cudaq.sample(circuit, shots_count=shots)
    out = {str(k): int(v) for k, v in counts.items()}
    print(json.dumps({"counts": out}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`
