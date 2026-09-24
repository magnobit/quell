// Copyright 2026 Magnobit, Inc. All rights reserved.

package ir_test

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
)

func lower(t *testing.T, src string) *ir.Program {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ir.Lower(c)
}

func TestHash_SameCanonicalIRIsDeterministic(t *testing.T) {
	src := "PARAM theta\nH 0\nRX theta 0\nMEASURE"
	a := ir.Hash(lower(t, src))
	b := ir.Hash(lower(t, src))
	if a == "" || !strings.HasPrefix(a, "sha256:") {
		t.Fatalf("hash = %q, want sha256:<hex>", a)
	}
	if a != b {
		t.Fatalf("same IR hashed differently: %s vs %s", a, b)
	}
}

func TestHash_ParamDeclarationOrderDoesNotChangeHash(t *testing.T) {
	// Params are collected from a set in the parser; CanonicalBytes sorts
	// them so declaration/map-iteration order cannot change the digest.
	left := ir.Hash(lower(t, "PARAM theta\nPARAM phi\nRX theta 0\nRY phi 0\nMEASURE"))
	right := ir.Hash(lower(t, "PARAM phi\nPARAM theta\nRX theta 0\nRY phi 0\nMEASURE"))
	if left != right {
		t.Fatalf("param declaration order changed IR hash: %s vs %s", left, right)
	}
}

func TestHash_DifferentIRProducesDifferentHash(t *testing.T) {
	h := ir.Hash(lower(t, "H 0\nMEASURE"))
	x := ir.Hash(lower(t, "X 0\nMEASURE"))
	if h == x {
		t.Fatal("H and X circuits produced the same IR hash")
	}
}

func TestHash_BoundProgramDiffersFromSymbolic(t *testing.T) {
	prog := lower(t, "PARAM theta\nRX theta 0\nMEASURE")
	bound, err := ir.Bind(prog, map[string]float64{"theta": 1.5707963267948966})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if ir.Hash(prog) == ir.Hash(bound) {
		t.Fatal("symbolic and bound IR must not share a hash — the actual executed IR is the bound one")
	}
}

func TestCanonicalBytes_DoesNotLookLikeGoDump(t *testing.T) {
	got := string(ir.CanonicalBytes(lower(t, "H 0\nCNOT 0 1\nMEASURE")))
	if strings.Contains(got, "0x") || strings.Contains(got, "struct") {
		t.Fatalf("canonical bytes look like an unstable Go dump:\n%s", got)
	}
	if !strings.HasPrefix(got, ir.CanonicalVersion+"\n") {
		t.Fatalf("canonical bytes should start with %s, got %q", ir.CanonicalVersion, got[:min(40, len(got))])
	}
}
