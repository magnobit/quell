// Copyright 2026 Magnobit, Inc. All rights reserved.

package anneal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SampleLeap submits a QUBO to D-Wave Leap via the Ocean SDK (Python) when
// available. Requires DWAVE_API_TOKEN (or cfg token) and `pip install dwave-ocean-sdk`.
// If Ocean is not installed, returns an error — callers should fall back to SampleLocal.
func SampleLeap(token, solver string, p *Problem, numReads int) (*Result, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if token == "" {
		token = os.Getenv("DWAVE_API_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("anneal: Leap token required (dwave.api_token or DWAVE_API_TOKEN)")
	}
	if numReads < 1 {
		numReads = 100
	}
	if solver == "" {
		solver = "hybrid_binary_quadratic_model_version2"
	}

	lin := map[string]float64{}
	for i, v := range p.Linear {
		lin[fmt.Sprintf("%d", i)] = v
	}
	quad := map[string]float64{}
	for k, v := range p.Quadratic {
		quad[fmt.Sprintf("%d,%d", k[0], k[1])] = v
	}
	payload, _ := json.Marshal(map[string]any{
		"token":     token,
		"solver":    solver,
		"num_reads": numReads,
		"n":         p.NumVars,
		"linear":    lin,
		"quadratic": quad,
	})

	script := leapPythonScript
	cmd := exec.Command("python", "-c", script)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(), "DWAVE_API_TOKEN="+token)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		// try python3
		cmd3 := exec.Command("python3", "-c", script)
		cmd3.Stdin = bytes.NewReader(payload)
		cmd3.Env = cmd.Env
		stdout.Reset()
		stderr.Reset()
		cmd3.Stdout = &stdout
		cmd3.Stderr = &stderr
		if err3 := cmd3.Run(); err3 != nil {
			msg3 := strings.TrimSpace(stderr.String())
			if msg3 == "" {
				msg3 = msg
			}
			return nil, fmt.Errorf("anneal: Leap submit failed (install dwave-ocean-sdk): %s", msg3)
		}
	}

	var parsed struct {
		Error   string `json:"error"`
		Info    string `json:"info"`
		Samples []struct {
			Bits     []int   `json:"bits"`
			Energy   float64 `json:"energy"`
			NumOccur int     `json:"num_occur"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("anneal: decode Leap output: %w (%s)", err, stdout.String())
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("anneal: Leap: %s", parsed.Error)
	}
	out := &Result{Info: parsed.Info, Samples: make([]Sample, 0, len(parsed.Samples))}
	for _, s := range parsed.Samples {
		out.Samples = append(out.Samples, Sample{Bits: s.Bits, Energy: s.Energy, NumOccur: s.NumOccur})
	}
	return out, nil
}

// Embedded Ocean bridge — keeps the Go module free of a Python package dep
// while still talking to real Leap when Ocean is installed on the host.
const leapPythonScript = `
import json, sys
try:
    import dimod
    from dwave.system import LeapHybridBQMSampler, DWaveSampler, EmbeddingComposite
except Exception as e:
    print(json.dumps({"error": "dwave-ocean-sdk required: pip install dwave-ocean-sdk (%s)" % e}))
    sys.exit(0)

req = json.load(sys.stdin)
token = req.get("token") or ""
solver = req.get("solver") or "hybrid_binary_quadratic_model_version2"
n = int(req.get("n") or 0)
lin = {int(k): float(v) for k, v in (req.get("linear") or {}).items()}
quad = {}
for k, v in (req.get("quadratic") or {}).items():
    a, b = k.split(",")
    quad[(int(a), int(b))] = float(v)

bqm = dimod.BinaryQuadraticModel(lin, quad, 0.0, dimod.BINARY)
samples = []
info = ""
try:
    if "hybrid" in solver.lower():
        sampler = LeapHybridBQMSampler(token=token)
        ss = sampler.sample(bqm, label="quell-anneal")
        info = "Leap hybrid BQM: " + solver
    else:
        qpu = DWaveSampler(token=token, solver={"name__eq": solver} if solver else None)
        sampler = EmbeddingComposite(qpu)
        ss = sampler.sample(bqm, num_reads=int(req.get("num_reads") or 100), label="quell-anneal")
        info = "Leap QPU: " + getattr(qpu, "solver", solver)
    for rec in ss.data(["sample", "energy", "num_occurrences"]):
        bits = [int(rec.sample.get(i, 0)) for i in range(n)]
        samples.append({"bits": bits, "energy": float(rec.energy), "num_occur": int(rec.num_occurrences)})
except Exception as e:
    print(json.dumps({"error": str(e)}))
    sys.exit(0)
print(json.dumps({"info": info, "samples": samples}))
`
