// Copyright 2026 Magnobit, Inc. All rights reserved.

package format

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "uppercases gate keyword",
			in:   "h 0\ncnot 0 1\nmeasure\n",
			want: "H 0\nCNOT 0 1\nMEASURE\n",
		},
		{
			name: "qubit keyword stays lowercase, commas normalized",
			in:   "QUBIT alice,bob\nH alice\nCNOT alice bob\nMEASURE\n",
			want: "qubit alice, bob\nH alice\nCNOT alice bob\nMEASURE\n",
		},
		{
			name: "collapses extra inter-token whitespace",
			in:   "H    0\nCNOT   0   1\n",
			want: "H 0\nCNOT 0 1\n",
		},
		{
			name: "preserves angle expression exactly, only normalizes keyword",
			in:   "rx PI/2 0\nry 1.5708 0\n",
			want: "RX PI/2 0\nRY 1.5708 0\n",
		},
		{
			name: "collapses multiple blank lines to one",
			in:   "H 0\n\n\n\nCNOT 0 1\n",
			want: "H 0\n\nCNOT 0 1\n",
		},
		{
			name: "trims leading and trailing blank lines",
			in:   "\n\nH 0\nMEASURE\n\n\n",
			want: "H 0\nMEASURE\n",
		},
		{
			name: "header comment block preserved verbatim",
			in:   "// Bell pair\n// second line\n\nH 0\nCNOT 0 1\nMEASURE\n",
			want: "// Bell pair\n// second line\n\nH 0\nCNOT 0 1\nMEASURE\n",
		},
		{
			name: "aligns trailing comments within a contiguous block",
			in:   "H alice // put alice in superposition\nCNOT alice bob // entangle them\n",
			want: "H alice         // put alice in superposition\nCNOT alice bob  // entangle them\n",
		},
		{
			name: "comment alignment resets across a blank line (each line its own block, min 2-space gap)",
			in:   "H 0 // short\n\nCNOT 0 1 // this starts a new alignment group\n",
			want: "H 0  // short\n\nCNOT 0 1  // this starts a new alignment group\n",
		},
		{
			name: "no trailing comments in block needs no padding",
			in:   "H 0\nX 1\nMEASURE\n",
			want: "H 0\nX 1\nMEASURE\n",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "only comments and blanks",
			in:   "\n// just a comment\n\n",
			want: "// just a comment\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Format(tc.in)
			if got != tc.want {
				t.Errorf("Format(%q):\n got:  %q\n want: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatIdempotent(t *testing.T) {
	inputs := []string{
		"h 0\ncnot 0 1\nmeasure\n",
		"QUBIT alice,bob\nH alice   // hi\nCNOT alice bob\nMEASURE\n",
		"// header\n\nH 0\n\n\nX 1\nMEASURE\n",
		"rx PI/2 0\nry 2*PI 1\n",
	}
	for _, in := range inputs {
		once := Format(in)
		twice := Format(once)
		if once != twice {
			t.Errorf("Format is not idempotent for %q:\n once:  %q\n twice: %q", in, once, twice)
		}
	}
}

// Real example files under quell/examples/ should already be (close to)
// canonically formatted, and formatting must always be idempotent on them.
func TestFormatExamples(t *testing.T) {
	dir := "../examples"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("examples dir not found: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".quell" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			once := Format(string(data))
			twice := Format(once)
			if once != twice {
				t.Errorf("Format is not idempotent on %s:\n once:  %q\n twice: %q", e.Name(), once, twice)
			}
		})
	}
}
