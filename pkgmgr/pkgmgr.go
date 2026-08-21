// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package pkgmgr is Quell's package manager: it fetches packages referenced
// by a project's quell.pkg.yml into .quell/pkg/, where the import resolver
// (see internal/parser's ParseFile) can find them. Two source kinds are
// requirements, resolved uniformly by Get/GetOne:
//
//   - A git repository, addressed by its clone URL with no scheme
//     (e.g. "github.com/someuser/quell-gates") — the same bring-your-own-VCS
//     model Go itself used for dependencies before GOPROXY existed. Fetched
//     into .quell/pkg/<source>/.
//   - A hosted-registry package, addressed as "registry/<name>" with Version
//     set — QubitLabs's own package registry (see the qubitlabs-platform
//     repo's internal/packages and RegistryBase below), a real publish/
//     download service, not a placeholder. Fetched into
//     .quell/pkg/registry/<name>/<version>/ — the same layout `quell pkg
//     install` (cmd/quell/pkgcmd.go) already produced before this file
//     started tracking it in the manifest too; install now also calls
//     AddRequirement so a fresh `quell pkg get` reproduces it.
//
// There is still no *discovery* UI (browsing what's published) — only the
// CLI (`quell pkg install`, `quell pkg publish`) and the registry's raw API
// talk to it today.
package pkgmgr

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the well-known manifest filename at a project's root —
// also what internal/parser's import resolver walks up looking for to
// find the project root.
const ManifestFile = "quell.pkg.yml"

type Manifest struct {
	Require []Requirement `yaml:"require"`
}

type Requirement struct {
	// Source is a git-clonable host+path, e.g. "github.com/someuser/quell-gates" —
	// no scheme, no ".git" suffix; both are added when cloning.
	Source string `yaml:"source"`
	// Version is a branch, tag, or commit — passed to `git clone --branch`
	// (fresh install) or `git fetch origin <version>` (update). Empty means
	// the remote's default branch.
	Version string `yaml:"version,omitempty"`
}

// FindProjectRoot walks up from dir looking for quell.pkg.yml, returning
// the directory that contains it, or "" if none is found — mirrors
// internal/parser's own project-root search exactly, since both need to
// agree on where ".quell/pkg/" lives.
func FindProjectRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func manifestPath(projectRoot string) string {
	return filepath.Join(projectRoot, ManifestFile)
}

// LoadManifest reads quell.pkg.yml under projectRoot. A missing file is not
// an error — it's an empty manifest (a brand new project before its first
// `quell pkg add`).
func LoadManifest(projectRoot string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath(projectRoot))
	if os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	return &m, nil
}

func SaveManifest(projectRoot string, m *Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(projectRoot), data, 0644)
}

