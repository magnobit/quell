package qasmimport_test

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/qasmimport"
)

func TestConvertWhileForSwitch(t *testing.T) {
	src := `OPENQASM 3.0;
qubit[3] q;
bit[3] c;
h q[0];
c[0] = measure q[0];
while (c[0] == 0) {
  x q[0];
  h q[0];
  c[0] = measure q[0];
}
for i in [0:3] {
  h q[i];
}
switch (c[0]) {
  0: x q[1];
  1: y q[1];
  default: z q[1];
}
`
	out, err := qasmimport.ToQuell(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"WHILE c[0]==0 MAX 32",
		"H 0", "H 1", "H 2", // for unrolled
		"SWITCH c[0]",
		"CASE 0:",
		"DEFAULT:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
