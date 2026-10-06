// Copyright 2026 Magnobit, Inc. All rights reserved.

package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOldManifestStillLoads(t *testing.T) {
	dir := t.TempDir()
	body := "require:\n  - source: github.com/example/gates\n    version: v1\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Require) != 1 || m.Require[0].Source != "github.com/example/gates" {
		t.Fatalf("%+v", m)
	}
	lock, err := LoadLock(dir)
	if err != nil || len(lock.Require) != 0 {
		t.Fatal(err, lock)
	}
}

func TestLockRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sum := Sum([]byte("tree"))
	lock := &Lock{Require: []LockEntry{{Source: "github.com/example/gates", Version: "abc", SHA256: sum}}}
	if err := SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Require) != 1 || got.Require[0].SHA256 != sum || got.Schema != LockSchema {
		t.Fatalf("%+v", got)
	}
}
