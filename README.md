# Quell

**The simplest quantum programming language. Backend-agnostic, open source.**

Write quantum circuits once. Run them on any platform — IBM, AWS, Google, or local simulator.

```quell
H 0
CNOT 0 1
MEASURE
```

That's a Bell pair. The most entangled two-qubit state in the universe. Three lines.

---

## vs every other quantum language

| | Quell | Qiskit | Q# | Cirq | OpenQASM |
|---|---|---|---|---|---|
| **Syntax** | Human-readable | Python + boilerplate | .NET + types | Python + boilerplate | Assembly |
| **Install** | None (browser) | pip + 50MB | dotnet SDK | pip + 50MB | None |
| **Backend** | Any | IBM only | Azure only | Google only | Any (machine output) |
| **Credentials** | You own them | IBM account | Azure account | GCP account | — |
| **Open source** | Yes | Yes | Yes | Yes | Yes |
| **Browser native** | Yes | No | No | No | No |
| **AI assistant** | Yes | No | No | No | No |
| **Learn + run same place** | Yes | No | No | No | No |

---

## Bell pair — the same circuit, four languages

**Quell**
```quell
H 0
CNOT 0 1
MEASURE
```

**Qiskit (Python)**
```python
from qiskit import QuantumCircuit, transpile
from qiskit_ibm_runtime import QiskitRuntimeService

qc = QuantumCircuit(2, 2)
qc.h(0)
qc.cx(0, 1)
qc.measure([0, 1], [0, 1])
```

**Q# (.NET)**
```qsharp
operation BellPair() : (Result, Result) {
    use (q1, q2) = (Qubit(), Qubit());
    H(q1);
    CNOT(q1, q2);
    return (M(q1), M(q2));
}
```

**Cirq (Python)**
```python
import cirq
q0, q1 = cirq.LineQubit.range(2)
circuit = cirq.Circuit([
    cirq.H(q0),
    cirq.CNOT(q0, q1),
    cirq.measure(q0, q1)
])
```

---

## Backend config

Quell never holds your credentials. You configure where circuits run:

```yaml
# quell.config.yml
backend: ibm
ibm:
  token: ${IBM_QUANTUM_TOKEN}
  instance: ibm-q/open/main
  device: ibm_brisbane
```

```yaml
backend: aws
aws:
  region: us-east-1
  device: arn:aws:braket:::device/quantum-simulator/amazon/sv1
```

```yaml
backend: google
google:
  project: my-gcp-project
  processor: rainbow
```

```yaml
backend: local   # default, no credentials needed
```

Change the config, same `.quell` file runs on a different machine.

---

## Quick start

```bash
# Install
go install github.com/magnobit/quell/cmd/quell@latest

# Run locally
quell run examples/bell.quell

# Compile to Qiskit
quell compile --target qiskit examples/bell.quell

# Compile to OpenQASM
quell compile --target openqasm examples/bell.quell

# Ask the AI assistant
quell ask "how do I implement Grover search for 3 qubits?"

# Convert Python/Qiskit to Quell
quell convert myqiskit.py
```

---

## Language reference

Full spec: [SPEC.md](SPEC.md)

### Gates

| Gate | Qubits | Description |
|---|---|---|
| `H` | 1 | Hadamard — creates superposition |
| `X` | 1 | Pauli-X (NOT gate) |
| `Y` | 1 | Pauli-Y |
| `Z` | 1 | Pauli-Z (phase flip) |
| `S` | 1 | S gate (√Z) |
| `T` | 1 | T gate (π/8) |
| `RX θ` | 1 | Rotate around X-axis by θ radians |
| `RY θ` | 1 | Rotate around Y-axis by θ radians |
| `RZ θ` | 1 | Rotate around Z-axis by θ radians |
| `CNOT` | 2 | Controlled-NOT |
| `CX` | 2 | Alias for CNOT |
| `CZ` | 2 | Controlled-Z |
| `SWAP` | 2 | Swap two qubits |
| `CCX` | 3 | Toffoli (controlled-controlled-NOT) |
| `MEASURE` | — | Measure all qubits |
| `MEASURE n` | 1 | Measure qubit n |

---

## Compile targets

Quell compiles to:

- `openqasm` — runs on any OpenQASM-compatible hardware
- `qiskit` — IBM Quantum, local Aer simulator
- `cirq` — Google Quantum AI
- `braket` — AWS Braket
- `quil` — Rigetti

---

## Why Quell?

Most quantum languages were built by hardware vendors. They're excellent — but they're built for their own backend. Qiskit is for IBM. Cirq is for Google. Q# is for Azure.

Quell is built for the circuit, not the hardware. The same `.quell` file compiles to any of the above. When IBM releases a better QPU, you don't rewrite your circuits. When Google goes down, you switch to AWS in one config line.

We also believe learning quantum computing should be as simple as opening a browser. Try it at [qubitlabs.magnobit.com](https://qubitlabs.magnobit.com) — no install, no account.

---

## Repository structure

```
quell/
├── cmd/quell/        — CLI
├── internal/
│   ├── parser/       — Quell → AST
│   ├── compiler/     — AST → OpenQASM / Qiskit / Cirq / Braket
│   └── config/       — quell.config.yml reader
├── examples/         — Bell pair, Grover, Teleportation
└── SPEC.md           — Full language specification
```

---

## Contributing

Quell is early. The most useful contributions right now:

1. New compile targets (Quil, QASM3)
2. More example circuits
3. Language server (LSP for VSCode)
4. Test suite for compiler output

---

## License

MIT — [Magnobit](https://www.magnobit.com)
