# Quell Language Specification

Version: 0.3.0

---

## Overview

Quell is a domain-specific language for describing quantum circuits. It is:

- **Declarative**: you describe gates and measurements, not control flow
- **Backend-agnostic**: compiled to OpenQASM 3, Qiskit, Cirq, or Braket
- **Human-readable**: one gate per line, no boilerplate
- **Safe**: the compiler validates arity, angles, and qubit constraints before producing output

---

## File format

Quell programs are plain text files with the `.quell` extension. Files are UTF-8. Line endings are LF or CRLF.

---

## Syntax

### Comments

```quell
// single-line comment
H 0  // inline comment
```

No multi-line comments. Everything from `//` to end-of-line is ignored.

### Instructions

Each non-empty, non-comment line is one instruction:

```
GATE [angle-args...] [qubit...]
```

- `GATE` — gate name, **uppercase only**
- `angle-args` — float arguments come **before** qubit indices (rotation gates)
- `qubit` — zero-indexed integer (0, 1, 2, …) or named qubit

Examples:
```quell
H 0
CNOT 0 1
RX PI/2 0
MEASURE
```

### Angle arguments and PI notation

Rotation gates take a float angle in radians. You may write:

| Expression | Value | Example |
|---|---|---|
| plain float | radians | `RX 1.5708 0` |
| integer | promoted to float | `RX 1 0` (= RX 1.0) |
| `PI` | 3.14159… | `RZ PI 0` |
| `PI/2` | π/2 ≈ 1.5708 | `RX PI/2 0` |
| `PI/4` | π/4 ≈ 0.7854 | `P PI/4 0` |
| `2*PI` | 2π ≈ 6.2832 | `RY 2*PI 0` |
| `3*PI/2` | 3π/2 ≈ 4.7124 | `RZ 3*PI/2 0` |

Angles are in radians. `PI` is case-insensitive. Simple `*` and `/` expressions are supported.

**Order**: angle args come **before** qubit indices. `RX PI/2 0` means angle=π/2, qubit=0.

### Qubits

Qubits are referenced by zero-based integer index. The circuit width is inferred from the highest index used.

```quell
H 0       // 1-qubit circuit
H 2       // 3-qubit circuit (qubits 0, 1, 2 all allocated)
```

### Named qubits

Qubits can be given descriptive names using the `qubit` declaration. Names map to integer indices in declaration order.

```quell
qubit alice, bob

H alice        // same as H 0
CNOT alice bob // same as CNOT 0 1
MEASURE
```

Rules:
- `qubit` declarations must appear before the first gate that references the name
- Names are **case-sensitive**
- Mixing named and indexed qubits is supported; indices continue after the last declared name
- Multiple names on one line (comma-separated) or across multiple `qubit` lines

### Whitespace

Leading/trailing whitespace is ignored. Blank lines are ignored. Multiple spaces between tokens are treated as one.

---

## Gate reference

### Single-qubit gates

| Gate | Syntax | Description |
|---|---|---|
| H | `H q` | Hadamard — creates superposition |
| X | `X q` | Pauli-X — bit flip (quantum NOT) |
| Y | `Y q` | Pauli-Y |
| Z | `Z q` | Pauli-Z — phase flip |
| S | `S q` | S gate = √Z |
| T | `T q` | T gate = π/8 phase |
| SDG | `SDG q` | S† (S-dagger, inverse S) |
| TDG | `TDG q` | T† (T-dagger, inverse T) |
| SX | `SX q` | √X gate |

### Rotation gates

Angle argument comes **before** the qubit.

| Gate | Syntax | Description |
|---|---|---|
| RX | `RX θ q` | Rotate around X-axis by θ radians |
| RY | `RY θ q` | Rotate around Y-axis by θ radians |
| RZ | `RZ θ q` | Rotate around Z-axis by θ radians |
| P | `P θ q` | Phase gate (= RZ up to global phase) |
| U | `U θ φ λ q` | General single-qubit unitary U(θ,φ,λ) |

The U gate applies the sequence Rz(λ)·Ry(θ)·Rz(φ). It covers every possible single-qubit gate.

### Two-qubit gates

