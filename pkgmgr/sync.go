// Copyright 2026 Magnobit, Inc. All rights reserved.

package pkgmgr

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options controls lock enforcement. Get stays permissive.
// Strict fails when the lock file is missing or a checksum does not match.
// WriteLock records the resolved sources after a successful fetch.
type Options struct {
	Strict    bool
	WriteLock bool
}

// Sync fetches like Get, then enforces a lock when one exists.
// A project with no lock still resolves. Strict mode does not.
func Sync(projectRoot string, m *Manifest, opt Options) ([]string, error) {
	if m == nil {
		var err error
		m, err = LoadManifest(projectRoot)
		if err != nil {
			return nil, err
		}
	}
	lockPath := filepath.Join(projectRoot, LockFile)
	_, statErr := os.Stat(lockPath)
	missing := os.IsNotExist(statErr)
	if statErr != nil && !missing {
		return nil, statErr
	}
	var warnings []string
	if missing {
		if opt.Strict {
			return nil, fmt.Errorf("strict package install requires %s", LockFile)
		}
		warnings = append(warnings, "no quell.pkg.lock.yml; resolving without checksum enforcement")
	}
	lock, err := LoadLock(projectRoot)
	if err != nil {
		return nil, err
	}
	if !missing {
		if err := matchLock(m, lock); err != nil {
			return nil, err
		}
	}
	if err := Get(projectRoot, m); err != nil {
		return nil, err
	}
	if !missing {
		if err := enforceChecksums(projectRoot, lock); err != nil {
			return nil, err
		}
	}
	if opt.WriteLock {
		next := &Lock{Schema: LockSchema}
		for _, req := range m.Require {
			entry := LockEntry{Source: req.Source, Version: req.Version}
			if sum, err := hashPackage(projectRoot, req); err == nil {
				entry.SHA256 = sum
			}
			next.Require = append(next.Require, entry)
		}
		if err := SaveLock(projectRoot, next); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

func matchLock(m *Manifest, lock *Lock) error {
	have := map[string]LockEntry{}
	for _, e := range lock.Require {
		have[e.Source] = e
	}
	for _, req := range m.Require {
		e, ok := have[req.Source]
		if !ok {
			return fmt.Errorf("lock does not contain %s", req.Source)
		}
		if e.Version != req.Version {
			return fmt.Errorf("lock version for %s is %q, manifest says %q", req.Source, e.Version, req.Version)
		}
	}
	return nil
}

func enforceChecksums(projectRoot string, lock *Lock) error {
	for _, e := range lock.Require {
		if e.SHA256 == "" {
			continue
		}
		got, err := hashPackage(projectRoot, Requirement{Source: e.Source, Version: e.Version})
		if err != nil {
			return fmt.Errorf("%s: %w", e.Source, err)
		}
		if got != e.SHA256 {
			return ChecksumMismatch(e.Source, e.SHA256, got)
		}
	}
	return nil
}

func hashPackage(projectRoot string, req Requirement) (string, error) {
	var root string
	if name, ok := strings.CutPrefix(req.Source, registrySourcePrefix); ok {
		root = registryDestPath(projectRoot, name, req.Version)
	} else {
		root = destPath(projectRoot, req.Source)
	}
	return HashTree(root)
}

func HashTree(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
