# Publishing Quell on pkg.go.dev

pkg.go.dev indexes **public** Go modules from version-control hosts (GitHub, GitLab, …).
Your module path is already correct:

```
module github.com/magnobit/quell
```

Docs will appear at: **https://pkg.go.dev/github.com/magnobit/quell**

## Checklist (do this when you are ready to go public)

### 1. Repository visibility
- Make **https://github.com/magnobit/quell** public (or open the subtree that contains `go.mod`).
- Private repos do **not** show up on pkg.go.dev for anonymous users.

### 2. License
- pkg.go.dev works with any reachable module, but consumers expect an OSS license
  (Apache-2.0 / MIT / BSD). Today `LICENSE` is proprietary — replace or dual-license
  before advertising “open source” on the Go site.
- Keep a clear `LICENSE` file at the repo root.

### 3. Semantic version tags
You already have tags `v0.0.1` … `v0.0.9`. For the next release:

```bash
cd quell
# ensure tests pass
go test ./...

git add -A
git commit -m "release: v0.1.0"
git tag v0.1.0
git push origin main
git push origin v0.1.0
```

Use **`vMAJOR.MINOR.PATCH`** tags that match Go module rules (leading `v`).

### 4. Trigger indexing
After the tag is on GitHub:

```bash
# Ask the proxy to fetch the module (also works from any machine)
GOPROXY=https://proxy.golang.org go list -m github.com/magnobit/quell@v0.1.0
```

Or open:

`https://pkg.go.dev/github.com/magnobit/quell@v0.1.0`

and click **Request** if the page is not ready yet (can take a few minutes).

### 5. Import paths consumers use

```go
import (
  "github.com/magnobit/quell/compile"
  "github.com/magnobit/quell/qasm"
  "github.com/magnobit/quell/simulate"
  "github.com/magnobit/quell/anneal"
  "github.com/magnobit/quell/log"
  "github.com/magnobit/quell/qerr"
)
```

```bash
go get github.com/magnobit/quell@v0.1.0
```

### 6. Package docs quality (helps the Go site look good)
- Keep a strong root `README.md` (already present).
- Add/keep package comments on public packages (`// Package compile …`).
- Prefer exporting stable APIs from top-level packages (`compile`, `qasm`, `simulate`,
  `adapter`, `anneal`, `log`, `qerr`) rather than `internal/…`.
- Run `go doc github.com/magnobit/quell/compile` locally before tagging.

### 7. Go version
`go.mod` currently says `go 1.25`. That is fine if your users are on that toolchain;
if you want broader adoption, pin to the oldest Go you support (e.g. `go 1.22`) and
test with that version in CI.

### 8. Optional: vanity import / module proxy
Not required for pkg.go.dev. Only needed if you want `import "quell.dev/..."` with
custom hosting.

## What we added for library consumers
- **`quell/log`** — structured `slog` logging (`QUELL_LOG_LEVEL` / `--log-level`)
- **`quell/qerr`** — typed errors (`parse` / `compile` / `convert` / `simulate`) for robust handling

## Control flow (Quell ↔ other languages)
| Construct | Quell | Import (→ Quell) | Export (Quell →) |
|-----------|-------|------------------|------------------|
| IF / ELSE | yes | OpenQASM, Qiskit, Cirq, Q# | OpenQASM3, Qiskit, Cirq*, Q# |
| FOR (bounded) | parse-time unroll | OpenQASM `for i in [lo:hi]`, Qiskit `for i in range` | already unrolled |
| WHILE (bounded MAX) | yes | OpenQASM `while` (MAX 32 default) | OpenQASM3, Qiskit, Q# |
| SWITCH | yes | OpenQASM `switch` | OpenQASM3, Q# (if/elif) |

\* Cirq: classical controls for `IF c[i]==1` only.
