// Copyright 2026 Magnobit, Inc. All rights reserved.

package parser

import (
	"strings"
	"testing"
)

func TestParseHostIfStmt(t *testing.T) {
	src := strings.Join([]string{
		"fn abs(x: int) -> int {",
		"    if x < 0 {",
		"        return -x",
		"    } else {",
		"        return x",
		"    }",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	st := c.Functions[0].Body[0]
	if st.Kind != "if" || st.If == nil || st.If.Else == nil {
		t.Fatalf("%#v", st)
	}
	if len(c.Instructions) != 2 {
		t.Fatalf("gates changed: %+v", c.Instructions)
	}
	want := strings.Join([]string{
		"fn abs(x: int) -> int {",
		"    if x < 0 {",
		"        return -x",
		"    } else {",
		"        return x",
		"    }",
		"}",
	}, "\n")
	if got := c.Functions[0].Canonical(); got != want {
		t.Fatalf("canonical:\n%s", got)
	}
}

func TestParseElseIfIsNestedIf(t *testing.T) {
	src := strings.Join([]string{
		"fn sign(x: int) -> int {",
		"    if x < 0 {",
		"        return -1",
		"    } else if x == 0 {",
		"        return 0",
		"    } else {",
		"        return 1",
		"    }",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	ifs := c.Functions[0].Body[0].If
	if ifs == nil || len(ifs.Else) != 1 || ifs.Else[0].Kind != "if" || ifs.Else[0].If == nil {
		t.Fatalf("else if did not become a nested if: %+v", ifs)
	}
	want := strings.Join([]string{
		"fn sign(x: int) -> int {",
		"    if x < 0 {",
		"        return -1",
		"    } else {",
		"        if x == 0 {",
		"            return 0",
		"        } else {",
		"            return 1",
		"        }",
		"    }",
		"}",
	}, "\n")
	if got := c.Functions[0].Canonical(); got != want {
		t.Fatalf("canonical:\n%s", got)
	}
}

func TestHostIfKindAt(t *testing.T) {
	src := strings.Join([]string{
		"fn abs(x: int) -> int {",
		"    if x < 0 {",
		"        return -x",
		"    } else {",
		"        return if x == 0 { 0 } else { x }",
		"    }",
		"}",
		"H 0",
		"MEASURE",
		"",
	}, "\n")
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if HostIfKindAt(c, 2) != "stmt" {
		t.Fatalf("stmt kind: %q", HostIfKindAt(c, 2))
	}
	if HostIfKindAt(c, 5) != "expr" {
		t.Fatalf("expr kind: %q", HostIfKindAt(c, 5))
	}
}