| Gate | Syntax | Description |
|---|---|---|
| CNOT | `CNOT control target` | Controlled-NOT |
| CZ | `CZ q0 q1` | Controlled-Z |
| SWAP | `SWAP q0 q1` | Swap two qubits |
| ISWAP | `ISWAP q0 q1` | iSWAP gate |
| CRX | `CRX θ control target` | Controlled-RX |
| CRY | `CRY θ control target` | Controlled-RY |
| CRZ | `CRZ θ control target` | Controlled-RZ |

Note: control and target qubits must be different indices.

### Three-qubit gates

| Gate | Syntax | Description |
|---|---|---|
| CCX | `CCX c0 c1 target` | Toffoli gate (controlled-controlled-NOT) |
| CSWAP | `CSWAP control q0 q1` | Fredkin gate (controlled-SWAP) |

### Measurement

| Gate | Syntax | Description |
|---|---|---|
| MEASURE | `MEASURE` | Measure all qubits |
| MEASURE | `MEASURE q` | Measure qubit q |
| MEASURE | `MEASURE q0 q1 …` | Measure listed qubits |

Circuits should end with a `MEASURE` instruction. Without it, the simulation always returns all-zero outcomes.

### Circuit control gates

| Gate | Syntax | Description |
|---|---|---|
| BARRIER | `BARRIER` | Synchronisation barrier across all qubits |
| BARRIER | `BARRIER q0 q1 …` | Barrier on listed qubits only |
| RESET | `RESET q` | Reset qubit to \|0⟩ (for mid-circuit reuse) |

`BARRIER` prevents the backend from reordering gates across the barrier during optimisation.
`RESET` is a mid-circuit operation; it has no effect when used as the last operation on a qubit.

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

### Qiskit (IBM)

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

### Cirq (Google)

```bash
quell compile --target cirq bell.quell
```

```python
import cirq

q = cirq.LineQubit.range(2)
ops = []
ops.append(cirq.H(q[0]))
ops.append(cirq.CNOT(q[0], q[1]))
ops.append(cirq.measure(*q, key='result'))

circuit = cirq.Circuit(ops)
print(circuit)
```

### AWS Braket

```bash
quell compile --target braket bell.quell
```

```python
from braket.circuits import Circuit
from braket.devices import LocalSimulator

circuit = Circuit()
circuit.h(0)
circuit.cnot(0, 1)

device = LocalSimulator()
result = device.run(circuit, shots=1024).result()
print(result.measurement_counts)
```

---

## Optimizer

Compilation lowers the parsed `Circuit` AST into a backend-independent
intermediate representation — `ir.Program`, a flat list of `ir.Op` — before
any code generation happens. Every compile target (OpenQASM 3, Qiskit, Cirq,
Braket) generates its output from this IR, not from the parser AST directly.

By default, the IR is run through a conservative optimizer before codegen.
The optimizer never changes circuit semantics: every pass is purely a
correctness-preserving simplification, and none of them ever reorder or
cancel operations across a `MEASURE`, `BARRIER`, or `RESET` that touches any
of the qubits involved — those instructions are hard synchronisation points.

Three passes run in order:

1. **Zero-angle rotation elimination** — drops `RX`/`RY`/`RZ`/`P`/`CRX`/`CRY`/`CRZ`
   ops whose angle is a multiple of 2π (a no-op), within floating-point
   tolerance.
2. **Adjacent self-inverse cancellation** — drops back-to-back pairs of gates
   that undo each other on the exact same qubit(s) with nothing else
   touching those qubits in between: `X X`, `Y Y`, `Z Z`, `H H`, `S`+`SDG`,
   `T`+`TDG`, `CNOT a b`+`CNOT a b`, `CZ a b`+`CZ a b`, `SWAP a b`+`SWAP a b`,
   and `CCX`/`CSWAP` pairs on identical qubits. Cancellation cascades (`X X
   X X` fully cancels). `ISWAP` is deliberately never cancelled — applying it
   twice is not the identity, so an `ISWAP`/`ISWAP` pair is left alone.
3. **Rotation fusion** — merges consecutive `RX`/`RY`/`RZ`/`P` ops of the
   *same* kind on the *same* single qubit, with nothing else touching that
   qubit in between, into one op whose angle is the sum. The zero-angle
   check re-runs afterward, so a fused pair that sums to a multiple of 2π
   (e.g. `RZ(1.5)` then `RZ(-1.5)`) is then dropped too.

