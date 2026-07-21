// Copyright 2026 Magnobit, Inc. All rights reserved.

package adapter

import (
	"fmt"

	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/internal/ir"
)

func programQASM(job *Job) (qasm string, nq int, err error) {
	if job == nil || job.Program == nil {
		return "", 0, fmt.Errorf("adapter: nil program")
	}
	if ir.NeedsBind(job.Program) {
		return "", 0, fmt.Errorf("adapter: unbound parameters %v", ir.UnboundParams(job.Program))
	}
	code, _, err := compiler.CompileProgram(job.Program, compiler.TargetOpenQASM, job.Optimize)
	if err != nil {
		return "", 0, err
	}
	nq = job.Program.NumQubits
	if nq < 1 {
		nq = 1
	}
	return code, nq, nil
}

func withQuellSource(cfgExtra map[string]string, quellSource string) map[string]string {
	if quellSource == "" {
		return cfgExtra
	}
	out := map[string]string{}
	for k, v := range cfgExtra {
		out[k] = v
	}
	out["quell_source"] = quellSource
	return out
}

func asIBM(cfg any) (*config.IBMConfig, error) {
	c, ok := cfg.(*config.IBMConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("ibm: Config must be *config.IBMConfig")
	}
	return c, nil
}

func asAWS(cfg any) (*config.AWSConfig, error) {
	c, ok := cfg.(*config.AWSConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("aws: Config must be *config.AWSConfig")
	}
	return c, nil
}

func asGCP(cfg any) (*config.GCPConfig, error) {
	c, ok := cfg.(*config.GCPConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("google: Config must be *config.GCPConfig")
	}
	return c, nil
}

func asRigetti(cfg any) (*config.RigettiConfig, error) {
	c, ok := cfg.(*config.RigettiConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("rigetti: Config must be *config.RigettiConfig")
	}
	return c, nil
}

func asIonQ(cfg any) (*config.IonQConfig, error) {
	c, ok := cfg.(*config.IonQConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("ionq: Config must be *config.IonQConfig")
	}
	return c, nil
}

func asAzure(cfg any) (*config.AzureConfig, error) {
	c, ok := cfg.(*config.AzureConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("azure: Config must be *config.AzureConfig")
	}
	return c, nil
}

func asNVIDIA(cfg any) (*config.NVIDIAConfig, error) {
	c, ok := cfg.(*config.NVIDIAConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("nvidia: Config must be *config.NVIDIAConfig")
	}
	return c, nil
}

func asIntel(cfg any) (*config.IntelConfig, error) {
	c, ok := cfg.(*config.IntelConfig)
	if !ok || c == nil {
		return nil, fmt.Errorf("intel: Config must be *config.IntelConfig")
	}
	return c, nil
}
