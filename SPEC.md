# Quell Language Specification

Version: 0.10.0

---

## Overview

Quell is a domain-specific language for quantum circuits and a limited amount of dynamic control. It is:

- **Provider-independent**: source is not a provider SDK. Compile targets today are OpenQASM 3, OpenQASM 2, Qiskit, Cirq, Braket, and Q#.
- **Human-readable**: one gate per line, plus block forms for control flow and macros.
- **Checked**: the compiler validates arity, angles, and qubit constraints before producing output.

Language version in this document is **0.10.0**. That number is not the CLI version and not the IR schema version. 0.9.0 adds host arrays, qubit registers, `quantum fn`, observables, and string interpolation in `print` / `println`. 0.10.0 lowers controlled kernels for a reversible subset and adds host task calls. `async`, `await`, `all`, `race`, `timeout`, `cancel`, and `task` are not reserved identifiers. A host function with one of those names keeps the 0.9 meaning. The scheduler builtin is used only when no such function is declared. Host `for` ranges stay inclusive.

### Three control-flow layers

| Layer | What this version does |
|---|---|
| Compile-time | `FOR i IN a..b` unrolls (inclusive, max 65). `gate` macros expand before IR. Angle forms such as `PI/2` fold to a float at parse time. |
| Host / classical runtime | Immutable `let` and mutable `var` of `bool`, `int`, `float`, and `string`. Host `fn`. Lowercase `if`, `for`, and `while` run on the host. `break` and `continue` apply only to those host loops. Host arrays, qubit registers, and `quantum fn` kernels exist (0.9.0). No recursion. A function cannot read a `PARAM`. |
| QPU dynamic control | `IF` / `ELSE`, `WHILE … MAX`, `SWITCH`, mid-circuit `MEASURE`, `RESET`, and `ASSERT` run against the classical bit register during local simulation. The optimizer treats these as barriers. |

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

### File imports

```quell
import "./lib/bell_pair.quell"
MEASURE
```

`import "path"` splices another file's source in at that point — this is preprocessor-`#include` semantics, **not** a module/namespace system: there is no scoping, and an imported file's named qubits share the importing file's qubit namespace (as if its text had been pasted in directly). Gate definitions (`gate … { }`) are the callable/parameterized unit for reusable fragments within a single file; imports remain paste-in circuit text.

