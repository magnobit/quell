package qerr_test

import (
	"errors"
	"testing"

	"github.com/magnobit/quell/qerr"
)

func TestIsKind(t *testing.T) {
	err := qerr.Parse("FOR", 3, "bad range")
	if !qerr.IsKind(err, qerr.KindParse) {
		t.Fatal("expected parse kind")
	}
	wrapped := qerr.Compile("compile", err)
	if !qerr.IsKind(wrapped, qerr.KindParse) {
		// Compile wraps only non-*Error; Parse returns *Error so identity preserved
		t.Fatal("expected inner parse kind preserved")
	}
	plain := qerr.Compile("compile", errors.New("boom"))
	if !qerr.IsKind(plain, qerr.KindCompile) {
		t.Fatal("expected compile kind")
	}
}
