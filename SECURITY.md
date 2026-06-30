# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| 0.1.x (current) | ✅ Yes |
| < 0.1 | ✗ No |

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Email **security@magnobit.com** with:

1. A description of the vulnerability and its potential impact
2. Steps to reproduce (minimal Quell code or CLI command if applicable)
3. The version of Quell you tested against
4. Your name/handle for attribution (optional)

You will receive an acknowledgement within 48 hours. We aim to release a
fix within 14 days of confirming the vulnerability.

## Scope

The Quell compiler processes `.quell` source files locally. It does not
make network connections and does not handle untrusted input by default.
Security-relevant areas include:

- **Parser resource exhaustion:** malformed input causing excessive memory
  or CPU use during parsing
- **Compiler injection:** output that, when executed as Qiskit/Cirq/Braket
  Python, injects unexpected code beyond the circuit definition
- **CLI flag injection:** shell injection via file paths or flag values

## Out of scope

- Vulnerabilities in third-party backends (IBM Quantum, AWS Braket, etc.)
- Issues that require the user to already have code execution on the machine
- Compiler output that produces incorrect (but non-malicious) quantum circuits
