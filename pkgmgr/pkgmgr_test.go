// Copyright 2026 Magnobit, Inc. All rights reserved.

package pkgmgr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
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

// ─── hosted-registry source ("registry/<name>") ────────────────────────────

// buildTarball produces a real gzip tarball with one .quell file — the same
// shape both the real internal/packages.Publish accepts and Download
// serves, and what cmd/quell's `pkg publish` command produces.
func buildTarball(t *testing.T, filename, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: filename, Mode: 0644, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// fakeRegistry serves GET /packages/{name}/{version}/download for exactly
// one known name@version, matching the real route pattern registered in
// qubitlabs-platform's main.go — a stand-in for the real hosted registry so
// these tests don't depend on a running server.
func fakeRegistry(t *testing.T, name, version string, tarball []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /packages/{name}/{version}/download", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("name") != name || r.PathValue("version") != version {
			http.Error(w, "package not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(tarball)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("QUELL_REGISTRY_URL", srv.URL)
	return srv
}

func TestGetOne_RegistryPackage_FetchesAndExtracts(t *testing.T) {
	tarball := buildTarball(t, "grover.quell", "H 0\nMEASURE\n")
	fakeRegistry(t, "grover", "1.0.0", tarball)

	projectRoot := t.TempDir()
	if err := GetOne(projectRoot, Requirement{Source: "registry/grover", Version: "1.0.0"}); err != nil {
		t.Fatalf("GetOne: %v", err)
	}

	dest := registryDestPath(projectRoot, "grover", "1.0.0")
	data, err := os.ReadFile(filepath.Join(dest, "grover.quell"))
	if err != nil {
		t.Fatalf("expected grover.quell to exist at %s: %v", dest, err)
	}
	if string(data) != "H 0\nMEASURE\n" {
		t.Errorf("unexpected extracted content: %q", data)
	}
}

func TestGetOne_RegistryPackage_RequiresVersion(t *testing.T) {
	projectRoot := t.TempDir()
	err := GetOne(projectRoot, Requirement{Source: "registry/grover"}) // no Version
	if err == nil {
		t.Fatal("expected an error for a registry requirement with no version")
	}
}

func TestGetOne_RegistryPackage_PropagatesNotFound(t *testing.T) {
	fakeRegistry(t, "grover", "1.0.0", buildTarball(t, "grover.quell", "H 0\nMEASURE\n"))

	projectRoot := t.TempDir()
	// Real package exists at 1.0.0, but this asks for a version that isn't
	// published — must fail, not silently succeed with an empty directory.
	err := GetOne(projectRoot, Requirement{Source: "registry/grover", Version: "9.9.9"})
	if err == nil {
		t.Fatal("expected an error fetching an unpublished version")
	}
}

func TestGetOne_RegistryPackage_IdempotentAtSameVersion(t *testing.T) {
	tarball := buildTarball(t, "grover.quell", "H 0\nMEASURE\n")
	fakeRegistry(t, "grover", "1.0.0", tarball)

	projectRoot := t.TempDir()
	req := Requirement{Source: "registry/grover", Version: "1.0.0"}
	if err := GetOne(projectRoot, req); err != nil {
		t.Fatalf("first GetOne: %v", err)
	}
	// Second call must not error even though the destination now exists —
	// a registry package is immutable per version, so there's nothing to
	// "update" the way a git branch/tag would need re-fetching.
	if err := GetOne(projectRoot, req); err != nil {
		t.Fatalf("second GetOne (already-fetched path): %v", err)
	}
}

// This is the actual gap this whole change closes: quell pkg install used
// to fetch a registry package without ever recording it in quell.pkg.yml,
// so a fresh `quell pkg get` on a clean checkout wouldn't reproduce it.
// AddRequirement + Get must now round-trip a registry source exactly like
// a git source already does (see TestEndToEndImportFromInstalledPackage).
func TestEndToEndReproducibleRegistryInstall(t *testing.T) {
	tarball := buildTarball(t, "grover.quell", "H 0\nMEASURE\n")
	fakeRegistry(t, "grover", "1.0.0", tarball)

	projectRoot := t.TempDir()
	if _, err := AddRequirement(projectRoot, "registry/grover", "1.0.0"); err != nil {
		t.Fatalf("AddRequirement: %v", err)
	}

	// Simulate a fresh clone: wipe the fetched package, keep only the
	// manifest, and confirm `quell pkg get`'s equivalent (LoadManifest +
	// Get) reproduces it from scratch.
	if err := os.RemoveAll(filepath.Join(projectRoot, ".quell")); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Require) != 1 || m.Require[0].Source != "registry/grover" || m.Require[0].Version != "1.0.0" {
		t.Fatalf("manifest didn't record the registry requirement: %+v", m.Require)
	}
	if err := Get(projectRoot, m); err != nil {
		t.Fatalf("Get (reproducing from manifest alone): %v", err)
	}

	dest := registryDestPath(projectRoot, "grover", "1.0.0")
	if _, err := os.Stat(filepath.Join(dest, "grover.quell")); err != nil {
		t.Fatalf("registry package not reproduced from manifest: %v", err)
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
