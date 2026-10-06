package qerr_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magnobit/quell/qerr"
)

func TestDiagnosticJSON(t *testing.T) {
	d := qerr.Diagnostic{
		Code: qerr.CodeUnknownIdent, Severity: qerr.SeverityError,
		Message: "unknown identifier \"x\"", Line: 2, Column: 9, EndLine: 2, EndColumn: 10,
		SuggestedFix: "Declare it with let.", DocsURL: qerr.DocsHostValues,
	}
	raw, err := d.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"code":"QL2001"`, `"line":2`, `"column":9`, `"suggestedFix"`, `"docsURL"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
	err = qerr.New(qerr.KindCheck, d)
	got, ok := qerr.AsDiagnostic(err)
	if !ok || got.Code != qerr.CodeUnknownIdent || !strings.Contains(err.Error(), "QL2001") {
		t.Fatalf("%v %v", err, got)
	}
}

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
