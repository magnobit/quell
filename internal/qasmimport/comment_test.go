package qasmimport_test

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/qasmimport"
)

func TestConvertIgnoresCommentGates(t *testing.T) {
	src := `OPENQASM 3.0;
qubit[2] q;
// h q[1];  should not import
/* cx q[0], q[1]; */
h q[0];
cx q[0], q[1];
`
	out, err := qasmimport.ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "H 1") {
		t.Fatalf("imported commented H 1:\n%s", out)
	}
	if !strings.Contains(out, "H 0") || !strings.Contains(out, "CNOT 0 1") {
		t.Fatalf("missing live gates:\n%s", out)
	}
}
