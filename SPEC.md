# Quell Language Specification

Version: 0.1.0

---

## Overview

Quell is a domain-specific language for describing quantum circuits. It is:

- **Declarative**: you describe gates and measurements, not control flow
- **Backend-agnostic**: compiled to OpenQASM, Qiskit, Cirq, or Braket
- **Human-readable**: gate per line, no boilerplate

---

## File format

Quell programs are plain text files with the `.quell` extension. Files are UTF-8. Line endings are LF or CRLF.

---

## Syntax

### Comments

```quell
// single-line comment
```

No multi-line comments.

### Instructions

Each non-empty, non-comment line is one instruction:

```
GATE [qubit...] [args...]
```

- `GATE` — gate name, uppercase
- `qubit` — zero-indexed integer (0, 1, 2, …)
- `args` — optional numeric arguments (e.g. rotation angle in radians)

Examples:
```quell
H 0
CNOT 0 1
RX 1.5707963 0
MEASURE 0
```

### Qubits

Qubits are referenced by index starting at 0. The circuit width is inferred from the highest qubit index used.

```quell
H 0       // 1-qubit circuit
H 2       // 3-qubit circuit (qubits 0, 1, 2 all allocated)
```

### Named qubits

Qubits can be given descriptive names using the `qubit` declaration. Names map to 0-indexed integers in declaration order.

```quell
qubit alice, bob

H alice        // same as H 0
CNOT alice bob // same as CNOT 0 1
MEASURE
```

Rules:
- `qubit` declarations must appear before the first gate that uses the name
- Names are case-sensitive
- Mixing named and indexed qubits in the same file is supported; indices pick up after the last declared named qubit
- Multiple names can be declared on one line (comma-separated) or across multiple `qubit` lines

### Whitespace

Leading and trailing whitespace is ignored. Blank lines are ignored. Multiple spaces between tokens are collapsed.

---

## Gate reference

### Single-qubit gates

| Instruction | Syntax | Matrix |
|---|---|---|
| Hadamard | `H q` | 1/√2 [[1,1],[1,-1]] |
| Pauli-X | `X q` | [[0,1],[1,0]] |
| Pauli-Y | `Y q` | [[0,-i],[i,0]] |
| Pauli-Z | `Z q` | [[1,0],[0,-1]] |
| S gate | `S q` | [[1,0],[0,i]] |
| T gate | `T q` | [[1,0],[0,e^(iπ/4)]] |
| S† gate | `SDG q` | [[1,0],[0,-i]] |
| T† gate | `TDG q` | [[1,0],[0,e^(-iπ/4)]] |
| √X | `SX q` | 1/2 [[1+i,1-i],[1-i,1+i]] |

### Rotation gates

| Instruction | Syntax | Description |
|---|---|---|
| RX | `RX θ q` | Rotate around X-axis by θ radians |
| RY | `RY θ q` | Rotate around Y-axis by θ radians |
| RZ | `RZ θ q` | Rotate around Z-axis by θ radians |
| U | `U θ φ λ q` | General single-qubit unitary |
| P | `P θ q` | Phase gate (RZ up to global phase) |

### Two-qubit gates

| Instruction | Syntax | Description |
|---|---|---|
| CNOT | `CNOT control target` | Controlled-NOT |
| CX | `CX control target` | Alias for CNOT |
| CZ | `CZ q0 q1` | Controlled-Z |
| SWAP | `SWAP q0 q1` | Swap two qubits |
| ISWAP | `ISWAP q0 q1` | iSWAP gate |
| CRX | `CRX θ control target` | Controlled-RX |
| CRY | `CRY θ control target` | Controlled-RY |
| CRZ | `CRZ θ control target` | Controlled-RZ |

### Three-qubit gates

| Instruction | Syntax | Description |
|---|---|---|
| CCX | `CCX c0 c1 target` | Toffoli gate |
| CSWAP | `CSWAP control q0 q1` | Fredkin gate |

### Measurement

| Instruction | Syntax | Description |
|---|---|---|
| MEASURE | `MEASURE` | Measure all qubits |
| MEASURE | `MEASURE q` | Measure qubit q |
| MEASURE | `MEASURE q0 q1 …` | Measure listed qubits |
| M | `M` | Alias for `MEASURE` — identical in all compile targets |

---

## Quell config

`quell.config.yml` in the working directory (or path passed with `--config`):

```yaml
backend: local  # local | ibm | aws | google | rigetti

local:
  shots: 1024

ibm:
  token: ${IBM_QUANTUM_TOKEN}      # env var expansion supported
  instance: ibm-q/open/main
  device: ibm_brisbane
  shots: 4096

aws:
  region: us-east-1
  device: arn:aws:braket:::device/quantum-simulator/amazon/sv1
  s3_bucket: my-braket-results
  s3_prefix: quell-results
  shots: 1000

google:
  project: my-gcp-project
  processor: rainbow
  shots: 1000

rigetti:
  api_key: ${RIGETTI_API_KEY}
  device: Aspen-M-3
  shots: 1000
```

Environment variable expansion: `${VAR_NAME}` is replaced with the value of the environment variable at runtime. Quell never stores or transmits credentials.

---

## Compile targets

### OpenQASM 3

```bash
quell compile --target openqasm bell.quell
```

```openqasm
OPENQASM 3;
qubit[2] q;
bit[2] c;
h q[0];
cx q[0], q[1];
c = measure q;
```

### Qiskit (Python)

```bash
quell compile --target qiskit bell.quell
```

```python
from qiskit import QuantumCircuit
qc = QuantumCircuit(2, 2)
qc.h(0)
qc.cx(0, 1)
qc.measure([0, 1], [0, 1])
```

### Cirq (Python)

```bash
quell compile --target cirq bell.quell
```

```python
import cirq
q = cirq.LineQubit.range(2)
circuit = cirq.Circuit([
    cirq.H(q[0]),
    cirq.CNOT(q[0], q[1]),
    cirq.measure(*q, key='result'),
])
```

### Braket (Python)

```bash
quell compile --target braket bell.quell
```

```python
from braket.circuits import Circuit
circuit = Circuit()
circuit.h(0).cnot(0, 1)
circuit.probability()
```

---

## CLI reference

```
quell run <file>              Run circuit on configured backend
quell compile <file>          Compile to configured target
  --target openqasm|qiskit|cirq|braket
  --config path/to/quell.config.yml
  --output out.py
quell ask "<question>"        AI assistant (requires API key)
quell convert <file>          Convert Python/Qiskit to Quell
quell version                 Print version
quell help                    Print help
```

---

## Constants

```quell
// Common angles — use directly or define with //π notation in comments
// π/2  = 1.5707963
// π/4  = 0.7853982
// π    = 3.1415927
// 2π   = 6.2831853
RX 1.5707963 0    // RX(π/2) on qubit 0
```

---

## Roadmap

- [x] Named qubits: `qubit alice, bob` then `H alice` ✅ v0.0.1
- [x] `M` alias for `MEASURE` ✅ v0.0.1
- [ ] Classical registers and conditional gates
- [ ] Subroutines / gate definitions
- [ ] Parameterized circuits
- [ ] Native noise models
- [ ] QASM 3.0 full import/export
