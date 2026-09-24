// Copyright 2026 Magnobit, Inc. All rights reserved.

package compile

import (
	"runtime/debug"
	"strings"
)

// Identity is the immutable build that produced this binary's compiler
// and optimizer. Schema constants name the IR/pass format; ModuleVersion
// and VCSRevision name the actual code that ran.
type Identity struct {
	GoVersion     string
	MainPath      string
	MainVersion   string
	ModuleVersion string
	VCSRevision   string
}

// BuildIdentity reads this process's embedded module/VCS info. Quell as
// the main module (CLI / quell tests) uses info.Main; the platform server
// sees quell as a dependency.
func BuildIdentity() Identity {
	id := Identity{ModuleVersion: "unknown"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return id
	}
	id.GoVersion = info.GoVersion
	id.MainPath = info.Main.Path
	id.MainVersion = info.Main.Version
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			id.VCSRevision = s.Value
		}
	}
	if info.Main.Path == "github.com/magnobit/quell" && info.Main.Version != "" {
		id.ModuleVersion = info.Main.Version
	}
	for _, dep := range info.Deps {
		if dep.Path != "github.com/magnobit/quell" {
			continue
		}
		id.ModuleVersion = dep.Version
		if dep.Replace != nil {
			replaced := dep.Replace.Path
			if dep.Replace.Version != "" {
				replaced += "@" + dep.Replace.Version
			}
			id.ModuleVersion = dep.Version + "+replace=" + replaced
		}
		break
	}
	return id
}

// PinnedCompiler is quell-compiler-v1@<build-id>.
func (id Identity) PinnedCompiler() string {
	return pin(CompilerVersion, id)
}

// PinnedOptimizer is quell-optimizer-v1@<build-id>.
func (id Identity) PinnedOptimizer() string {
	return pin(OptimizerVersion, id)
}

func pin(schema string, id Identity) string {
	build := strings.TrimSpace(id.ModuleVersion)
	if build == "" || build == "unknown" {
		switch {
		case id.MainPath != "" && id.MainVersion != "":
			build = id.MainPath + "@" + id.MainVersion
		case id.GoVersion != "":
			build = id.GoVersion
		default:
			build = "unknown"
		}
	}
	out := schema + "@" + build
	if r := strings.TrimSpace(id.VCSRevision); r != "" {
		out += "+" + r
	}
	return out
}
