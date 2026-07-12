// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import "os"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
