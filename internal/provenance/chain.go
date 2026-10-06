// Copyright 2026 Magnobit, Inc. All rights reserved.

package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/optequiv"
	"github.com/magnobit/quell/internal/optimizer"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

// Build records the local chain from source through Verify.
// EstimatedCost and ActualCost stay nil. This function does not submit a job.
func Build(src string, shots int) (Record, error) {
	rec := Record{
		Language:  compile.LanguageVersion,
		Compiler:  compile.CompilerVersion,
		Optimizer: compile.OptimizerVersion,
		Engine:    EngineLocal,
		Backend:   EngineLocal,
	}
	rec.SourceHash = hashText(src)
	circ, err := parser.Parse(src)
	if err != nil {
		return rec, err
	}
	prog := ir.Lower(circ)
	rec.IRHash = hashText(string(ir.CanonicalBytes(prog)))
	opt, _ := optimizer.Optimize(prog)
	ev := optequiv.Compare(prog, opt, optequiv.Options{})
	rec.VerifyStatus = ev.Status
	body, _ := json.Marshal(ev)
	rec.VerifyReport = string(body)
	res, err := simulate.Run(src, shots)
	if err != nil {
		return rec, err
	}
	counts, _ := json.Marshal(res.Counts)
	rec.ResultHash = hashText(string(counts))
	if len(circ.Observables) > 0 {
		raw, _ := json.Marshal(circ.Observables)
		rec.ObservableHash = hashText(string(raw))
	}
	rec.CapabilitySnapshot = hashText(EngineLocal + "|shots")
	return rec, nil
}

// Link attaches ids that already exist in QubitLabs. It does not insert a job row.
func Link(rec Record, jobID, experimentID, provider, backend string, estimated, actual *float64) Record {
	rec.ProviderJob = jobID
	rec.ExperimentID = experimentID
	if provider != "" {
		rec.Provider = provider
	}
	if backend != "" {
		rec.Backend = backend
	}
	rec.EstimatedCost = estimated
	rec.ActualCost = actual
	return rec
}

// AttachSweep notes which existing experiment group a point belongs to.
func AttachSweep(rec Record, group, paramsHash, lockHash string) Record {
	rec.SweepGroup = group
	rec.ParamsHash = paramsHash
	rec.LockHash = lockHash
	return rec
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
