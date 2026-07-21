// Copyright 2026 Magnobit, Inc. All rights reserved.

package adapter

import (
	"fmt"

	"github.com/magnobit/quell/internal/backends"
	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/simulate"
)

func tag(res *Result, requested, engine string) {
	if res == nil {
		return
	}
	if res.Requested == "" {
		res.Requested = requested
	}
	if res.Engine == "" {
		res.Engine = engine
	}
}

// SimulatorAdapter runs IR on Quell's local statevector simulator.
type SimulatorAdapter struct{}

func (SimulatorAdapter) Name() string { return "local" }

func (SimulatorAdapter) Run(job *Job) (*Result, error) {
	if job == nil || job.Program == nil {
		return nil, fmt.Errorf("local: nil job/program")
	}
	shots := job.Shots
	if shots <= 0 {
		shots = 1000
	}
	opt := simulate.Options{Shots: shots}
	if n, ok := job.Noise.(simulate.NoiseModel); ok {
		opt.Noise = n
	} else if n, ok := job.Noise.(*simulate.NoiseModel); ok && n != nil {
		opt.Noise = *n
	}
	res, err := simulate.RunProgramOpts(job.Program, opt)
	if err != nil {
		return nil, err
	}
	return backends.NewResult("local", "local-statevector", "local simulator", "local", shots, res.Counts), nil
}

// IBMAdapter submits IR → OpenQASM → IBM Quantum.
type IBMAdapter struct{}

func (IBMAdapter) Name() string { return "ibm" }

func (IBMAdapter) Run(job *Job) (*Result, error) {
	cfg, err := asIBM(job.Config)
	if err != nil {
		return nil, err
	}
	qasm, nq, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	res, err := backends.RunIBM(cfg, qasm, nq)
	if err != nil {
		return nil, err
	}
	tag(res, "ibm", "ibm-runtime")
	return res, nil
}

// BraketAdapter is AWS Braket (catalog id "aws").
type BraketAdapter struct{}

func (BraketAdapter) Name() string { return "aws" }

func (BraketAdapter) Run(job *Job) (*Result, error) {
	cfg, err := asAWS(job.Config)
	if err != nil {
		return nil, err
	}
	qasm, _, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	res, err := backends.RunBraket(cfg, qasm)
	if err != nil {
		return nil, err
	}
	tag(res, "aws", "braket")
	return res, nil
}

// GoogleAdapter submits via Google Quantum Engine.
type GoogleAdapter struct{}

func (GoogleAdapter) Name() string { return "google" }

func (GoogleAdapter) Run(job *Job) (*Result, error) {
	cfg, err := asGCP(job.Config)
	if err != nil {
		return nil, err
	}
	qasm, _, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	res, err := backends.RunGoogle(cfg, qasm)
	if err != nil {
		return nil, err
	}
	tag(res, "google", "quantum-engine")
	return res, nil
}

// RigettiAdapter submits via Rigetti QCS.
type RigettiAdapter struct{}

func (RigettiAdapter) Name() string { return "rigetti" }

func (RigettiAdapter) Run(job *Job) (*Result, error) {
	cfg, err := asRigetti(job.Config)
	if err != nil {
		return nil, err
	}
	qasm, _, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	res, err := backends.RunRigetti(cfg, qasm)
	if err != nil {
		return nil, err
	}
	tag(res, "rigetti", "qcs")
	return res, nil
}

// IonQAdapter submits via IonQ Cloud.
type IonQAdapter struct{}

func (IonQAdapter) Name() string { return "ionq" }

func (IonQAdapter) Run(job *Job) (*Result, error) {
	cfg, err := asIonQ(job.Config)
	if err != nil {
		return nil, err
	}
	qasm, nq, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	res, err := backends.RunIonQ(cfg, qasm, nq)
	if err != nil {
		return nil, err
	}
	tag(res, "ionq", "ionq-cloud")
	return res, nil
}

// AzureAdapter submits via Azure Quantum.
type AzureAdapter struct{}

func (AzureAdapter) Name() string { return "azure" }

func (AzureAdapter) Run(job *Job) (*Result, error) {
	cfg, err := asAzure(job.Config)
	if err != nil {
		return nil, err
	}
	qasm, _, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	res, err := backends.RunAzure(cfg, qasm)
	if err != nil {
		return nil, err
	}
	tag(res, "azure", "azure-quantum")
	return res, nil
}

// NVIDIAAdapter runs CUDA-Q (or local fallback) from IR.
type NVIDIAAdapter struct{}

func (NVIDIAAdapter) Name() string { return "nvidia" }

func (NVIDIAAdapter) Run(job *Job) (*Result, error) {
	var cfg *config.NVIDIAConfig
	if job.Config != nil {
		c, err := asNVIDIA(job.Config)
		if err != nil {
			return nil, err
		}
		cfg = c
	} else {
		cfg = &config.NVIDIAConfig{}
	}
	cfg.Extra = withQuellSource(cfg.Extra, job.QuellSource)
	qasm, _, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	return backends.RunNVIDIA(cfg, qasm)
}

// IntelAdapter runs Intel path (local fallback today) from IR.
type IntelAdapter struct{}

func (IntelAdapter) Name() string { return "intel" }

func (IntelAdapter) Run(job *Job) (*Result, error) {
	var cfg *config.IntelConfig
	if job.Config != nil {
		c, err := asIntel(job.Config)
		if err != nil {
			return nil, err
		}
		cfg = c
	} else {
		cfg = &config.IntelConfig{}
	}
	cfg.Extra = withQuellSource(cfg.Extra, job.QuellSource)
	qasm, _, err := programQASM(job)
	if err != nil {
		return nil, err
	}
	return backends.RunIntel(cfg, qasm)
}
