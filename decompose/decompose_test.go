package decompose

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
)

func TestRXXParses(t *testing.T) {
	src := strings.Join(FormatAnnotate(RXX("PI/2", 0, 1)), "\n") + "\nMEASURE\n"
	if _, err := parser.Parse(src); err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
}

func TestPhasedXPowParses(t *testing.T) {
	src := strings.Join(FormatAnnotate(PhasedXPow("0.5", "1", 0)), "\n") + "\nMEASURE\n"
	if _, err := parser.Parse(src); err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
}

func TestRZZParses(t *testing.T) {
	src := strings.Join(FormatAnnotate(RZZ("0.25", 0, 1)), "\n") + "\nMEASURE\n"
	if _, err := parser.Parse(src); err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
}
