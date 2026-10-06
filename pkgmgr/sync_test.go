// Copyright 2026 Magnobit, Inc. All rights reserved.

package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictRequiresLock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte("require: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, &Manifest{}, Options{Strict: true}); err == nil {
		t.Fatal("strict mode must fail when the lock is missing")
	}
}

func TestMissingLockWarns(t *testing.T) {
	dir := t.TempDir()
	warnings, err := Sync(dir, &Manifest{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

func TestChecksumEnforced(t *testing.T) {
	dir := t.TempDir()
	src := "github.com/example/gates"
	pkg := destPath(dir, src)
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "a.quell"), []byte("H 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := HashTree(pkg)
	if err != nil {
		t.Fatal(err)
	}
	lock := &Lock{Schema: LockSchema, Require: []LockEntry{{Source: src, Version: "v1", SHA256: "deadbeef"}}}
	if err := enforceChecksums(dir, lock); err == nil {
		t.Fatal("checksum mismatch must fail")
	}
	lock.Require[0].SHA256 = sum
	if err := enforceChecksums(dir, lock); err != nil {
		t.Fatal(err)
	}
}
