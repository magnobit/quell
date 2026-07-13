// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFileRelativeImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib/bell.quell", "H 0\nCNOT 0 1\n")
	main := writeFile(t, dir, "main.quell", "import \"./lib/bell.quell\"\nMEASURE\n")

	circ, err := ParseFile(main)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(circ.Instructions) != 3 {
		t.Fatalf("expected 3 spliced instructions (H, CNOT, MEASURE), got %d: %+v", len(circ.Instructions), circ.Instructions)
	}
	if circ.Instructions[0].Gate != "H" || circ.Instructions[1].Gate != "CNOT" || circ.Instructions[2].Gate != "MEASURE" {
		t.Errorf("unexpected instruction order: %+v", circ.Instructions)
	}
}

func TestParseFileSharesQubitNamespace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib/decl.quell", "qubit alice, bob\nH alice\n")
	main := writeFile(t, dir, "main.quell", "import \"./lib/decl.quell\"\nCNOT alice bob\nMEASURE\n")

	circ, err := ParseFile(main)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if circ.QubitNames["alice"] != 0 || circ.QubitNames["bob"] != 1 {
		t.Errorf("expected alice=0, bob=1 shared across the import, got %v", circ.QubitNames)
	}
	// CNOT alice bob in main.quell must resolve to the SAME indices the
	// imported file declared — proving the namespace is genuinely shared,
	// not re-declared/shadowed.
	last := circ.Instructions[len(circ.Instructions)-2] // CNOT, before MEASURE
	if last.Gate != "CNOT" || last.Qubits[0] != 0 || last.Qubits[1] != 1 {
		t.Errorf("expected CNOT on shared qubits 0,1, got %+v", last)
	}
}

func TestParseFileImportCycle(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.quell")
	pathB := filepath.Join(dir, "b.quell")
	writeFile(t, dir, "a.quell", "import \"./b.quell\"\nH 0\n")
	writeFile(t, dir, "b.quell", "import \"./a.quell\"\nX 0\n")

	_, err := ParseFile(pathA)
	if err == nil {
		t.Fatal("expected an import cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected error to mention 'cycle', got: %v", err)
	}
	_ = pathB
}

func TestParseFileDiamondImportIsNotACycle(t *testing.T) {
	// main imports both b.quell and c.quell, and both of those import the
	// same shared.quell — a diamond, not a cycle, and should splice
	// shared.quell's content twice (once via each path), not error.
	dir := t.TempDir()
	writeFile(t, dir, "shared.quell", "H 0\n")
	writeFile(t, dir, "b.quell", "import \"./shared.quell\"\n")
	writeFile(t, dir, "c.quell", "import \"./shared.quell\"\n")
	main := writeFile(t, dir, "main.quell", "import \"./b.quell\"\nimport \"./c.quell\"\nMEASURE\n")

	circ, err := ParseFile(main)
	if err != nil {
		t.Fatalf("diamond import should not error: %v", err)
	}
	hCount := 0
	for _, inst := range circ.Instructions {
		if inst.Gate == "H" {
			hCount++
		}
	}
	if hCount != 2 {
		t.Errorf("expected shared.quell's H to be spliced twice (once per diamond path), got %d", hCount)
	}
}

func TestParseFileSelfImportIsACycle(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "self.quell", "import \"./self.quell\"\nH 0\n")
	_, err := ParseFile(path)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected a cycle error for direct self-import, got: %v", err)
	}
}

func TestParseFilePackageImportWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "main.quell", "import \"github.com/someone/quell-gates/qft.quell\"\nMEASURE\n")
	_, err := ParseFile(main)
	if err == nil {
		t.Fatal("expected an error resolving a package import with no quell.pkg.yml")
	}
	if !strings.Contains(err.Error(), "quell.pkg.yml") {
		t.Errorf("expected error to mention quell.pkg.yml, got: %v", err)
	}
}

func TestParseFilePackageImportResolvesUnderProjectRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "quell.pkg.yml", "require: []\n")
	writeFile(t, dir, ".quell/pkg/github.com/someone/quell-gates/qft.quell", "H 0\n")
	// entry file lives in a subdirectory — project root must still be found
	// by walking up to quell.pkg.yml.
	main := writeFile(t, dir, "src/main.quell", "import \"github.com/someone/quell-gates/qft.quell\"\nMEASURE\n")

	circ, err := ParseFile(main)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(circ.Instructions) != 2 || circ.Instructions[0].Gate != "H" {
		t.Errorf("expected the package's H gate spliced in, got: %+v", circ.Instructions)
	}
}

func TestParseRejectsImportOnBareSource(t *testing.T) {
	_, err := Parse("import \"./x.quell\"\nH 0\n")
	if err == nil {
		t.Fatal("expected Parse to reject a bare import line")
	}
	if !strings.Contains(err.Error(), "ParseFile") {
		t.Errorf("expected error to point at ParseFile, got: %v", err)
	}
}
