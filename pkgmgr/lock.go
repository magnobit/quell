// Copyright 2026 Magnobit, Inc. All rights reserved.

package pkgmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LockFile is the optional lock beside quell.pkg.yml.
const LockFile = "quell.pkg.lock.yml"

// LockSchema is the lockfile version. It is not the language version.
const LockSchema = 1

// Lock is a resolved dependency list. An absent file means "no lock".
type Lock struct {
	Schema  int         `yaml:"schema"`
	Require []LockEntry `yaml:"require,omitempty"`
}

// LockEntry is one resolved package. Checksum is sha256 hex of the recorded bytes.
type LockEntry struct {
	Source   string `yaml:"source"`
	Version  string `yaml:"version,omitempty"`
	Resolved string `yaml:"resolved,omitempty"`
	SHA256   string `yaml:"sha256,omitempty"`
}

// LoadLock reads the lockfile. A missing file is an empty lock and a nil error.
func LoadLock(projectRoot string) (*Lock, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, LockFile))
	if os.IsNotExist(err) {
		return &Lock{Schema: LockSchema}, nil
	}
	if err != nil {
		return nil, err
	}
	var lock Lock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse %s: %w", LockFile, err)
	}
	if lock.Schema == 0 {
		lock.Schema = LockSchema
	}
	return &lock, nil
}

// SaveLock writes the lockfile. It does not fetch packages.
func SaveLock(projectRoot string, lock *Lock) error {
	if lock == nil {
		lock = &Lock{}
	}
	if lock.Schema == 0 {
		lock.Schema = LockSchema
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectRoot, LockFile), data, 0644)
}

// Sum returns the sha256 hex of b.
func Sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ChecksumMismatch is a hard error when a lock entry names bytes that differ.
func ChecksumMismatch(source, want, got string) error {
	return fmt.Errorf("checksum mismatch for %s: lock has %s, bytes have %s", source, want, got)
}
