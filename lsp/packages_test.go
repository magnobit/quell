// Copyright 2026 Magnobit, Inc. All rights reserved.

package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magnobit/quell/pkgmgr"
)

func TestPackageLockDiagnostics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "quell.pkg.yml"), []byte("schema: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "lib.quell")
	body := []byte("gate bell a b {\n  H a\n  CNOT a b\n}\n")
	if err := os.WriteFile(lib, body, 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.quell")
	src := "import \"./lib.quell\"\nbell 0 1\nMEASURE\n"
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(main)

	lock := &pkgmgr.Lock{Schema: pkgmgr.LockSchema, Require: []pkgmgr.LockEntry{{
		Source: "./lib.quell", Version: "v1", SHA256: pkgmgr.Sum(body),
	}}}
	if err := pkgmgr.SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	if msgs := diagMessages(packageDiagnostics(uri, src)); hasPrefix(msgs, "checksum") || hasPrefix(msgs, "unresolved") {
		t.Fatalf("valid lock: %v", msgs)
	}

	lock.Require[0].SHA256 = "deadbeef"
	if err := pkgmgr.SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	if !hasPrefix(diagMessages(packageDiagnostics(uri, src)), "checksum") {
		t.Fatal("wrong checksum was not diagnosed")
	}

	lock.Require = []pkgmgr.LockEntry{
		{Source: "github.com/example/gates", Version: "v1"},
		{Source: "github.com/example/gates", Version: "v2"},
	}
	if err := pkgmgr.SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	if !hasPrefix(diagMessages(packageDiagnostics(uri, src)), "version conflict") {
		t.Fatal("version conflict was not diagnosed")
	}

	if err := os.Remove(filepath.Join(dir, pkgmgr.LockFile)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUELL_PKG_STRICT", "1")
	if !hasPrefix(diagMessages(packageDiagnostics(uri, src)), "strict") {
		t.Fatal("strict mode did not require a lock")
	}

	missing := "import \"./missing.quell\"\nMEASURE\n"
	if !hasPrefix(diagMessages(packageDiagnostics(uri, missing)), "unresolved") && !hasPrefix(diagMessages(packageDiagnostics(uri, missing)), "strict") {
		t.Fatal("missing import was not unresolved")
	}
	foundUnresolved := false
	for _, m := range diagMessages(packageDiagnostics(uri, missing)) {
		if strings.Contains(m, "unresolved") {
			foundUnresolved = true
		}
	}
	if !foundUnresolved {
		t.Fatal("missing import was not unresolved")
	}
}

func diagMessages(ds []diagnostic) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Message
	}
	return out
}

func hasPrefix(msgs []string, prefix string) bool {
	for _, m := range msgs {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}
