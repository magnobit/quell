// Copyright 2026 Magnobit, Inc. All rights reserved.

package pkgmgr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newFixtureRepo creates a real git repository on disk (via the real git
// binary — not a mock) with one .quell file, committed and tagged, so
// GetOne/Get can be tested against a real "git clone" without needing
// network access. Skips the test if git isn't available.
func newFixtureRepo(t *testing.T) (repoDir, fileURL string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoDir = t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.local",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.local",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoDir, "qft.quell"), []byte("H 0\nMEASURE\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")
	run("tag", "v1.0.0")

	fileURL = "file://" + filepath.ToSlash(repoDir)
	return repoDir, fileURL
}

func TestGetOneClonesRealRepo(t *testing.T) {
	_, fileURL := newFixtureRepo(t)
	projectRoot := t.TempDir()

	err := GetOne(projectRoot, Requirement{Source: fileURL, Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("GetOne: %v", err)
	}

	dest := destPath(projectRoot, fileURL)
	data, err := os.ReadFile(filepath.Join(dest, "qft.quell"))
	if err != nil {
		t.Fatalf("expected qft.quell to exist in cloned package: %v", err)
	}
	// Normalize CRLF: on Windows, git's core.autocrlf may check the file
	// out with CRLF line endings even though it was committed as LF — the
	// Quell parser accepts both (see SPEC.md), so this is a platform git
	// behavior to normalize in the test, not something pkgmgr should fight.
	got := strings.ReplaceAll(string(data), "\r\n", "\n")
	if got != "H 0\nMEASURE\n" {
		t.Errorf("unexpected cloned file content: %q", data)
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Errorf("expected a real .git directory in the clone: %v", err)
	}
}

func TestGetOneIsIdempotent(t *testing.T) {
	_, fileURL := newFixtureRepo(t)
	projectRoot := t.TempDir()
	req := Requirement{Source: fileURL, Version: "v1.0.0"}

	if err := GetOne(projectRoot, req); err != nil {
		t.Fatalf("first GetOne: %v", err)
	}
	// Second call must update (fetch+checkout), not fail because the
	// destination already exists.
	if err := GetOne(projectRoot, req); err != nil {
		t.Fatalf("second GetOne (update path): %v", err)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest on a project with no manifest yet: %v", err)
	}
	if len(m.Require) != 0 {
		t.Fatalf("expected an empty manifest, got %+v", m)
	}

	if _, err := AddRequirement(dir, "github.com/someuser/quell-gates", "v1.0.0"); err != nil {
		t.Fatalf("AddRequirement: %v", err)
	}

	m2, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest after add: %v", err)
	}
	if len(m2.Require) != 1 || m2.Require[0].Source != "github.com/someuser/quell-gates" || m2.Require[0].Version != "v1.0.0" {
		t.Fatalf("unexpected manifest content: %+v", m2)
	}

	// Adding the same source again with a new version should update in
	// place, not append a duplicate entry.
	if _, err := AddRequirement(dir, "github.com/someuser/quell-gates", "v2.0.0"); err != nil {
		t.Fatalf("AddRequirement (update): %v", err)
	}
	m3, _ := LoadManifest(dir)
	if len(m3.Require) != 1 || m3.Require[0].Version != "v2.0.0" {
		t.Fatalf("expected the existing entry updated in place, got: %+v", m3.Require)
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ManifestFile), []byte("require: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	got := FindProjectRoot(sub)
	// Resolve both sides through EvalSymlinks-free Abs comparison since
	// t.TempDir() can include a symlinked prefix on some platforms.
	wantAbs, _ := filepath.Abs(root)
	gotAbs, _ := filepath.Abs(got)
	if gotAbs != wantAbs {
		t.Errorf("FindProjectRoot(%q) = %q, want %q", sub, got, root)
	}

	noManifest := t.TempDir()
	if got := FindProjectRoot(noManifest); got != "" {
		t.Errorf("expected \"\" with no manifest anywhere, got %q", got)
	}
}

func TestListReflectsDisk(t *testing.T) {
	_, fileURL := newFixtureRepo(t)
	projectRoot := t.TempDir()

	empty, err := List(projectRoot)
	if err != nil {
		t.Fatalf("List on a project with no packages yet: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no packages, got %v", empty)
	}

	if err := GetOne(projectRoot, Requirement{Source: fileURL, Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}

	got, err := List(projectRoot)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one installed package, got %v", got)
	}
}

func TestEndToEndImportFromInstalledPackage(t *testing.T) {
	_, fileURL := newFixtureRepo(t)
	projectRoot := t.TempDir()

	if _, err := AddRequirement(projectRoot, fileURL, "v1.0.0"); err != nil {
		t.Fatalf("AddRequirement: %v", err)
	}
	m, err := LoadManifest(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := Get(projectRoot, m); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// The installed package's qft.quell must now be at exactly the path
	// internal/parser's import resolver expects: <root>/.quell/pkg/<source>/qft.quell.
	dest := destPath(projectRoot, fileURL)
	if _, err := os.Stat(filepath.Join(dest, "qft.quell")); err != nil {
		t.Fatalf("installed package file not where the import resolver would look for it: %v", err)
	}
}
