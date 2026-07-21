// Copyright 2026 Magnobit, Inc. All rights reserved.

package adapter

import (
	"fmt"
	"runtime"
)

// RegisterPlugin registers a third-party BackendAdapter in-process.
// Prefer this over shared-object plugins: it works on all platforms (including
// Windows) and keeps the IR → adapter contract type-safe.
//
// Example (in a separate module):
//
//	func init() { adapter.RegisterPlugin(MyProvider{}) }
func RegisterPlugin(a BackendAdapter) error {
	if a == nil {
		return fmt.Errorf("adapter: nil plugin")
	}
	if a.Name() == "" {
		return fmt.Errorf("adapter: plugin Name() is empty")
	}
	Register(a)
	return nil
}

// LoadSharedPlugin would load a Go plugin .so/.dll. Go's plugin package is
// unsupported on Windows and fragile across Go versions — return a clear error
// and point third parties at RegisterPlugin instead.
func LoadSharedPlugin(path string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("adapter: shared-object plugins are not supported on Windows — implement BackendAdapter and call adapter.RegisterPlugin (path=%s)", path)
	}
	return fmt.Errorf("adapter: LoadSharedPlugin is reserved for a future Linux plugin loader — use RegisterPlugin for now (path=%s)", path)
}
