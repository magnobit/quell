// Copyright 2026 Magnobit, Inc. All rights reserved.

package adapter

import "sync"

var builtinsOnce sync.Once

func ensureBuiltins() {
	builtinsOnce.Do(func() {
		Register(SimulatorAdapter{})
		Register(IBMAdapter{})
		Register(BraketAdapter{})
		Register(GoogleAdapter{})
		Register(RigettiAdapter{})
		Register(IonQAdapter{})
		Register(AzureAdapter{})
		Register(NVIDIAAdapter{})
		Register(IntelAdapter{})
	})
}
