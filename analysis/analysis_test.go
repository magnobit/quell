// Copyright 2026 Magnobit, Inc. All rights reserved.

package analysis

import (
	"math"
	"strings"
	"testing"
)

func mustLoad(t *testing.T, src string) *Model {
	t.Helper()
	m, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestStateBellAmplitudes(t *testing.T) {
	m := mustLoad(t, "H 0\nCNOT 0 1\nMEASURE\n")
	sv, err := m.State(nil)
	if err != nil {
		t.Fatal(err)
	}
	top := TopAmplitudes(sv, 0, 1e-12)
	if len(top) != 2 || top[0].Bits != "00" || top[1].Bits != "11" {
		t.Fatalf("%+v", top)
	}
	for _, a := range top {
		if !near(a.Prob, 0.5, 1e-12) || !near(a.Re, math.Sqrt2/2, 1e-12) {
			t.Fatalf("%+v", a)
		}
	}
}

func TestStateBitOrderQubitZeroRightmost(t *testing.T) {
	// X on qubit 0 of a 2-qubit register: counts use MSB-first with qubit 0
	// rightmost, so the state is "01".
	m := mustLoad(t, "X 0\nX 1\nX 1\nMEASURE\n")
	sv, err := m.State(nil)
	if err != nil {
		t.Fatal(err)
	}
	top := TopAmplitudes(sv, 0, 0)
	if len(top) != 1 || top[0].Bits != "01" {
		t.Fatalf("%+v", top)
	}
}

func TestExpectationAndGradient(t *testing.T) {
	src := "PARAM theta : angle\nobservable z = Z(0)\nRY theta 0\nMEASURE\n"
	m := mustLoad(t, src)
	// <Z> after RY(theta)|0> is cos(theta).
	for _, th := range []float64{0, 0.7, 2.1} {
		got, err := m.Expectation("z", map[string]float64{"theta": th})
		if err != nil {
			t.Fatal(err)
		}
		if !near(got, math.Cos(th), 1e-12) {
			t.Fatalf("theta=%v got %v want %v", th, got, math.Cos(th))
		}
	}
	g, err := m.Gradient("z", map[string]float64{"theta": 0.7}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !near(g["theta"], -math.Sin(0.7), 1e-7) {
		t.Fatalf("grad %v want %v", g["theta"], -math.Sin(0.7))
	}
	if _, err := m.Gradient("z", map[string]float64{"theta": 0.7}, []string{"nope"}); err == nil {
		t.Fatal("unknown wrt must fail")
	}
}

func TestExpectationUnknownObservableAndMissingParam(t *testing.T) {
	m := mustLoad(t, "PARAM theta : angle\nobservable z = Z(0)\nRY theta 0\nMEASURE\n")
	if _, err := m.Expectation("missing", map[string]float64{"theta": 1}); err == nil {
		t.Fatal("unknown observable must fail")
	}
	if _, err := m.Expectation("z", nil); err == nil {
		t.Fatal("unbound PARAM must fail")
	}
}

func TestMinimizeFindsGroundState(t *testing.T) {
	// min over theta of <Z> after RY(theta) is -1 at theta = pi.
	m := mustLoad(t, "PARAM theta : angle\nobservable z = Z(0)\nRY theta 0\nMEASURE\n")
	res, err := m.Minimize("z", nil, MinOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Converged || !near(res.Value, -1, 1e-6) {
		t.Fatalf("%+v", res)
	}
	if res.Backend != "local-statevector" || res.Strategy != "nelder-mead" {
		t.Fatalf("labels %+v", res)
	}
}

func TestMinimizeTwoParams(t *testing.T) {
	src := "PARAM a : angle\nPARAM b : angle\nobservable h = Z(0) + X(1)\nRY a 0\nRY b 1\nMEASURE\n"
	m := mustLoad(t, src)
	res, err := m.Minimize("h", nil, MinOptions{MaxIter: 800})
	if err != nil {
		t.Fatal(err)
	}
	if !near(res.Value, -2, 1e-5) {
		t.Fatalf("%+v", res)
	}
}

// The example ansatz has a closed form: E(theta) = 1.44 cos(theta) + 0.18 sin(theta).
func TestVQEAnsatzMatchesClosedForm(t *testing.T) {
	src := "PARAM theta : angle\n" +
		"observable h = -1.05 * Z(0) + 0.39 * Z(1) + 0.18 * X(0) * X(1)\n" +
		"X 0\nRY theta 1\nCNOT 1 0\nMEASURE\n"
	m := mustLoad(t, src)
	for _, th := range []float64{0, 0.7, 2.5} {
		got, err := m.Expectation("h", map[string]float64{"theta": th})
		if err != nil {
			t.Fatal(err)
		}
		want := 1.44*math.Cos(th) + 0.18*math.Sin(th)
		if !near(got, want, 1e-12) {
			t.Fatalf("theta=%v got %v want %v", th, got, want)
		}
	}
	g, err := m.Gradient("h", map[string]float64{"theta": 0.7}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := -1.44*math.Sin(0.7) + 0.18*math.Cos(0.7); !near(g["theta"], want, 1e-7) {
		t.Fatalf("grad %v want %v", g["theta"], want)
	}
	res, err := m.Minimize("h", nil, MinOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := -math.Sqrt(1.44*1.44 + 0.18*0.18); !near(res.Value, want, 1e-7) {
		t.Fatalf("min %v want %v", res.Value, want)
	}
}

func TestPrefixOnly(t *testing.T) {
	if mustLoad(t, "H 0\nCNOT 0 1\nMEASURE\n").PrefixOnly() {
		t.Fatal("trailing MEASURE is not truncation")
	}
	if !mustLoad(t, "H 0\nMEASURE 0\nX 0\nMEASURE\n").PrefixOnly() {
		t.Fatal("gate after MEASURE must be flagged")
	}
}

func TestMinimizeRequiresParam(t *testing.T) {
	m := mustLoad(t, "observable z = Z(0)\nH 0\nMEASURE\n")
	if _, err := m.Minimize("z", nil, MinOptions{}); err == nil {
		t.Fatal("no PARAM must fail")
	}
}

func TestDrawBell(t *testing.T) {
	m := mustLoad(t, "H 0\nCNOT 0 1\nMEASURE\n")
	got := Draw(m.Program())
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(lines[0], "[H]") || !strings.Contains(lines[0], "*") || !strings.Contains(lines[0], "[M]") {
		t.Fatalf("%q", lines[0])
	}
	if !strings.Contains(lines[2], "(+)") || !strings.Contains(lines[2], "[M]") {
		t.Fatalf("%q", lines[2])
	}
	if !strings.Contains(lines[1], "|") {
		t.Fatalf("no connector: %q", lines[1])
	}
	// The connector sits directly under the control and target.
	if strings.Index(lines[1], "|") != strings.Index(lines[0], "*") {
		t.Fatalf("connector not aligned:\n%s", got)
	}
}

func TestDrawCrossingWireAndParallelPacking(t *testing.T) {
	m := mustLoad(t, "H 0\nH 2\nCNOT 0 2\nMEASURE\n")
	got := Draw(m.Program())
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("%q", got)
	}
	// H 0 and H 2 share a column; q1 must show a crossing wire in the CNOT column.
	if !strings.Contains(lines[2], "|") {
		t.Fatalf("q1 should show crossing: %q", lines[2])
	}
	if strings.Index(lines[0], "[H]") != strings.Index(lines[4], "[H]") {
		t.Fatalf("H gates should pack into one column:\n%s", got)
	}
}

func TestDrawParamAndEmpty(t *testing.T) {
	m := mustLoad(t, "PARAM theta : angle\nRX theta 0\nMEASURE\n")
	if !strings.Contains(Draw(m.Program()), "RX(theta)") {
		t.Fatal(Draw(m.Program()))
	}
	if Draw(nil) != "" {
		t.Fatal("nil program must draw nothing")
	}
}

func TestCanonicalOmitsZeroBitFlip(t *testing.T) {
	plain := mustLoad(t, "H 0\nMEASURE\n").Canonical()
	zero := mustLoad(t, "NOISE bit_flip 0\nH 0\nMEASURE\n").Canonical()
	if plain != zero {
		t.Fatal("zero bit_flip must not change quell-ir-v1")
	}
	if strings.Contains(plain, "noise_bit_flip") {
		t.Fatal(plain)
	}
	flip := mustLoad(t, "NOISE bit_flip 0.2\nH 0\nMEASURE\n").Canonical()
	if !strings.Contains(flip, "noise_bit_flip=0.2") {
		t.Fatal(flip)
	}
}

func TestParseParams(t *testing.T) {
	got, err := ParseParams([]string{"theta=PI/2"})
	if err != nil {
		t.Fatal(err)
	}
	if !near(got["theta"], 1.5707963267948966, 1e-12) {
		t.Fatalf("%v", got)
	}
	if _, err := ParseParams([]string{"bad"}); err == nil {
		t.Fatal("malformed flag must fail")
	}
}

func TestQIRBell(t *testing.T) {
	text, err := mustLoad(t, "H 0\nCNOT 0 1\nMEASURE\n").QIR()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "__quantum__qis__h__body") {
		t.Fatal(text)
	}
}
