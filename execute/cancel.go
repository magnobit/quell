// Copyright 2026 Magnobit, Inc. All rights reserved.

package execute

import (
	"fmt"

	"github.com/magnobit/quell/internal/backends"
)

// Cancel asks a provider to stop an in-flight job identified by
// providerJobID. NVIDIA, Intel, and D-Wave local/fallback paths have no
// remote job to cancel and return nil. Unknown backends are a no-op rather
// than an error — cancel is best-effort.
func Cancel(backend string, creds any, providerJobID string) error {
	if providerJobID == "" {
		return nil
	}
	switch backend {
	case IBM:
		cfg, ok := creds.(*IBMCredentials)
		if !ok || cfg == nil {
			return fmt.Errorf("ibm: credentials required to cancel")
		}
		return backends.CancelIBM(cfg, providerJobID)
	case AWS:
		cfg, ok := creds.(*AWSCredentials)
		if !ok || cfg == nil {
			return fmt.Errorf("aws: credentials required to cancel")
		}
		return backends.CancelBraket(cfg, providerJobID)
	case Google:
		cfg, ok := creds.(*GoogleCredentials)
		if !ok || cfg == nil {
			return fmt.Errorf("google: credentials required to cancel")
		}
		return backends.CancelGoogle(cfg, providerJobID)
	case Rigetti:
		cfg, ok := creds.(*RigettiCredentials)
		if !ok || cfg == nil {
			return fmt.Errorf("rigetti: credentials required to cancel")
		}
		return backends.CancelRigetti(cfg, providerJobID)
	case IonQ:
		cfg, ok := creds.(*IonQCredentials)
		if !ok || cfg == nil {
			return fmt.Errorf("ionq: credentials required to cancel")
		}
		return backends.CancelIonQ(cfg, providerJobID)
	case Azure:
		cfg, ok := creds.(*AzureCredentials)
		if !ok || cfg == nil {
			return fmt.Errorf("azure: credentials required to cancel")
		}
		return backends.CancelAzure(cfg, providerJobID)
	case DWave, NVIDIA, Intel, Local:
		return nil
	default:
		return nil
	}
}
