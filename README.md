# Quell

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://go.dev)
[![Made by Magnobit](https://img.shields.io/badge/Made%20by-Magnobit-6C3BD1.svg)](https://magnobit.com)

**The simplest quantum programming language. Write once, run on any platform.**

One `.quell` file. One config line to switch backends. No rewrites.

```quell
H 0
CNOT 0 1
MEASURE
```

That's a Bell pair. Three lines. Runs on IBM Quantum, AWS Braket, or Google Quantum Engine — just change one line in `quell.config.yml`.

---

## vs every other quantum language

| | Quell | Qiskit | Q# | Cirq | OpenQASM |
|---|---|---|---|---|---|
| **Syntax** | Human-readable | Python + boilerplate | .NET + types | Python + boilerplate | Assembly |
| **Multi-backend** | ✅ Native | ✗ IBM only | ✗ Azure only | ✗ Google only | ✗ Compile-only |
| **Install** | `go install` or Docker | pip + 50 MB | dotnet SDK | pip + 50 MB | None |
| **Run on real QPU** | ✅ One config line | Manual API setup | Manual setup | Manual setup | External tooling |
| **Browser native** | ✅ QubitLabs | ✗ | ✗ | ✗ | ✗ |
| **AI assistant** | ✅ Built-in | ✗ | ✗ | ✗ | ✗ |
| **Named qubits** | ✅ `qubit alice, bob` | ✗ | ✗ | ✗ | ✗ |
| **Open source** | ✅ Apache 2.0 | ✅ | ✅ | ✅ | ✅ |

---

## Quick start

```bash
# Install (requires Go 1.25+)
go install github.com/magnobit/quell/cmd/quell@latest

# Or use Docker (no Go needed)
docker pull ghcr.io/magnobit/quell:latest

# Run locally (shows OpenQASM preview)
quell run examples/bell.quell

# Compile to any backend format
quell compile --target qiskit   examples/bell.quell
quell compile --target openqasm examples/bell.quell
quell compile --target cirq     examples/bell.quell
quell compile --target braket   examples/bell.quell

# Run on IBM Quantum (after configuring quell.config.yml)
quell run examples/bell.quell   # backend: ibm in config → submits to IBM

# AI assistant (requires ANTHROPIC_API_KEY)
quell ask "how does Grover's algorithm work?"

# Convert existing Qiskit/Cirq Python to Quell
quell convert my_qiskit_circuit.py
```

---

## Docker

```bash
# Run a circuit with Docker (mount your working directory)
docker run --rm -v $(pwd):/workspace ghcr.io/magnobit/quell:latest \
  run /workspace/examples/bell.quell

# Compile to Qiskit
docker run --rm -v $(pwd):/workspace ghcr.io/magnobit/quell:latest \
  compile --target qiskit /workspace/examples/bell.quell

# With a config file for hardware backends
docker run --rm \
  -v $(pwd):/workspace \
  -e IBM_QUANTUM_TOKEN=$IBM_QUANTUM_TOKEN \
  -e AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY \
  ghcr.io/magnobit/quell:latest \
  run /workspace/examples/bell.quell
```

Build locally:
```bash
docker build -t quell .
docker run --rm -v $(pwd):/workspace quell run /workspace/examples/bell.quell
```

---

## Named qubits

Name your qubits instead of using indices. The compiler maps names to 0-indexed integers automatically.

```quell
qubit alice, bob

H alice          // same as H 0
CNOT alice bob   // same as CNOT 0 1
MEASURE
```

Multiple declarations work too:
```quell
qubit control
qubit target
qubit ancilla

H control
CCX control target ancilla
MEASURE
```

---

## Backend config

Quell never stores credentials. Credentials live in environment variables, expanded at runtime via `${VAR_NAME}` in the config.

### IBM Quantum

```yaml
# quell.config.yml
backend: ibm
ibm:
  token: ${IBM_QUANTUM_TOKEN}
  instance: ibm-q/open/main
  device: ibm_brisbane
  shots: 1024
```

Quell submits the circuit as OpenQASM 3 to the IBM Quantum Runtime (Sampler V2) and polls until complete. Results printed as a measurement histogram.

### AWS Braket

```yaml
backend: aws
aws:
  region: us-east-1
  device: arn:aws:braket:::device/quantum-simulator/amazon/sv1
  s3_bucket: my-braket-results
  s3_prefix: quell-results
  shots: 1000
```

AWS credentials come from environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and optionally `AWS_SESSION_TOKEN`. Quell uses SigV4-signed requests — no AWS SDK required.

### Google Quantum Engine

```yaml
backend: google
google:
  project: my-gcp-project
  processor: rainbow
  shots: 1000
  key_file: /path/to/service-account.json
```

`key_file` is the path to a Google Cloud service account JSON key file with Quantum Engine access. Quell handles JWT creation and OAuth2 token exchange internally.

### Local (default)

```yaml
backend: local
local:
  shots: 1024
```

Previews the OpenQASM 3 output. For full simulation use the [QubitLabs Playground](https://qubitlabs.magnobit.com).

**Switch backends with one line change. Same `.quell` file, zero code changes.**

---

## Language reference

Full spec: [SPEC.md](SPEC.md)

### Named qubits

```quell
qubit alice, bob, carol

H alice
CNOT alice bob
CZ bob carol
MEASURE
```

### Gate reference

| Gate | Syntax | Description |
|---|---|---|
| `H` | `H q` | Hadamard — creates superposition |
| `X` | `X q` | Pauli-X (NOT gate) |
| `Y` | `Y q` | Pauli-Y |
| `Z` | `Z q` | Pauli-Z (phase flip) |
| `S` | `S q` | S gate (√Z) |
| `T` | `T q` | T gate (π/8) |
| `SDG` | `SDG q` | Inverse S (S†) |
| `TDG` | `TDG q` | Inverse T (T†) |
| `SX` | `SX q` | √X gate |
| `RX θ` | `RX 1.5708 q` | Rotate around X-axis by θ radians |
| `RY θ` | `RY 1.5708 q` | Rotate around Y-axis by θ radians |
| `RZ θ` | `RZ 1.5708 q` | Rotate around Z-axis by θ radians |
| `P θ` | `P 0.7854 q` | Phase gate — e^iθ on \|1⟩ |
| `CNOT` | `CNOT c t` | Controlled-NOT |
| `CX` | `CX c t` | Alias for CNOT |
| `CZ` | `CZ q0 q1` | Controlled-Z |
| `SWAP` | `SWAP q0 q1` | Swap two qubits |
| `ISWAP` | `ISWAP q0 q1` | iSWAP — SWAP with i phase |
| `CRX θ` | `CRX 1.5708 c t` | Controlled-RX |
| `CRY θ` | `CRY 1.5708 c t` | Controlled-RY |
| `CRZ θ` | `CRZ 1.5708 c t` | Controlled-RZ |
| `CCX` | `CCX c0 c1 t` | Toffoli (controlled-controlled-NOT) |
| `CSWAP` | `CSWAP c q0 q1` | Fredkin (controlled-SWAP) |
| `MEASURE` | `MEASURE` | Measure all qubits |
| `MEASURE n` | `MEASURE 0` | Measure qubit n |
| `M` | `M` | Alias for MEASURE |

Angles are in radians. Common values: `π/2 = 1.5708`, `π/4 = 0.7854`, `π = 3.1416`.

---

## Compile targets

| Target | Flag | Language | Used by |
|---|---|---|---|
| OpenQASM 3 | `--target openqasm` | openqasm | IBM, AWS, Google, any hardware |
| Qiskit | `--target qiskit` | Python | IBM Quantum / Aer simulator |
| Cirq | `--target cirq` | Python | Google Quantum AI |
| Braket SDK | `--target braket` | Python | AWS Braket |

```bash
quell compile --target qiskit   --output bell_qiskit.py  examples/bell.quell
quell compile --target openqasm --output bell.qasm        examples/bell.quell
```

---

## Use as a Go library

```go
import "github.com/magnobit/quell/compile"

// Compile Quell source to any target
result, err := compile.Compile(`
  qubit alice, bob
  H alice
  CNOT alice bob
  MEASURE
`, compile.Qiskit)
```

Available targets: `compile.Qiskit`, `compile.OpenQASM`, `compile.Cirq`, `compile.Braket`

---

## Repository structure

```
quell/
├── cmd/quell/            — CLI entry point
├── compile/              — Public Go API (github.com/magnobit/quell/compile)
├── internal/
│   ├── parser/           — Quell source → circuit IR (named qubit support)
│   ├── compiler/         — IR → OpenQASM 3 / Qiskit / Cirq / Braket
│   ├── backends/         — Hardware runners: IBM, AWS Braket, Google Quantum
│   └── config/           — quell.config.yml reader + ${ENV_VAR} expansion
├── examples/
│   ├── bell.quell        — Bell pair
│   ├── ghz.quell         — 3-qubit GHZ state
│   ├── grover.quell      — Grover's search (target |11⟩)
│   ├── teleport.quell    — Quantum teleportation
│   └── named_qubits.quell — Named qubit demo
├── Dockerfile            — Multi-stage build (scratch-based static binary)
├── SPEC.md               — Full language specification
├── TRADEMARK.md          — Trademark usage policy
├── CONTRIBUTING.md       — How to contribute
├── SECURITY.md           — Vulnerability reporting
└── NOTICE                — Attribution requirements
```

---

## Bell pair — the same circuit, four languages

**Quell** — 3 lines
```quell
H 0
CNOT 0 1
MEASURE
```

**Qiskit** — 7 lines + imports
```python
from qiskit import QuantumCircuit
qc = QuantumCircuit(2, 2)
qc.h(0)
qc.cx(0, 1)
qc.measure_all()
```

**Cirq** — 6 lines + imports
```python
import cirq
q = cirq.LineQubit.range(2)
circuit = cirq.Circuit([cirq.H(q[0]), cirq.CNOT(q[0], q[1]), cirq.measure(*q)])
```

**Braket** — 5 lines + imports
```python
from braket.circuits import Circuit
circuit = Circuit().h(0).cnot(0, 1)
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Quick checklist:

- Every new gate must be implemented in all four compile targets
- Run `go test ./...` before submitting
- Add `// Copyright 2026 Magnobit. All rights reserved.` to new files
- New backends go in `internal/backends/`

---

## License

Apache 2.0 — see [LICENSE](LICENSE)

Copyright 2026 [Magnobit](https://magnobit.com)

> **Trademark notice:** "Quell" and the Quell hexagon logo are trademarks of Magnobit, Inc.
> The Apache 2.0 license covers the source code; the trademarks are not included.
> See [TRADEMARK.md](TRADEMARK.md) for usage guidelines.