Disable the optimizer with `--no-optimize` on `quell compile`, or pass
`optimize=false` to `compile.CompileWithWarnings` when using Quell as a Go
library. When enabled, any changes the optimizer makes are reported as
human-readable notes (e.g. `removed 2 redundant gate(s) on qubit 0`),
printed by the CLI as `Optimizer: <note>` lines and returned from the Go
library as `CompileResult.OptimizerNotes`.

---

## CLI reference

```
quell run <file>                  Run circuit on configured backend
  backend: local | ibm | aws | google | ionq | rigetti | azure | dwave
quell compile <file>              Compile to target language
  --target openqasm|qiskit|cirq|braket
  --optimize | --no-optimize       Enable/disable the IR optimizer (default: enabled)
  --config path/to/quell.config.yml
  --output out.py
quell serve                       Start HTTP compile server (PORT env var, default 8081)
  --port <port>
quell ask "<question>"            AI assistant (requires ANTHROPIC_API_KEY)
quell convert <file>              Convert Python/Qiskit to Quell
quell version                     Print version
quell help                        Print help
```

---

## Quell config

`quell.config.yml` in the working directory (or path passed with `--config`):

```yaml
backend: local  # local | ibm | aws | google | ionq | rigetti | azure | dwave

local:
  shots: 1024

ibm:
  token: ${IBM_QUANTUM_TOKEN}
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

ionq:
  api_key: ${IONQ_API_KEY}
  device: simulator          # or a QPU name, e.g. qpu.harmony
  shots: 1024

rigetti:
  api_key: ${RIGETTI_API_KEY}
  device: Aspen-M-3
  shots: 1024

azure:
  tenant_id: ${AZURE_TENANT_ID}
  client_id: ${AZURE_CLIENT_ID}
  client_secret: ${AZURE_CLIENT_SECRET}
  subscription_id: ${AZURE_SUBSCRIPTION_ID}
  resource_group: my-resource-group
  workspace: my-quantum-workspace
  target: ionq.simulator
  shots: 500

dwave:
  api_token: ${DWAVE_API_TOKEN}
  solver: Advantage_system6.4
  shots: 100
```

`${VAR_NAME}` is replaced at runtime from environment variables. Quell never stores or transmits credentials.

**IonQ, Rigetti, and Azure Quantum** all follow the same OpenQASM 3 submit →
poll → results shape as IBM/AWS/Google — Quell compiles the circuit once and
submits it to whichever backend is configured.

**D-Wave is not supported.** D-Wave builds quantum annealers, which solve
QUBO/Ising optimization problems by relaxing a system into its ground
state — they have no gate-model execution path at all. A compiled `.quell`
gate circuit cannot run on a D-Wave solver as-is, so `backend: dwave`
always returns an error explaining this rather than silently submitting
something meaningless. Supporting D-Wave properly would require a separate
QUBO/Ising program representation (and Quell syntax to author them), which
is a larger, separate effort from the gate-model compiler documented here.

---

## HTTP compile server

`quell serve` starts a lightweight HTTP server for use as a compilation microservice.

**Endpoints:**

```
GET  /health       → {"status":"ok","service":"quell-compiler","version":"0.2.0"}
POST /compile      → {"code":"...","target":"qiskit"} → {"result":"...","target":"qiskit","language":"python","errorType":"parse|compile"}
```

**Request:**
```json
{
  "code": "H 0\nCNOT 0 1\nMEASURE",
  "target": "qiskit"
}
```

**Response (success):**
```json
{
  "result": "from qiskit import QuantumCircuit\n...",
  "target": "qiskit",
  "language": "python"
}
```

**Response (error):**
```json
{
  "error": "line 2: CNOT requires 2 qubit(s), got 1",
  "errorType": "parse"
}
```

---

## Error reference

### Parse errors

These indicate invalid Quell syntax and prevent compilation.

| Error message | Cause | Fix |
|---|---|---|
| `line N: unknown gate "X"` | Gate name not recognised | Check spelling; valid gates listed in error |
| `line N: H requires 1 qubit(s), got 0` | Too few or too many qubits for this gate | Check gate arity in gate reference |
| `line N: RX requires 1 angle argument(s), got 0` | Missing float angle | Add angle: `RX PI/2 0` |
| `line N: CNOT has duplicate qubit 0` | Control and target qubit are the same | Use different qubit indices |
| `line N: qubit index -1 is negative` | Negative qubit index | Qubit indices start at 0 |
| `line N: unexpected token "foo"` | Token is not a gate name, qubit index, or float | Check for typos |
| `empty circuit: no gate instructions found` | File has only comments or qubit declarations | Add at least one gate |

