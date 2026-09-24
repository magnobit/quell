package examples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/estimate"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
)

// Documented bind values for parameterized teaching examples.
var exampleParams = map[string]map[string]float64{
	"qaoa_maxcut.quell": {
		"gamma1": 0.5, "beta1": 0.3, "gamma2": 0.4, "beta2": 0.2,
	},
	"amplitude_estimation.quell": {
		"theta": 0.7,
	},
	"trotter_step.quell": {
		"theta": 0.3,
	},
}

func TestExamplesParseCompileSimulate(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".quell" {
			continue
		}
		n++
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			text := string(src)
			if _, err := parser.Parse(text); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if params, ok := exampleParams[name]; ok {
				bound, berr := estimate.BindSource(text, params)
				if berr != nil {
					t.Fatalf("bind: %v", berr)
				}
				text = bound
			}
			if _, err := compile.Compile(text, compile.OpenQASM); err != nil {
				t.Fatalf("compile openqasm: %v", err)
			}
			if strings.Contains(strings.ToLower(name), "anneal") || strings.HasSuffix(name, ".qubo") {
				return
			}
			if _, err := simulate.Run(text, 64); err != nil {
				t.Fatalf("simulate: %v", err)
			}
		})
	}
	if n == 0 {
		t.Fatal("no .quell examples found")
	}
}