Rules:
- A path starting with `.` (`./` or `../`) is a plain relative filesystem path, resolved against the importing file's own directory.
- Any other path (e.g. `github.com/someuser/quell-gates/qft.quell`) is a **package** import, resolved against `.quell/pkg/<path>` under the project root (the nearest ancestor directory containing `quell.pkg.yml`) — see [Package manager](#package-manager) below. Resolving one with no `quell.pkg.yml` anywhere in the tree is an error, not a silent no-op.
- Import cycles (a file importing itself, directly or transitively) are an error. A **diamond** import — the same file reachable via two different, non-cyclic paths — is not a cycle, and is spliced in at each place it's referenced (no deduplication).
- `import` requires a file on disk to resolve paths against: it only works via `quell run`/`quell compile` (or `parser.ParseFile`/`compile.CompileFile` if you're using Quell as a Go library) — not the string-only `Parse`/`Compile` APIs, and not (yet) `quell fmt`/`quell lsp`, which still operate on a single file's own text.

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

### Parameters

```quell
PARAM theta : angle
RX theta 0
MEASURE
```

`PARAM theta` (untyped) is also accepted. Type annotations are `angle`, `float`, or `real`.
Bind at run time: `quell run file.quell --param theta=1.5708`.

`PARAM` is not a host local. A program that declares both `PARAM theta` and `let theta` is rejected.

### Typed host values

```quell
let shots: int = 1000
let theta: float = 1.5708
let enabled: bool = true
let label: string = "baseline"
```

A `let` is an immutable host value. The name, the type, and the initializer are all required. The initializer is a host expression, not a gate angle. `PI` and `PI/2` stay gate-angle spellings. They are not host identifiers.

Types are `bool`, `int`, `float`, and `string`. There is no implicit conversion between `int` and `float`. Write `float(1)` or `int(1.9)` when a conversion is intended. `int` from a float truncates toward zero.

Locals cannot be assigned again. `x = 6` is an error. There is no `var`.

Scope for a file-level `let` is one list for the whole file, in source order. A name is visible only after its `let`. File-level locals are not allowed inside `IF`, `WHILE`, `SWITCH`, `PAR`, `FOR`, or a `gate` body. A `fn` body is a separate lexical scope. See Host functions.

`let` does not become a `PARAM` and does not appear in canonical IR or in compile-target text. A gate angle still needs a numeric token, `PI` notation, or a `PARAM` name. Using a local's name as a gate angle is a name clash, not a substitution.

#### Expressions

| Operators | Types | Result |
|---|---|---|
| `+` `-` `*` `/` | `int` with `int`, or `float` with `float` | same numeric type |
| `%` | `int` with `int` only | `int` |
| `<` `<=` `>` `>=` | two `int`s or two `float`s | `bool` |
| `==` `!=` | two values of the same type | `bool` |
| boolean and, boolean or | `bool` with `bool` | `bool` |
| `!` | `bool` | `bool` |
| unary `-` | `int` or `float` | same type |

Precedence, tightest first: grouping and conversions, unary `!` and `-`, `*` `/` `%`, `+` `-`, ordering comparisons, `==` `!=`, `&&`, `||`.

Conversions are `bool(...)`, `int(...)`, `float(...)`, and `string(...)`. `bool` accepts `bool`, `int` (zero is false), and `float`. It does not accept `string`. `int` accepts an integer `string` such as `"12"` and rejects `"no"`. Division or remainder by zero is an error.

Diagnostics use stable codes (`QL1001` syntax, `QL2001` unknown identifier, `QL2002` duplicate local, `QL2003` PARAM/local collision, `QL2004` type mismatch, `QL2005` invalid operator, `QL2006` invalid conversion, `QL2007` assignment to an immutable local, `QL2008` division by zero). Function codes are listed under Host functions. Each diagnostic carries a line, a column span, a suggested fix, and a docs URL. Tooling can read the same records as JSON.

### Host functions

```quell
fn square(x: float) -> float {
    return x * x
}

fn add(a: int, b: int) -> int {
    return a + b
}

let n: int = add(2, 3)
```

`fn` is a host function. `gate` remains a compile-time quantum macro. `quantum fn` is not in this version.

A function has typed scalar parameters and one declared return type. The types are `bool`, `int`, `float`, and `string`. There are no overloads, default arguments, variadic parameters, generics, closures, or lambdas. Recursion, including mutual recursion, is rejected.

The body may contain immutable `let` declarations, host expressions, calls to other host functions in the same file, lowercase `if` expressions, statement-form host `if`, and `return`. Every reachable path must return the declared type. It may not contain quantum gates, host loops, assignment, async, provider calls, or I/O.

File scope holds `PARAM` names, top-level `let` names, function names, and gate macros. Duplicate names in that scope are errors. A function name must not match a gate macro or a built-in gate. A function sees file-level lets that appear above it, then its own parameters and lets. A parameter may hide a file-level let. A name is not visible before its declaration. A later function may call an earlier one, and an earlier function may call a later one, because signatures are collected before bodies are checked.

`PARAM` is still an externally bindable circuit input. A host function cannot read or assign a `PARAM`. Using a `PARAM` name inside a function is an unknown-identifier error, not a silent conversion into a local.

Calls are ordinary host expressions: `add(2, 3)`. Conversions such as `int(...)` stay conversions, not function calls. Argument count and argument types must match the signature. There is no implicit numeric conversion.

`import` still splices source text. A function in an imported file is visible as if it had been pasted into the importer. The same file reached by two import paths is pasted twice, so a second copy of the function is a duplicate-function error. There is no separate export list.

An unused function is not an IR operation and does not change canonical IR or compile-target text.

| Code | Meaning |
|---|---|
| QL2101 | Unknown function |
| QL2102 | Duplicate function |
| QL2103 | Function and gate share a name |
| QL2104 | Wrong argument count |
| QL2105 | Argument type mismatch |
| QL2201 | No return on any path |
| QL2202 | Return type mismatch |
| QL2203 | Recursion, including mutual recursion |
| QL2204 | Duplicate parameter |

### Host conditionals

Lowercase `if` is **host control**. Uppercase `IF` is **QPU dynamic control** and is specified in [QPU conditionals](#qpu-conditionals). The spelling is case-sensitive: `If` and `iF` remain QPU `IF`. A lowercase `if` is never a gate and never an `ir.Op`. An unused host conditional does not change canonical quantum IR, optimizer barriers, or compile-target text.

There are two host forms. They stay distinct.

#### Conditional expression

```quell
let sign: int = if flag {
    -1
} else {
    1
}

fn abs(x: int) -> int {
    return if x < 0 {
        -x
    } else {
        x
    }
}
```

A conditional expression yields one host scalar.

- The condition has type `bool`.
- Both branches are required.
- Each branch yields one value. A branch is not a list of statements, and it does not use `return`.
- The branch types match exactly. `int` and `float` do not combine.
- The result type is that common type: `bool`, `int`, `float`, or `string`.
- Evaluation is lazy. Only the selected branch runs. `let x: int = if true { 1 } else { 1 / 0 }` succeeds, because the division is not evaluated. Both branches are still typechecked. Division by zero and overflow are reported only for the branch a constant condition selects.
- A branch may declare immutable `let` names. A name is visible only in that branch, and only after its declaration. The other branch may use the same name for a different local. A second declaration of the same name in one branch is an error.
- A branch local may shadow a function parameter or an outer local that is not a circuit `PARAM`. Inside the branch, the inner binding is the one that is visible. Rename and go-to-definition stay inside that block.
- A branch local may not reuse a circuit `PARAM` name.

#### Conditional statement

```quell
fn abs(x: int) -> int {
    if x < 0 {
        return -x
    } else {
        return x
    }
}
```

A conditional statement controls host statements. It does not yield a value. It is allowed only inside `fn`. A file-level value still uses `let name: type = if condition { expr } else { expr }`.

`else` is optional. `else if` is sugar for an else branch whose only statement is another host `if`. The formatter prints that nesting with braces.

A branch may contain:

- immutable `let`
- a nested host `if`
- `return`
- a host call where a call is already valid, inside a `let` or a `return`

A branch may not contain quantum gates, assignment, loops, async, provider calls, or I/O.

Each branch is its own lexical block. Branch locals are visible only inside that block. Sibling branches may reuse a name. A branch local may shadow a function parameter. It may not reuse a circuit `PARAM` name. A function-body `let` still may not reuse a parameter name, because that block already contains the parameters. Rename and go-to-definition use the innermost block.

A function with a declared return type must return on every reachable path.

```quell
fn sign(x: int) -> int {
    if x < 0 {
        return -1
    } else {
        return 1
    }
}
```

`if x < 0 { return -1 }` with no later return does not cover the other path. A function that never returns at all is still a missing return, not a partial path. A statement after a `return` is unreachable. That is a warning. The statements are still typechecked, and the function still compiles.

Only the selected statement branch runs. The condition is evaluated once. A `return` stops the block and becomes the caller's result. Both branches are still typechecked. When the condition is a constant bool, division by zero and overflow are reported only for the selected branch. When the condition is not a constant, those checks wait until the call.

There is no host `for`, `while`, `break`, or `continue`.

| Code | Meaning |
|---|---|
| QL2301 | Host condition must be bool |
| QL2302 | Conditional branches have different types |
| QL2303 | Conditional expression requires else |
| QL2304 | Invalid host block value |
| QL2305 | Not all host paths return |
| QL2306 | Unreachable host statement (warning) |

<a id="host-loops"></a>

### Host loops and mutation

Lowercase `for` and `while` are host loops. They exist only inside `fn`. They are not compile-time `FOR` and not QPU `WHILE ... MAX`.

```quell
fn sum(n: int) -> int {
    var total: int = 0
    for i in 1..n {
        total = total + i
    }
    return total
}
```

`for i in start..end` is an inclusive `int` range. `while condition` repeats while the condition is `bool` and true. `break` and `continue` leave or restart the innermost host loop. They are errors outside a host loop, and they are not valid in `FOR` or `WHILE`.

`let` stays immutable. `var` is mutable host state. Assignment is allowed only to a `var` in scope. A `var` may not reuse a circuit `PARAM` name. Qubits are not mutable host values.

A loop that might not run does not by itself satisfy "every path returns". `while true { return 1 }` does, because the condition is constantly true and the body always returns. A host loop stops after 10000 iterations.

The same return-flow results used for `if` apply here: always, sometimes, or falls through. `break` ends the loop without returning from the function.

<a id="host-output"></a>

### Host output

`print` and `println` are host statements. `format` returns a `string` and writes nothing.

```quell
println("shots", 3)
let msg: string = format("Energy = {:.1f}", 1.5)
```

Arguments to `print` and `println` are separated by one space. `println` adds one trailing newline. Format placeholders are `{}`, `{:d}`, `{:s}`, `{:b}`, and `{:.Nf}` with N from 0 to 12. `{{` is a literal brace. Output goes through one writer: the CLI uses standard output, and tests can capture a buffer. A single write is capped and strings that look like bearer tokens are redacted. `print` inside `gate` or QPU `IF` is rejected.

| Code | Meaning |
|---|---|
| QL2401 | Invalid format specifier |
| QL2402 | Format argument count |
| QL2403 | Format type mismatch |
| QL2405 | print does not yield a value |
| QL2501 | Host loop iteration limit |
| QL2502 | break or continue outside a host loop |
| QL2503 | Invalid host loop condition or range |

<a id="conditionals"></a>

### QPU conditionals

This is uppercase `IF`. It reads the classical bit register during local simulation. It is not lowercase host `if`. Do not send these programs through the host checker.

Conditions may be `c[i]==v`, `c[i]==c[j]`, or `c==v` (little-endian integer over the classical register).

Line form (single gate):

```quell
H 0
MEASURE 0
IF c[0]==1 X 1
MEASURE
```

Block form with optional ELSE:

```quell
H 0
MEASURE 0
IF c[0]==1 {
  X 1
  H 1
}
ELSE {
  Z 1
}
MEASURE
```

Bit-vs-bit and register equality:

```quell
H 0
H 1
MEASURE 0 -> c[0]
MEASURE 1 -> c[1]
IF c[0]==c[1] X 2
MEASURE
```

### SWITCH

```quell
H 0
H 1
MEASURE 0
MEASURE 1
SWITCH c {
  CASE 0: X 2
  CASE 1: Y 2
  CASE 2: Z 2
  DEFAULT: H 2
}
MEASURE
```

Discriminant is `c[i]` or whole register `c`.

### MEASURE targets

```quell
H 0
MEASURE 0 -> c[1]
```

Default `MEASURE q` still writes `c[q]`. Arrow form requires an explicit qubit list.

### PAR (commuting layer)

```quell
PAR {
  H 0
  H 1
}
MEASURE
```

Gates in a `PAR` block must use disjoint qubits. Simulation runs them sequentially; depth treats the block as one parallel layer intent.

### ASSERT (local sim only)

```quell
X 0
MEASURE 0
ASSERT c[0]==1
MEASURE
```

Compile targets emit a comment; local / Practice simulators fail the shot (or run) if the condition is false.

### Loops

`FOR` unrolls at parse/compile time (inclusive range):

```quell
FOR i IN 0..3 {
  H i
}
MEASURE
```

`WHILE` is bounded (required `MAX`) for local simulation safety:

```quell
H 0
MEASURE 0
WHILE c[0]==0 MAX 8 {
  X 0
  MEASURE 0
}
MEASURE
```

Local simulator (and Practice) run a per-shot mid-circuit MEASURE + IF/WHILE/SWITCH path. OpenQASM 3 export emits `if` / `else` / `while` / `switch`.

### Gate definitions (macros)

Expanded at parse time into built-in gates (not a separate runtime call stack):

```quell
gate bell a b {
  H a
  CNOT a b
}
bell 0 1
MEASURE
```

Single-line form: `gate bell a b { H a; CNOT a b }`.

### Noise models (local sim)

Stochastic (trajectory) channels applied after each gated qubit. Not a density-matrix simulator.

```quell
NOISE depolarizing 0.01
NOISE amplitude_damping 0.05
H 0
CNOT 0 1
MEASURE
```

Or CLI: `quell simulate file.quell --noise depolarizing=0.01 --noise amplitude_damping=0.05`.

| Model | Effect |
|---|---|
| `depolarizing` | With probability *p*, apply X, Y, or Z (each *p*/3) |
| `amplitude_damping` | T1-like jump / damp toward \|0⟩ with rate *γ* (`t1` alias) |
| `phase_damping` | T2-like dephasing — apply Z with probability *p* (`t2` alias) |
| `bit_flip` | Apply X with probability *p* after each gated qubit (`NOISE bit_flip 0.1` or `--noise bit_flip=0.1`) |
| `readout` | Classical SPAM: flip each measured bit with probability *p* |

Hardware compile targets ignore `NOISE`; it only affects local simulation.

---

## Compile targets

### OpenQASM 3

```bash
quell compile --target openqasm bell.quell
# Thin import (common gates):
quell convert bell.qasm
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
quell fmt <file>                  Format a Quell source file
  --write | --check                Reformat in place | exit 1 if not already formatted
quell simulate <file>             Local statevector run, no credentials
  --shots N | --noise model=p      Models: depolarizing, amplitude_damping, phase_damping, bit_flip, readout
quell draw <file>                 ASCII circuit diagram, one row per qubit
quell state <file>                Exact final statevector of the gates before the first MEASURE
  --param name=v | --top N | --json
quell observe <file>              Exact expectation of an `observable` on the local statevector
  --observable name | --param name=v | --json
quell gradient <file>             Central-difference gradient of an expectation over PARAMs
  --observable name | --param name=v | --wrt name | --json
quell vqe <file>                  Nelder-Mead minimisation of an expectation over PARAMs
  --observable name | --param name=start | --max-iter N | --json
quell lsp                         Start the language server (LSP over stdio)
quell pkg add <source> [version]  Add a package to quell.pkg.yml and fetch it
quell pkg get                     Fetch every package in quell.pkg.yml
quell pkg list                    List installed packages
quell serve                       Start HTTP compile server (PORT env var, default 8081)
  --port <port>
quell ask "<question>"            AI assistant (requires ANTHROPIC_API_KEY)
quell convert <file>              Convert Python/Qiskit to Quell
quell version                     Print version
quell help                        Print help
```

---

## Package manager

`quell.pkg.yml` at a project's root lists git-hosted dependencies — there's no hosted registry; a "package" is just a git repo:

```yaml
require:
  - source: github.com/someuser/quell-gates
    version: v1.2.0   # a branch, tag, or commit; omit for the default branch
```

```sh
quell pkg add github.com/someuser/quell-gates v1.2.0   # adds to the manifest, then fetches
quell pkg get                                           # fetches everything already in the manifest
quell pkg list                                          # what's actually installed on disk
```

`quell pkg get`/`add` shell out to the real `git` binary (clone on first fetch, fetch + checkout to update), into `.quell/pkg/<source>/` under the project root — which is exactly where an `import "<source>/<file>.quell"` statement resolves against (see [File imports](#file-imports)). `<source>` is never an npm/PyPI-style short name — it's the literal git host+path, so `import`, `quell pkg add`, and the on-disk cache directory all use the same string.

---

## Quell config

`quell.config.yml` in the working directory (or path passed with `--config`):

```yaml
backend: local  # local | ibm | aws | google | ionq | rigetti | azure | dwave

local:
  shots: 1024

ibm:
  token: ${IBM_QUANTUM_TOKEN}
  instance: ${IBM_QUANTUM_INSTANCE}  # instance CRN
  device: ibm_fez
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
  location: eastus
  target: quantinuum.sim.h2-1e
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

**D-Wave is annealer-only (not gate Quell).** D-Wave solvers take QUBO/Ising
problems, not gate circuits. Use a separate text format via `quell/anneal`
(`ParseQUBO`), Cloud execute with `kind=qubo`, or:

```bash
quell anneal run problem.qubo          # Leap if DWAVE_API_TOKEN + Ocean; else local SA
quell anneal run problem.qubo --local  # force local simulated annealing
```

Minimal `.qubo` format (`#` or `//` comments):

```
# max-cut toy
n 2
h 0 -1
h 1 -1
q 0 1 2
```

Gate-model `backend: dwave` / OpenQASM submit still errors on purpose — do not
send `.quell` gate source to an annealer.

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
| Simulation is perfect but hardware is not | Normal — use `NOISE` / `--noise` to approximate decoherence locally |

Use the QubitLabs simulator for algorithm development. Switch to real hardware only to characterise noise behaviour or validate a final circuit.

---

## Current limitations

- A host `fn` has a return on every reachable path and no recursion. Lowercase `if` is either an expression (required `else`, one value per branch, exact types) or a statement inside `fn`. Only the selected branch is evaluated. A host `fn` body has no loops, assignment, async, provider calls, or I/O. Host `for` and `while` run at top level and never on the QPU. A host function cannot read a `PARAM`. A branch local may shadow an outer local, and may not reuse a `PARAM` name.
- There is no `async` keyword and no thread, mutex, or lock syntax. Host task calls (`async`, `await`, `all`, `race`, `timeout`, `cancel`) submit through the existing QubitLabs scheduler and are not a second scheduler.
- Observable expectation is exact on the local statevector only. Provider-native expectation is not claimed, and the statevector stops at the first `MEASURE`.
- `quell gradient` is a central difference, not the parameter-shift rule. `quell vqe` is Nelder-Mead and can stop at a local minimum.
- `WHILE` requires `MAX` between 1 and 256 and cannot nest.
- `FOR` is parse-time unrolling, not a QPU loop.
- Optimizer equivalence is exact for unitary circuits and unsupported or weaker once dynamic control or noise is present.
- Statevector simulation is capped at 24 qubits. Noise uses the pure-Go simulator.
- NVIDIA CUDA-Q support is a narrow Python path (H, X, CX, measure) with local fallback. Fallback is not GPU execution.
- QIR export is a base-profile subset (`quell inspect --kind qir`). It is structurally checked with `llvm-as` and pyqir when those tools are installed. It has not been executed on qir-runner.
- Compile output is source text for another toolchain. It is not a certificate that a provider will accept or run that text.

---

<a id="host-arrays"></a>

## Host arrays and qubit registers (0.9.0)

`let xs: int[] = [1, 2, 3]` is immutable. `var xs: int[]` allows `xs[i] = value`. `len(xs)` returns an int. A constant index outside `0 .. len-1` is QL2601 at check time. Any other index is checked at runtime. QL2602 is the size limit. QL2603 is an element-type error. Host `for` ranges stay inclusive, so the last valid index is `len(xs)-1`.

`qubit q[4]` allocates four qubits, `q[0]` through `q[3]`. The register lives until the circuit ends. There is no manual free. `RESET` is the existing reset gate.

## Quantum functions (0.9.0)

`quantum fn bell(a: qubit, b: qubit) { H a; CNOT a, b }` is a kernel. `gate` remains a compile-time macro. `fn` remains a host function. A call is inlined into the existing instruction list before canonical IR, so IR gains no call opcode. Recursion is rejected. `print` and other host statements are rejected in the body. `adjoint` and `inverse` rewrite a safe gate subset. `controlled` lowers only gates that already exist in the IR: one control on X, Z, RX, RY, RZ, or SWAP becomes CNOT, CZ, CRX, CRY, CRZ, or CSWAP; two controls on X, or one extra control on CNOT, become CCX. Measurement, reset, dynamic control, and gates without that direct form (including H, Y, S, and T) are rejected. Nested `controlled controlled` counts as two controls.

## Observables (0.9.0)

`observable h = -1.05 * Z(0) + 0.18 * X(0) * X(1)` is a value beside the circuit, not an opcode. `expectation(h)` on the local simulator is the exact statevector expectation of the unitary prefix. RESET and classical control are rejected on that path. Provider-native expectation is not claimed.

## Roadmap

- [x] Named qubits: `qubit alice, bob` — v0.0.1
- [x] PI angle notation: `RX PI/2 0` — v0.2.0
- [x] BARRIER and RESET gates — v0.2.0
- [x] U gate (general single-qubit unitary) — v0.2.0
- [x] Semantic warnings (no MEASURE, depth, no-op gates) — v0.2.0
- [x] Panic-safe HTTP compile server with error types — v0.2.0
- [x] Backend-independent IR (`internal/ir`) and conservative optimizer (`internal/optimizer`) — v0.3.0
- [x] BackendAdapter interface (`quell/adapter`) — IBM, IonQ, Google, Rigetti, Braket(aws), Azure, NVIDIA, Intel, Simulator(local); IR → OpenQASM inside adapters; third parties via `RegisterPlugin` (`.so` plugins deferred — unsupported on Windows)
- [x] IonQ, Rigetti, and Azure Quantum backend adapters — v0.3.0
- [x] File imports (`import "./path.quell"`) and a git-based package manager (`quell pkg`) — v0.4.0
- [x] QUBO / annealer foundation (`quell/anneal` ParseQUBO) — gate circuits still cannot run on D-Wave
- [x] D-Wave Leap submission for QUBO problems (Ocean + token; local SA fallback; `QUELL_DWAVE_REQUIRE_LEAP=1` to refuse fallback)
- [x] NVIDIA cuQuantum / CUDA-Q GPU simulation adapter (`backend: nvidia`; `QUELL_NVIDIA_REQUIRE_CUDAQ=1` to refuse fallback)
- [x] Intel Quantum SDK path (`backend: intel`; local statevector today; `QUELL_INTEL_REQUIRE_SDK=1` to refuse fallback)
- [ ] Direct Quantinuum / QuEra / Pasqal / IQM / OQC / Xanadu adapters (catalogued as planned; often via Azure/AWS today)
- [x] Classical registers and conditional gates (`IF c[0]==1 X 1`) — local sim + OpenQASM 3 export; Practice JS mid-circuit path
- [x] Block `IF` / `ELSE` and bounded `WHILE` / parse-time `FOR` — v0.5 control flow
- [x] Richer conditions (`c[i]==c[j]`, `c==v`), `SWITCH`, `MEASURE q -> c[i]`, `PAR`, typed `PARAM`, `ASSERT`
- [x] Subroutines and gate definitions (`gate bell q0 q1 { H q0; CNOT q0 q1 }`) — parse-time macros
- [x] Decomposition-backed converters (RXX/RYY/RZZ, PhasedXPowGate, …) via `quell/decompose`
- [x] Parameterized circuits (`PARAM theta` / `RX theta 0` + `--param theta=1.57`) — bind before sim/compile
- [x] Immutable host locals (`let name: bool|int|float|string = expr`) — v0.4.0. Not a PARAM, not a function.
- [x] Host functions (`fn name(params) -> type { return expr }`) — v0.5.0. Not a gate macro and not `quantum fn`.
- [x] Host conditionals (`if condition { expr } else { expr }`) — v0.6.0. Lowercase `if` only. Not QPU `IF`.
- [x] Statement-form host `if` inside `fn`, with return-path checking — v0.7.0. Not a host loop and not QPU `IF`.
- [x] Host `for` / `while`, `break` / `continue`, and `var` — v0.8.0. Not compile-time `FOR` and not QPU `WHILE`.
- [x] Host arrays (`int[]` and the other scalar arrays), `len`, and `qubit name[n]` — v0.9.0. Host `for` stays inclusive.
- [x] `quantum fn` kernels, inlined into the existing instruction list — v0.9.0. `gate` stays a macro. `controlled` lowers the reversible subset in v0.10.0 and rejects measurement and reset.
- [x] Pauli observables and exact local `expectation` — v0.9.0. Not an IR opcode.
- [x] `quell draw`, `state`, `observe`, `gradient`, `vqe` and a `bit_flip` noise channel — CLI and library only (`quell/analysis`). No syntax, no IR change. Local statevector, exact, ≤ 24 qubits.
- [x] String interpolation in `println`, using the same formatter as `format` — v0.9.0.
- [x] Native noise models (depolarising, amplitude damping) — stochastic local sim + `NOISE` / `--noise`
- [x] Quantum Digital Twin / multi-provider estimate (`quell estimate` + Cloud Benchmark; educational cost models)
- [x] One-click migration + AI optimize (`POST /ai/convert` QASM/Q#; `POST /ai/optimize`; Labs `/migrate` + Assist loop)
- [x] Hardware-aware routing + noise-aware score (`--coupling heavyhex-toy|linear-N`; SWAP insertion)
- [x] Hosted package registry (`POST/GET /api/v1/packages`; `quell pkg install|publish`)
- [x] Research Notebook (`/notebook`; cells, versions, publish to circuits)
- [x] Marketplace circuit checkout (`POST /circuits/{id}/checkout`; fixture + Stripe payment mode)
- [ ] QASM 3.0 full import/export (thin import via `quell convert *.qasm` ships; full parity still open)