### Semantic warnings

These do not prevent compilation but indicate likely mistakes.

| Warning | Cause | Fix |
|---|---|---|
| `no MEASURE instruction` | Circuit has no measurement | Add `MEASURE` at the end |
| `RZ(0) is a no-op` | Zero-angle rotation has no effect | Remove the gate |
| `circuit uses N qubits — simulator supports ≤12` | Too wide for browser simulator | Use a smaller circuit or a real backend |
| `circuit depth is N` | Deep circuit will be noisy on real hardware | Use simulator for deep circuits |
| `RX angle 360 — large angle` | Angle may be in degrees instead of radians | Divide by 57.296 to convert |
| `RESET on qubit N has no subsequent gates` | Reset at end of circuit has no effect | Remove, or add gates after RESET |

---

## Quantum error concepts

Understanding these is essential when running on real hardware.

### Decoherence (T1 and T2)

Every real qubit has two coherence times:

- **T1** (energy relaxation) — time for a qubit in |1⟩ to spontaneously decay to |0⟩. Typical values: 50–300 µs on superconducting hardware.
- **T2** (phase coherence) — time before the relative phase between |0⟩ and |1⟩ becomes random. Always T2 ≤ 2·T1.

Circuit execution time must stay well below T1 and T2 or results become noise. Deep circuits (depth > ~100 layers) on current hardware will show significant decoherence degradation.

### Gate fidelity

Real gates are not perfect. A two-qubit gate (CNOT, CZ) on IBM Quantum hardware has typical error rates of 0.1–1%. Error accumulates multiplicatively: 100 CNOT gates at 99.5% fidelity each gives an overall fidelity of 0.995^100 ≈ 61%.

Single-qubit gates are much better (~0.01–0.1% error per gate).

### Readout (measurement) error

Measuring a qubit that is in |1⟩ can incorrectly read as |0⟩, and vice versa. Typical readout error is 0.5–5% per qubit. For multi-qubit measurements this compounds.

### Hardware topology (connectivity)

Real quantum processors do not allow CNOT between arbitrary qubit pairs. IBM Quantum devices have a fixed connectivity graph (e.g. heavy-hex topology). The transpiler inserts SWAP gates to route circuits to the device topology, increasing gate count and depth.

### Crosstalk

Adjacent qubits on the same chip can affect each other even when no gate is applied. This is partially mitigated by BARRIER instructions, which group gates into explicit synchronised layers.

### What this means for Quell

| Symptom | Likely cause |
|---|---|
| Histogram shows noise (small wrong-state counts) | Gate errors + readout error |
| Expected 50/50 split but heavily skewed | Circuit too deep (decoherence) |
| Results completely wrong | Circuit far exceeds T2 |
| Simulation is perfect but hardware is not | Normal — simulator has no noise |

Use the QubitLabs simulator for algorithm development. Switch to real hardware only to characterise noise behaviour or validate a final circuit.

---

## Roadmap

- [x] Named qubits: `qubit alice, bob` — v0.0.1
- [x] PI angle notation: `RX PI/2 0` — v0.2.0
- [x] BARRIER and RESET gates — v0.2.0
- [x] U gate (general single-qubit unitary) — v0.2.0
- [x] Semantic warnings (no MEASURE, depth, no-op gates) — v0.2.0
- [x] Panic-safe HTTP compile server with error types — v0.2.0
- [x] Backend-independent IR (`internal/ir`) and conservative optimizer (`internal/optimizer`) — v0.3.0
- [x] IonQ, Rigetti, and Azure Quantum backend adapters — v0.3.0
- [ ] Classical registers and conditional gates (`IF c[0]==1 X 1`)
- [ ] Subroutines and gate definitions (`gate bell q0 q1 { H q0; CNOT q0 q1 }`)
- [ ] Parameterized circuits
- [ ] Native noise models (depolarising, amplitude damping)
- [ ] QASM 3.0 full import/export
