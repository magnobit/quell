// Copyright 2026 Magnobit, Inc. All rights reserved.

package qir

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pyqirCheck loads bitcode with pyqir 0.12 and checks the base-profile
// subset this package emits. It is a structural profile check, not an
// execution of the circuit on qir-runner.
const pyqirCheck = `
import sys
import pyqir

allowed_body = {
    "__quantum__qis__h__body",
    "__quantum__qis__x__body",
    "__quantum__qis__y__body",
    "__quantum__qis__z__body",
    "__quantum__qis__s__body",
    "__quantum__qis__t__body",
    "__quantum__qis__s__adj",
    "__quantum__qis__t__adj",
    "__quantum__qis__rx__body",
    "__quantum__qis__ry__body",
    "__quantum__qis__rz__body",
    "__quantum__qis__cnot__body",
    "__quantum__qis__cz__body",
    "__quantum__qis__swap__body",
}
data = open(sys.argv[1], "rb").read()
mod = pyqir.Module.from_bitcode(pyqir.Context(), data, "quell")
err = mod.verify()
if err is not None:
    raise SystemExit("verify: " + err)
if pyqir.qir_major_version(mod) != 2 or pyqir.qir_minor_version(mod) != 0:
    raise SystemExit("qir version %s.%s" % (pyqir.qir_major_version(mod), pyqir.qir_minor_version(mod)))
if pyqir.dynamic_qubit_management(mod) is not False or pyqir.dynamic_result_management(mod) is not False:
    raise SystemExit("dynamic management is not base profile")
entries = [f for f in mod.functions if pyqir.is_entry_point(f)]
if len(entries) != 1:
    raise SystemExit("expected one entry point, got %d" % len(entries))
ep = entries[0]
profile = ep.attributes.func["qir_profiles"].string_value
if profile != "base_profile":
    raise SystemExit("profile %s" % profile)
nq = pyqir.required_num_qubits(ep)
nr = pyqir.required_num_results(ep)
if nq is None or nr is None:
    raise SystemExit("missing required counts")
blocks = {b.name: b for b in ep.basic_blocks}
for name in ("entry", "body", "measurements", "output"):
    if name not in blocks:
        raise SystemExit("missing block " + name)

def calls(block):
    out = []
    for inst in block.instructions:
        if isinstance(inst, pyqir.Call):
            out.append(inst)
    return out

def qid(value):
    got = pyqir.ptr_id(value)
    if got is None:
        raise SystemExit("static id missing")
    return got

entry_calls = calls(blocks["entry"])
if len(entry_calls) != 1 or entry_calls[0].callee.name != "__quantum__rt__initialize":
    raise SystemExit("entry block must only initialize")
max_q = -1
for inst in calls(blocks["body"]):
    name = inst.callee.name
    if name not in allowed_body:
        raise SystemExit("body call " + name)
    args = list(inst.args)
    if name in ("__quantum__qis__rx__body", "__quantum__qis__ry__body", "__quantum__qis__rz__body"):
        ids = [qid(args[1])]
    else:
        ids = [qid(a) for a in args]
    for i in ids:
        if i > max_q:
            max_q = i
meas = calls(blocks["measurements"])
if len(meas) != nr:
    raise SystemExit("measurements %d != required_num_results %d" % (len(meas), nr))
for inst in meas:
    if inst.callee.name != "__quantum__qis__mz__body":
        raise SystemExit("measurement call " + inst.callee.name)
    q = qid(list(inst.args)[0])
    if q > max_q:
        max_q = q
out_calls = calls(blocks["output"])
records = [c for c in out_calls if c.callee.name == "__quantum__rt__result_record_output"]
if len(records) != nr:
    raise SystemExit("recorded results %d" % len(records))
if max_q >= nq:
    raise SystemExit("qubit id %d outside required_num_qubits %d" % (max_q, nq))
print("ok")
print("qubits=%d" % nq)
print("results=%d" % nr)
`

// Validate assembles the module and checks it with pyqir against the
// published base-profile subset. A missing tool is an error.
func Validate(text string) error {
	bin, err := exec.LookPath("llvm-as")
	if err != nil {
		return fmt.Errorf("llvm-as is not installed")
	}
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
		if err != nil {
			return fmt.Errorf("python is not installed")
		}
	}
	dir, err := os.MkdirTemp("", "quell-qir")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ll := filepath.Join(dir, "m.ll")
	bc := filepath.Join(dir, "m.bc")
	script := filepath.Join(dir, "check.py")
	if err := os.WriteFile(ll, []byte(text), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(script, []byte(strings.TrimSpace(pyqirCheck)+"\n"), 0o644); err != nil {
		return err
	}
	cmd := exec.Command(bin, "-o", bc, ll)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("llvm-as: %w: %s", err, strings.TrimSpace(string(out)))
	}
	check := exec.Command(py, script, bc)
	out, err := check.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pyqir: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "ok") {
		return fmt.Errorf("pyqir: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
