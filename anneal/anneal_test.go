package anneal_test

import (
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