// AddRequirement adds source@version to the manifest (updating the version
// in place if source is already required), saves it, and fetches it.
func AddRequirement(projectRoot, source, version string) (*Manifest, error) {
	m, err := LoadManifest(projectRoot)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range m.Require {
		if m.Require[i].Source == source {
			m.Require[i].Version = version
			found = true
			break
		}
	}
	if !found {
		m.Require = append(m.Require, Requirement{Source: source, Version: version})
	}
	if err := SaveManifest(projectRoot, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Get fetches every requirement in m — see GetOne for where each kind lands.
func Get(projectRoot string, m *Manifest) error {
	for _, req := range m.Require {
		if err := GetOne(projectRoot, req); err != nil {
			return fmt.Errorf("%s: %w", req.Source, err)
		}
	}
	return nil
}

const registrySourcePrefix = "registry/"

// GetOne fetches a single requirement.
//   - registry/<name> (Version required): downloads the published tarball
//     from the hosted registry into .quell/pkg/registry/<name>/<version>/.
//     Already present at that exact version → no-op (registry packages are
//     immutable per version, unlike a git branch/tag that can move; there's
//     nothing to "update" without a version bump, which is just a new
//     requirement version, handled the normal way).
//   - anything else: a fresh git clone if not already present under
//     .quell/pkg/, otherwise a fetch + checkout to (re)pin it to req.Version.
func GetOne(projectRoot string, req Requirement) error {
	if req.Source == "" {
		return fmt.Errorf("requirement has no source")
	}

	if name, ok := strings.CutPrefix(req.Source, registrySourcePrefix); ok {
		if req.Version == "" {
			return fmt.Errorf("registry package %q requires a version", name)
		}
		dest := registryDestPath(projectRoot, name, req.Version)
		if _, err := os.Stat(dest); err == nil {
			return nil // already fetched at this exact version
		}
		return fetchRegistryPackage(dest, name, req.Version)
	}

	dest := destPath(projectRoot, req.Source)
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		return updatePackage(dest, req.Version)
	}
	return clonePackage(req.Source, dest, req.Version)
}

// registryDestPath matches the layout cmd/quell/pkgcmd.go's install command
// already wrote to before requirements were tracked in the manifest —
// changing it would silently orphan every package a user installed with an
// older CLI build.
func registryDestPath(projectRoot, name, version string) string {
	return filepath.Join(projectRoot, ".quell", "pkg", "registry", name, version)
}

// RegistryBase returns the hosted-registry API base URL: QUELL_REGISTRY_URL
// if set, otherwise the local dev API. Exported so cmd/quell can share this
// one definition instead of keeping its own copy in sync.
func RegistryBase() string {
	if u := os.Getenv("QUELL_REGISTRY_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8085/api/v1"
}

// fetchRegistryPackage downloads name@version's tarball from the hosted
// registry and extracts its .quell files into dest.
func fetchRegistryPackage(dest, name, version string) error {
	url := fmt.Sprintf("%s/packages/%s/%s/download", RegistryBase(), name, version)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("registry fetch %s@%s: %w", name, version, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registry fetch %s@%s: %s: %s", name, version, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("registry package %s@%s: not a gzip tarball: %w", name, version, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// filepath.Base strips any directory components a malicious or
		// malformed tarball entry might carry (e.g. "../../etc/passwd") —
		// every file lands flat in dest regardless of what the entry claims.
		outPath := filepath.Join(dest, filepath.Base(hdr.Name))
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

// destPath is the local cache directory for source, under
// <projectRoot>/.quell/pkg/. A scheme (if any, e.g. for a file:// source
// used in tests) is stripped first so the cache layout stays predictable —
// real usage never has one ("github.com/user/repo" has no "://"). A
// Windows drive letter left behind by a file:// test fixture (file://C:/...)
// would otherwise leave a bare "C:" path segment, which Windows rejects
// anywhere but the very start of a path — so ":" is stripped too; real
// sources never contain one either.
func destPath(projectRoot, source string) string {
	clean := source
	if i := strings.Index(clean, "://"); i >= 0 {
		clean = clean[i+3:]
	}
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.ReplaceAll(clean, ":", "")
	return filepath.Join(projectRoot, ".quell", "pkg", filepath.FromSlash(clean))
}

func clonePackage(source, dest, version string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	url := cloneURL(source)
	args := []string{"clone", "--depth", "1"}
	if version != "" {
		args = append(args, "--branch", version)
	}
	args = append(args, url, dest)

	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone %s: %w: %s", url, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// cloneURL turns a bare "source" (e.g. "github.com/someuser/quell-gates")
// into a real clone URL. A source that already has a scheme (file://,
// ssh://, git://, https://) is passed through unchanged — this is what
// lets tests point at a local fixture repo without network access, and
// lets real users opt into ssh:// for a private package.
func cloneURL(source string) string {
	if strings.Contains(source, "://") {
		return source
	}
	return "https://" + source + ".git"
}

func updatePackage(dest, version string) error {
	ref := version
	if ref == "" {
		ref = "HEAD"
	}
	if out, err := exec.Command("git", "-C", dest, "fetch", "--depth", "1", "origin", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", dest, "checkout", "FETCH_HEAD").CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns the source paths of packages actually present on disk under
// <projectRoot>/.quell/pkg/ — read from the filesystem, not the manifest,
// so it reflects reality even if the two have drifted.
func List(projectRoot string) ([]string, error) {
	root := filepath.Join(projectRoot, ".quell", "pkg")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var sources []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			rel, relErr := filepath.Rel(root, filepath.Dir(path))
			if relErr == nil {
				sources = append(sources, filepath.ToSlash(rel))
			}
			return filepath.SkipDir
		}
		return nil
	})
	return sources, err
}
