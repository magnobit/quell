package anneal_test

import (
	"testing"

	"github.com/magnobit/quell/anneal"
)

func TestSampleLocal(t *testing.T) {
	p, err := anneal.ParseQUBO("n 2\nh 0 -1\nh 1 -1\nq 0 1 2\n")
	if err != nil {
		t.Fatal(err)
	}
	res, err := anneal.SampleLocal(p, 50, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Samples) == 0 {
		t.Fatal("expected samples")
	}
	counts := res.CountsMap()
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != 50 {
		t.Fatalf("total counts %d want 50", total)
	}
}
