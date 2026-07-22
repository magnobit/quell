# Quell

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://go.dev)
[![Made by Magnobit](https://img.shields.io/badge/Made%20by-Magnobit-6C3BD1.svg)](https://magnobit.com)

**Private.** This repo is Magnobit's proprietary quantum compiler/runtime — see [LICENSE](LICENSE). It's the engine behind Qubit Cloud, QubitLabs, and the public, Apache-2.0-licensed [quell-cli](https://github.com/magnobit/quell-cli), which depends on this module but doesn't contain its source. Don't add public-facing framing (open-source badges, public install instructions, external contributor workflow) back into this README — that belongs in quell-cli, not here.

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

---

## Quick start

Internal/dev use only — this repo is private, so `go install` and Docker pulls only work for people with access to it. The public artifact is [quell-cli](https://github.com/magnobit/quell-cli); its binaries are what external users actually download.

```bash
# Build from source (requires Go 1.25+, repo access)
git clone https://github.com/magnobit/quell
cd quell && go build ./cmd/quell

# Run locally (shows OpenQASM preview)
quell run examples/bell.quell

# Compile to any backend format
quell compile --target qiskit   examples/bell.quell
quell compile --target openqasm examples/bell.quell
quell compile --target cirq     examples/bell.quell
quell compile --target braket   examples/bell.quell

# Disable the IR optimizer (on by default) to see the raw, unoptimized output
quell compile --target qiskit --no-optimize examples/bell.quell

# Run on IBM Quantum (after configuring quell.config.yml)
quell run examples/bell.quell   # backend: ibm in config → submits to IBM

# Format a .quell file (canonical style: uppercase gate names, aligned comments)
quell fmt --write examples/bell.quell
quell fmt --check examples/bell.quell   # for CI: exits 1 if not already formatted

# Language server for editor integration (diagnostics + format-on-save)
quell lsp

# Package manager — packages are git repos, no hosted registry
quell pkg add github.com/someuser/quell-gates
quell pkg list

# AI assistant (requires ANTHROPIC_API_KEY)
quell ask "how does Grover's algorithm work?"

# Convert existing Qiskit/Cirq Python to Quell
quell convert my_qiskit_circuit.py
```

---

## Docker

No public image — this repo is private. Build locally:
```bash
docker build -t quell .
docker run --rm -v $(pwd):/workspace quell run /workspace/examples/bell.quell
docker run --rm -v $(pwd):/workspace quell compile --target qiskit /workspace/examples/bell.quell

# With a config file for hardware backends
docker run --rm \
  -v $(pwd):/workspace \
  -e IBM_QUANTUM_TOKEN=$IBM_QUANTUM_TOKEN \
  -e AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY \
  quell run /workspace/examples/bell.quell
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

### IonQ Cloud

```yaml
backend: ionq
ionq:
  api_key: ${IONQ_API_KEY}
  device: simulator     # or a QPU name, e.g. qpu.harmony
  shots: 1024
```

Quell submits the circuit as OpenQASM 3 to IonQ Cloud's REST job API and polls until complete, same submit → poll → results shape as the other backends.

### Rigetti QCS

```yaml
backend: rigetti
rigetti:
  api_key: ${RIGETTI_API_KEY}
  device: Aspen-M-3
  shots: 1024
```

Rigetti's production stack is normally accessed via pyQuil/gRPC (Quil-T), not a plain REST job API. This adapter targets Rigetti's public REST job-submission surface, modeled with the same submit → poll → results shape as every other Quell backend.

### Azure Quantum

```yaml
backend: azure
azure:
  tenant_id: ${AZURE_TENANT_ID}
  client_id: ${AZURE_CLIENT_ID}
  client_secret: ${AZURE_CLIENT_SECRET}
  subscription_id: ${AZURE_SUBSCRIPTION_ID}
  resource_group: my-resource-group
  workspace: my-quantum-workspace
  target: ionq.simulator
  shots: 500
```

Auth uses the Azure AD OAuth2 client-credentials flow (service principal). Quell exchanges your credentials for a bearer token, then submits, polls, and fetches results from the configured workspace/target.

### D-Wave — QUBO / annealer (not gate Quell)

Gate-model `.quell` cannot run on D-Wave. Use QUBO text instead:

```bash
quell anneal run problem.qubo
```

```
# or // comments
n 2
h 0 -1
h 1 -1
q 0 1 2
```

With `DWAVE_API_TOKEN` and the Ocean SDK, samples go to Leap; otherwise Quell
uses local simulated annealing. Cloud: pick D-Wave and submit `kind=qubo`.
Set `QUELL_DWAVE_REQUIRE_LEAP=1` to refuse the local fallback.

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
| OpenQASM 2 | `--target openqasm2` | openqasm | Legacy QASM tools |
| Qiskit | `--target qiskit` | Python | IBM Quantum / Aer simulator |
| Cirq | `--target cirq` | Python | Google Quantum AI |
| Braket SDK | `--target braket` | Python | AWS Braket |
| Q# | `--target qsharp` | Q# | Azure Quantum / Microsoft QDK |

```bash
quell compile --target qiskit   --output bell_qiskit.py  examples/bell.quell
quell compile --target openqasm --output bell.qasm        examples/bell.quell
quell compile --target qsharp   --output bell.qs         examples/bell.quell
```

---

## Use as a Go library

```go
import "github.com/magnobit/quell/compile"

// Compile Quell source to any target (optimizer enabled by default)
result, err := compile.Compile(`
  qubit alice, bob
  H alice
  CNOT alice bob
  MEASURE
`, compile.Qiskit)

// Or get warnings and optimizer notes alongside the compiled code, and
// control whether the IR optimizer runs:
r, err := compile.CompileWithWarnings(src, compile.Qiskit, true /* optimize */)
r.Code           // compiled source
r.Warnings       // non-fatal semantic warnings (e.g. missing MEASURE)
r.OptimizerNotes // e.g. "removed 2 redundant gate(s) on qubit 0"
```

Available targets: `compile.Qiskit`, `compile.OpenQASM`, `compile.Cirq`, `compile.Braket`

For real hardware execution (what quell-cli and Qubit Cloud's hosted API both use), see `github.com/magnobit/quell/execute` — it re-exports the per-backend credential types and `RunIBM`/`RunAWS`/`RunGoogle`/`RunRigetti`/`RunIonQ`/`RunAzure`/`RunDWave`, plus `Config`/`Load`/`Default` for `quell.config.yml`-based callers, so nothing outside this module needs to touch `internal/backends` or `internal/config` directly.

---

## Repository structure

```
quell/
├── cmd/quell/            — Internal/dev CLI entry point (the public artifact is the quell-cli repo, not this)
├── compile/              — Public Go API (github.com/magnobit/quell/compile)
├── execute/              — Public Go API for real hardware execution (github.com/magnobit/quell/execute) — what quell-cli and Qubit Cloud's hosted API both import, since Go's internal/ visibility rules block them from reaching internal/backends or internal/config directly
├── format/               — Public Go API for the canonical formatter (github.com/magnobit/quell/format) — quell fmt
├── lsp/                  — Public Go API for the language server (github.com/magnobit/quell/lsp) — quell lsp, JSON-RPC/stdio
├── pkgmgr/               — Public Go API for the package manager (github.com/magnobit/quell/pkgmgr) — quell pkg, git-based fetch into .quell/pkg/
├── internal/
│   ├── parser/           — Quell source → circuit AST (named qubit support, file imports — see imports.go)
│   ├── ir/               — Backend-independent IR (parser AST → ir.Program)
│   ├── optimizer/        — Conservative IR optimizer passes
│   ├── compiler/         — IR → OpenQASM 3 / Qiskit / Cirq / Braket
│   ├── backends/         — Hardware runners: IBM, AWS Braket, Google, IonQ, Rigetti, Azure Quantum
│   └── config/           — quell.config.yml reader + ${ENV_VAR} expansion
├── examples/
│   ├── bell.quell        — Bell pair
│   ├── ghz.quell         — 3-qubit GHZ state
│   ├── grover.quell      — Grover's search (target |11⟩)
│   ├── teleport.quell    — Quantum teleportation
│   └── named_qubits.quell — Named qubit demo
├── Dockerfile            — Multi-stage build (scratch-based static binary), internal use only
├── SPEC.md               — Full language specification
├── SECURITY.md           — Vulnerability reporting
└── LICENSE               — Proprietary — see LICENSE
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

## Internal development checklist

- Every new gate must be implemented in all four compile targets
- Run `go test ./...` before submitting
- Add `// Copyright 2026 Magnobit, Inc. All rights reserved.` to new files
- New backends go in `internal/backends/`

---

## License

Proprietary — see [LICENSE](LICENSE). Copyright 2026 [Magnobit, Inc.](https://magnobit.com) All rights reserved.

The public, Apache-2.0-licensed artifact is [quell-cli](https://github.com/magnobit/quell-cli) — it depends on this module but doesn't contain this source.
