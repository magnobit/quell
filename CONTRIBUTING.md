# Contributing to Quell

Thank you for your interest in contributing to Quell — the quantum circuit
language built by Magnobit.

---

## Before you start

- **Bug reports and feature requests:** Open an issue on GitHub first.
  Describe what you expected vs. what happened, and include a minimal
  `.quell` file that reproduces the problem.

- **Significant changes:** If you want to add a new compile target, change
  the parser grammar, or modify the gate set, open an issue to discuss
  before writing code. This avoids wasted effort if the direction doesn't
  fit the project roadmap.

- **Small fixes:** Typos, documentation improvements, and one-line bug
  fixes can go straight to a pull request.

---

## Contributor Licence Agreement (CLA)

By submitting a pull request, you confirm that:

1. You have the right to license the contribution under the Apache 2.0
   License.
2. You grant Magnobit a perpetual, worldwide, non-exclusive, royalty-free
   licence to use, reproduce, modify, and distribute your contribution as
   part of Quell.
3. You understand that your contribution will be publicly attributed in
   the project history.

If your contribution is made on behalf of your employer, you confirm that
your employer has authorised you to make this contribution.

---

## Development setup

```bash
# Clone the repo
git clone https://github.com/magnobit/quell
cd quell

# Requires Go 1.25+
go version

# Build the CLI
go build ./cmd/quell

# Run tests
go test ./...

# Run a circuit
./quell run examples/bell.quell
```

---

## Code style

- **Follow standard Go conventions.** Run `gofmt` and `go vet` before
  committing. No third-party linters are required.
- **Keep it simple.** The Quell parser and compiler are intentionally
  straightforward. Do not introduce abstractions that aren't needed by
  at least two callers.
- **Write tests for new gates.** Every new gate in `compiler.go` must
  have at least one test in `compiler_test.go` covering each compile target.
- **Update the spec.** If you change the language (new gate, new syntax),
  update `SPEC.md` in the same pull request.
- **Copyright header.** All new `.go` files must start with:
  ```go
  // Copyright 2026 Magnobit. All rights reserved.
  // SPDX-License-Identifier: Apache-2.0
  ```

---

## Adding a new gate

1. Add the gate to `internal/parser/parser.go` if it needs special
   tokenisation (most gates don't).
2. Add the gate mapping in all four compiler targets in
   `internal/compiler/compiler.go`:
   - `instToOpenQASM`
   - `instToQiskit`
   - `instToCirq`
   - `instToBraket`
3. Add an example in the appropriate example file under `examples/`.
4. Add the gate to the gate reference table in `SPEC.md`.
5. Add a test in `internal/compiler/compiler_test.go`.

---

## Adding a new compile target

New targets (e.g. Pennylane, Q#, CUDA Quantum) are welcome. You need to:

1. Add a new `Target` constant in `internal/compiler/compiler.go`.
2. Implement the full `instToXxx` function covering all gates defined in
   `SPEC.md`. Partial targets are not accepted.
3. Register the target in the CLI (`cmd/quell/main.go`).
4. Register it in the public package (`compile/compile.go`).
5. Add tests for all gates in the new target.
6. Update the compile targets table in `README.md` and `SPEC.md`.

---

## Pull request checklist

- [ ] `go test ./...` passes
- [ ] `gofmt ./...` produces no diff
- [ ] New gates are implemented in all four compile targets
- [ ] `SPEC.md` is updated if the language changed
- [ ] Copyright header is present on all new files
- [ ] The PR description explains WHY, not just WHAT

---

## Versioning

Quell follows [Semantic Versioning](https://semver.org/):

- **Patch (0.1.x):** Bug fixes, documentation. No language changes.
- **Minor (0.x.0):** New gates, new compile targets, new CLI flags.
  Backward-compatible.
- **Major (x.0.0):** Breaking language changes (gate renaming, removed
  syntax). Requires migration guide.

---

## Questions?

Open a GitHub Discussion or email dev@magnobit.com.
