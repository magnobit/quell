// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package execute is the public API for running a compiled Quell circuit on
// real quantum hardware (IBM Quantum, AWS Braket, Google Quantum Engine,
// IonQ, Rigetti, Azure Quantum). It exists so callers outside this module —
// e.g. a hosted service hitting these backends with per-customer stored
// credentials — can reach the same submit/poll/results logic the quell CLI
// uses, without depending on this module's internal/ packages (which Go's
// visibility rules make unreachable from outside github.com/magnobit/quell).
package execute

import (
	"github.com/magnobit/quell/internal/backends"
	"github.com/magnobit/quell/internal/config"
)

// Backend names, matching the CLI's --backend values and quell.config.yml's
// backend: field.
const (
	Local   = "local"
	IBM     = "ibm"
	AWS     = "aws"
	Google  = "google"
	Rigetti = "rigetti"
	IonQ    = "ionq"
	Azure   = "azure"
	DWave   = "dwave"
	NVIDIA  = "nvidia"
)

// Credential/config types, re-exported so callers don't need (and can't
// have) a direct dependency on internal/config.
type (
	Config             = config.Config
	LocalConfig        = config.LocalConfig
	IBMCredentials     = config.IBMConfig
	AWSCredentials     = config.AWSConfig
	GoogleCredentials  = config.GCPConfig
	RigettiCredentials = config.RigettiConfig
	IonQCredentials    = config.IonQConfig
	AzureCredentials   = config.AzureConfig
	DWaveCredentials   = config.DWaveConfig
	NVIDIACredentials  = config.NVIDIAConfig
)

// Load reads quell.config.yml (or the given path), expanding ${ENV_VAR}
// references. Default returns the zero-value config (local backend, 1024
// shots) for callers with no config file.
func Load(path string) (*Config, error) { return config.Load(path) }
func Default() *Config                  { return config.Default() }

// Result holds measurement counts from any hardware backend.
type Result = backends.RunResult

func RunIBM(cfg *IBMCredentials, qasm3 string, numQubits int) (*Result, error) {
	return backends.RunIBM(cfg, qasm3, numQubits)
}

func RunAWS(cfg *AWSCredentials, qasm3 string) (*Result, error) {
	return backends.RunBraket(cfg, qasm3)
}

func RunGoogle(cfg *GoogleCredentials, qasm3 string) (*Result, error) {
	return backends.RunGoogle(cfg, qasm3)
}

func RunRigetti(cfg *RigettiCredentials, qasm3 string) (*Result, error) {
	return backends.RunRigetti(cfg, qasm3)
}

func RunIonQ(cfg *IonQCredentials, qasm3 string, numQubits int) (*Result, error) {
	return backends.RunIonQ(cfg, qasm3, numQubits)
}

func RunAzure(cfg *AzureCredentials, qasm3 string) (*Result, error) {
	return backends.RunAzure(cfg, qasm3)
}

func RunDWave(cfg *DWaveCredentials, qasm3 string) (*Result, error) {
	return backends.RunDWave(cfg, qasm3)
}

func RunNVIDIA(cfg *NVIDIACredentials, qasm3 string) (*Result, error) {
	return backends.RunNVIDIA(cfg, qasm3)
}
