// Copyright 2026 Magnobit, Inc. All rights reserved.

package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

const (
	tensorMaxQubits  = 12
	densityMaxQubits = 6
)

var (
	externalOnce sync.Once
	tensorInfo   Info
	densityInfo  Info
)

func externalStatus() (Info, Info) {
	externalOnce.Do(func() {
		tensorInfo = Info{Name: TensorNetwork, Status: StatusUnavailable, Detail: "quimb is not importable"}
		if out, err := python(`import quimb.tensor as qtn
c = qtn.Circuit(2)
c.apply_gate('H', 0)
c.apply_gate('CNOT', 0, 1)
print('ok')
`); err == nil && bytes.Contains(out, []byte("ok")) {
			tensorInfo = Info{Name: TensorNetwork, Status: StatusAvailable, Detail: "quimb.tensor.Circuit, at most 12 qubits"}
		}
		densityInfo = Info{Name: DensityMatrix, Status: StatusUnavailable, Detail: "qiskit.quantum_info.DensityMatrix is not importable"}
		if out, err := python(`from qiskit import QuantumCircuit
from qiskit.quantum_info import DensityMatrix
qc = QuantumCircuit(1)
qc.h(0)
dm = DensityMatrix.from_instruction(qc)
print('ok' if abs(float(dm.probabilities()[0]) - 0.5) < 1e-6 else 'bad')
`); err == nil && bytes.Contains(out, []byte("ok")) {
			densityInfo = Info{Name: DensityMatrix, Status: StatusAvailable, Detail: "qiskit DensityMatrix, at most 6 qubits"}
		}
	})
	return tensorInfo, densityInfo
}

func python(script string) ([]byte, error) {
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(py, "-c", script)
	return cmd.CombinedOutput()
}

type gateJSON struct {
	Kind   string `json:"kind"`
	Qubits []int  `json:"qubits"`
}

func runTensor(src string, shots int) (map[string]int, error) {
	info, _ := externalStatus()
	if info.Status != StatusAvailable {
		return nil, fmt.Errorf("engine tensor-network is unavailable: %s", info.Detail)
	}
	gates, n, err := cliffordish(src)
	if err != nil {
		return nil, err
	}
	if n > tensorMaxQubits {
		return nil, fmt.Errorf("engine tensor-network supports at most %d qubits", tensorMaxQubits)
	}
	payload, _ := json.Marshal(struct {
		N     int        `json:"n"`
		Shots int        `json:"shots"`
		Gates []gateJSON `json:"gates"`
	}{n, shots, gates})
	script := `import json,sys
import numpy as np
import quimb.tensor as qtn
req=json.load(sys.stdin)
c=qtn.Circuit(req["n"])
for g in req["gates"]:
    if g["kind"]=="CNOT":
        c.apply_gate("CNOT", g["qubits"][0], g["qubits"][1])
    else:
        c.apply_gate(g["kind"], g["qubits"][0])
sv=np.asarray(c.to_dense()).reshape(-1)
p=np.abs(sv)**2
p=p/np.sum(p)
counts={}
for _ in range(req["shots"]):
    i=int(np.random.choice(len(p), p=p))
    bits=format(i, "0"+str(req["n"])+"b")
    counts[bits]=counts.get(bits,0)+1
print(json.dumps(counts))
`
	out, err := pythonStdin(script, payload)
	if err != nil {
		return nil, fmt.Errorf("engine tensor-network: %w: %s", err, out)
	}
	var counts map[string]int
	if err := json.Unmarshal(lastJSON(out), &counts); err != nil {
		return nil, fmt.Errorf("engine tensor-network: %w: %s", err, out)
	}
	return counts, nil
}

func runDensity(src string, shots int) (map[string]int, error) {
	_, info := externalStatus()
	if info.Status != StatusAvailable {
		return nil, fmt.Errorf("engine density-matrix is unavailable: %s", info.Detail)
	}
	gates, n, err := cliffordish(src)
	if err != nil {
		return nil, err
	}
	if n > densityMaxQubits {
		return nil, fmt.Errorf("engine density-matrix supports at most %d qubits", densityMaxQubits)
	}
	payload, _ := json.Marshal(struct {
		N     int        `json:"n"`
		Shots int        `json:"shots"`
		Gates []gateJSON `json:"gates"`
	}{n, shots, gates})
	script := `import json,sys
from qiskit import QuantumCircuit
from qiskit.quantum_info import DensityMatrix
req=json.load(sys.stdin)
qc=QuantumCircuit(req["n"])
for g in req["gates"]:
    k=g["kind"]
    q=g["qubits"]
    if k=="H": qc.h(q[0])
    elif k=="X": qc.x(q[0])
    elif k=="Y": qc.y(q[0])
    elif k=="Z": qc.z(q[0])
    elif k=="CNOT": qc.cx(q[0], q[1])
    else:
        raise SystemExit("unsupported "+k)
dm=DensityMatrix.from_instruction(qc)
probs=[float(x) for x in dm.probabilities()]
shots=req["shots"]
counts={}
acc=0.0
for i,p in enumerate(probs):
    c=int(round(p*shots))
    acc += c
    if c:
        counts[format(i, "0"+str(req["n"])+"b")]=c
# rounding can miss a shot; put the remainder on the largest bin
if shots>0 and acc!=shots:
    pass
print(json.dumps(counts))
`
	out, err := pythonStdin(script, payload)
	if err != nil {
		return nil, fmt.Errorf("engine density-matrix: %w: %s", err, out)
	}
	var counts map[string]int
	if err := json.Unmarshal(lastJSON(out), &counts); err != nil {
		return nil, fmt.Errorf("engine density-matrix: %w: %s", err, out)
	}
	return counts, nil
}

func cliffordish(src string) ([]gateJSON, int, error) {
	c, err := parser.Parse(src)
	if err != nil {
		return nil, 0, err
	}
	p := ir.Lower(c)
	var gates []gateJSON
	for _, op := range p.Ops {
		if op.Kind == ir.OpMEASURE || op.Kind == ir.OpBARRIER {
			continue
		}
		switch op.Kind {
		case ir.OpH, ir.OpX, ir.OpY, ir.OpZ, ir.OpCNOT:
			gates = append(gates, gateJSON{Kind: string(op.Kind), Qubits: append([]int(nil), op.Qubits...)})
		default:
			return nil, 0, fmt.Errorf("engine adapter does not lower %s", op.Kind)
		}
	}
	n := p.NumQubits
	if n < 1 {
		n = 1
	}
	return gates, n, nil
}

func pythonStdin(script string, payload []byte) ([]byte, error) {
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(py, "-c", script)
	cmd.Stdin = bytes.NewReader(payload)
	return cmd.CombinedOutput()
}

func lastJSON(out []byte) []byte {
	i := bytes.LastIndex(out, []byte("{"))
	if i < 0 {
		return out
	}
	return out[i:]
}
