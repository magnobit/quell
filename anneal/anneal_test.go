package anneal_test

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/anneal"
)

func TestParseQUBO(t *testing.T) {
	src := `
# max-cut style toy
n 2
h 0 -1
h 1 -1
q 0 1 2
`
	p, err := anneal.ParseQUBO(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.NumVars != 2 {
		t.Fatalf("NumVars=%d", p.NumVars)
	}
	if p.Linear[0] != -1 || p.Quadratic[[2]int{0, 1}] != 2 {
		t.Fatalf("unexpected terms: %+v %+v", p.Linear, p.Quadratic)
	}
}

func TestParseQUBORejectsGarbage(t *testing.T) {
	if _, err := anneal.ParseQUBO("foo 1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseQUBORejectsGateQuell(t *testing.T) {
	_, err := anneal.ParseQUBO("H 0\nCNOT 0 1\nMEASURE\n")
	if err == nil {
		t.Fatal("expected error for gate Quell")
	}
	if !strings.Contains(err.Error(), "gate Quell") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseQUBOSlashComments(t *testing.T) {
	src := `// Quell-style comment (Cloud users often paste these)
# hash comment
n 2
h 0 -1 // bias on x0
h 1 -1
q 0 1 2 # coupling
`
	p, err := anneal.ParseQUBO(src)
	if err != nil {
		t.Fatal(err)
	}
	if p.NumVars != 2 || p.Linear[0] != -1 || p.Quadratic[[2]int{0, 1}] != 2 {
		t.Fatalf("unexpected: %+v", p)
	}
}
