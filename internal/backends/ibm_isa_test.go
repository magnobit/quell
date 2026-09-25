// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"strings"
	"testing"
)

type mat [][]complex128

func ident(n int) mat {
	m := make(mat, n)
	for i := range m {
		m[i] = make([]complex128, n)
		m[i][i] = 1
	}
	return m
}

func mul(a, b mat) mat {
	n := len(a)
	out := make(mat, n)
	for i := range out {
		out[i] = make([]complex128, n)
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				out[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return out
}

// embed places gate g on qubits qs of a 2-qubit register; bit k of g's
// index corresponds to qs[k].
func embed(g mat, qs []int) mat {
	out := make(mat, 4)
	for i := range out {
		out[i] = make([]complex128, 4)
	}
	for col := 0; col < 4; col++ {
		sub := 0
		for k, q := range qs {
			sub |= (col >> q & 1) << k
		}
		for r := 0; r < len(g); r++ {
			row := col
			for k, q := range qs {
				row = row&^(1<<q) | (r>>k&1)<<q
			}
			out[row][col] += g[r][sub]
		}
	}
	return out
}

func e(a float64) complex128 { return cmplx.Exp(complex(0, a)) }

func u3m(th, ph, la float64) mat {
	c, s := complex(math.Cos(th/2), 0), complex(math.Sin(th/2), 0)
	return mat{{c, -e(la) * s}, {e(ph) * s, e(ph+la) * c}}
}

func ctrl(u mat) mat { // control on bit 0, target bit 1
	m := ident(4)
	m[1][1], m[1][3], m[3][1], m[3][3] = u[0][0], u[0][1], u[1][0], u[1][1]
	return m
}

func referenceGate(name string, p []float64) mat {
	pi := math.Pi
	switch name {
	case "id":
		return ident(2)
	case "x":
		return mat{{0, 1}, {1, 0}}
	case "y":
		return mat{{0, -1i}, {1i, 0}}
	case "z":
		return mat{{1, 0}, {0, -1}}
	case "h":
		r := complex(1/math.Sqrt2, 0)
		return mat{{r, r}, {r, -r}}
	case "s":
		return mat{{1, 0}, {0, 1i}}
	case "sdg":
		return mat{{1, 0}, {0, -1i}}
	case "t":
		return mat{{1, 0}, {0, e(pi / 4)}}
	case "tdg":
		return mat{{1, 0}, {0, e(-pi / 4)}}
	case "sx":
		return mat{{(1 + 1i) / 2, (1 - 1i) / 2}, {(1 - 1i) / 2, (1 + 1i) / 2}}
	case "sxdg":
		return mat{{(1 - 1i) / 2, (1 + 1i) / 2}, {(1 + 1i) / 2, (1 - 1i) / 2}}
	case "rx":
		c, s := complex(math.Cos(p[0]/2), 0), complex(math.Sin(p[0]/2), 0)
		return mat{{c, -1i * s}, {-1i * s, c}}
	case "ry":
		c, s := complex(math.Cos(p[0]/2), 0), complex(math.Sin(p[0]/2), 0)
		return mat{{c, -s}, {s, c}}
	case "rz":
		return mat{{e(-p[0] / 2), 0}, {0, e(p[0] / 2)}}
	case "p", "phase", "u1":
		return mat{{1, 0}, {0, e(p[0])}}
	case "u2":
		return u3m(pi/2, p[0], p[1])
	case "u", "u3":
		return u3m(p[0], p[1], p[2])
	case "cx", "cnot":
		return ctrl(mat{{0, 1}, {1, 0}})
	case "cz":
		return ctrl(mat{{1, 0}, {0, -1}})
	case "cy":
		return ctrl(mat{{0, -1i}, {1i, 0}})
	case "cp", "cphase":
		return ctrl(mat{{1, 0}, {0, e(p[0])}})
	case "crz":
		return ctrl(mat{{e(-p[0] / 2), 0}, {0, e(p[0] / 2)}})
	case "swap":
		m := ident(4)
		m[1][1], m[2][2], m[1][2], m[2][1] = 0, 0, 1, 1
		return m
	case "rzz":
		a, b := e(-p[0]/2), e(p[0]/2)
		return mat{{a, 0, 0, 0}, {0, b, 0, 0}, {0, 0, b, 0}, {0, 0, 0, a}}
	}
	panic("no reference for " + name)
}

func isaUnitary(ops []isaOp) mat {
	u := ident(4)
	for _, op := range ops {
		var g mat
		switch op.name {
		case "rz":
			g = referenceGate("rz", []float64{op.angle})
		case "sx", "x":
			g = referenceGate(op.name, nil)
		case "cz":
			g = referenceGate("cz", nil)
		default:
			panic("non-ISA op " + op.name)
		}
		u = mul(embed(g, op.q), u)
	}
	return u
}

func equalUpToPhase(a, b mat) bool {
	var phase complex128
	for i := range a {
		for j := range a[i] {
			if cmplx.Abs(b[i][j]) > 0.1 {
				phase = a[i][j] / b[i][j]
				break
			}
		}
		if phase != 0 {
			break
		}
	}
	if math.Abs(cmplx.Abs(phase)-1) > 1e-9 {
		return false
	}
	for i := range a {
		for j := range a[i] {
			if cmplx.Abs(a[i][j]-phase*b[i][j]) > 1e-9 {
				return false
			}
		}
	}
	return true
}

func TestDecomposeForIBM_MatchesReferenceUnitaries(t *testing.T) {
	angles := [][]float64{{0.37, -1.2, 2.9}, {math.Pi, math.Pi / 2, -math.Pi / 3}}
	for name, ar := range ibmGateArity {
		for _, a := range angles {
			p := a[:ar[1]]
			ops, err := decomposeForIBM(name, p, ar[0])
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			qs := []int{0}
			if ar[0] == 2 {
				qs = []int{0, 1}
			}
			want := embed(referenceGate(name, p), qs)
			if got := isaUnitary(ops); !equalUpToPhase(got, want) {
				t.Errorf("%s%v: ISA decomposition is not equivalent", name, p)
			}
		}
	}
}

func TestToIBMISA_TranslatesCompilerOutput(t *testing.T) {
	src := "OPENQASM 3;\nqubit[2] q;\nbit[2] c;\n\nh q[0];\ncx q[0], q[1];\nry(pi/4) q[1];\nc = measure q;\n"
	got, err := toIBMISA(src, defaultIBMTarget)
	if err != nil {
		t.Fatalf("toIBMISA: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if lines[0] != "OPENQASM 3;" || lines[1] != `include "stdgates.inc";` {
		t.Errorf("header = %q", lines[:2])
	}
	for _, l := range lines[2:] {
		name := strings.FieldsFunc(l, func(r rune) bool { return r == ' ' || r == '(' || r == '[' })[0]
		switch name {
		case "rz", "sx", "x", "cz", "qubit", "bit", "c":
		default:
			t.Errorf("non-ISA line %q", l)
		}
	}
	if !strings.Contains(got, "cz q[0], q[1];") || !strings.HasSuffix(got, "c = measure q;\n") {
		t.Errorf("translation:\n%s", got)
	}

	live := "OPENQASM 3.0;\ninclude \"stdgates.inc\";\nqubit[1] q;\nbit[1] c;\nh q[0];\nc[0] = measure q[0];\n"
	got, err = toIBMISA(live, defaultIBMTarget)
	if err != nil {
		t.Fatalf("live validation circuit: %v", err)
	}
	if strings.Count(got, "stdgates.inc") != 1 || strings.Contains(got, "h q[0]") || !strings.Contains(got, "c[0] = measure q[0];") {
		t.Errorf("live validation circuit translation:\n%s", got)
	}
}

func TestToIBMISA_RejectsWhatItCannotTranslate(t *testing.T) {
	coupled := ibmTarget{NumQubits: 3, Basis: defaultIBMTarget.Basis, Coupling: map[[2]int]bool{{0, 1}: true, {1, 2}: true}}
	cases := map[string]string{
		"uncoupled":    "OPENQASM 3;\nqubit[3] q;\ncx q[0], q[2];\n",
		"too wide":     "OPENQASM 3;\nqubit[4] q;\nx q[3];\n",
		"custom gate":  "OPENQASM 3;\nqubit[1] q;\nfoo q[0];\n",
		"three-qubit":  "OPENQASM 3;\nqubit[3] q;\nccx q[0], q[1], q[2];\n",
		"control flow": "OPENQASM 3;\nqubit[1] q;\nbit[1] c;\nif (c[0]) {\n",
		"broadcast":    "OPENQASM 3;\nqubit[2] q;\nh q;\n",
	}
	for name, src := range cases {
		if _, err := toIBMISA(src, coupled); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	ecrOnly := ibmTarget{Basis: map[string]bool{"ecr": true, "rz": true, "sx": true, "x": true}}
	if _, err := toIBMISA("OPENQASM 3;\nqubit[1] q;\nx q[0];\n", ecrOnly); err == nil {
		t.Error("expected an error for an ECR-only basis")
	}
	if _, err := toIBMISA("OPENQASM 3;\nqubit[3] q;\ncx q[2], q[1];\n", coupled); err != nil {
		t.Errorf("reverse-direction coupled pair rejected: %v", err)
	}
}

func TestEvalAngle(t *testing.T) {
	for in, want := range map[string]float64{"0.5": 0.5, "pi/2": math.Pi / 2, "-pi": -math.Pi, "2*pi/3": 2 * math.Pi / 3, "(pi+1)/2": (math.Pi + 1) / 2, "1e-3": 1e-3, "π": math.Pi} {
		got, err := evalAngle(in)
		if err != nil || math.Abs(got-want) > 1e-12 {
			t.Errorf("evalAngle(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"theta", "1/0", "(1", ""} {
		if _, err := evalAngle(bad); err == nil {
			t.Errorf("evalAngle(%q) should fail", bad)
		}
	}
}

// encodeNPYBitArray builds a RuntimeEncoder BitArray payload the way
// qiskit-ibm-runtime does: np.save, then zlib, then base64.
func encodeNPYBitArray(t *testing.T, rows [][]byte, numBits int) string {
	t.Helper()
	header := fmt.Sprintf("{'descr': '|u1', 'fortran_order': False, 'shape': (%d, %d), }", len(rows), len(rows[0]))
	for (10+len(header)+1)%64 != 0 {
		header += " "
	}
	header += "\n"
	var npy bytes.Buffer
	npy.WriteString("\x93NUMPY\x01\x00")
	binary.Write(&npy, binary.LittleEndian, uint16(len(header)))
	npy.WriteString(header)
	for _, r := range rows {
		npy.Write(r)
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(npy.Bytes())
	zw.Close()
	return fmt.Sprintf(`{"__type__": "BitArray", "__value__": {"array": {"__type__": "ndarray", "__value__": %q}, "num_bits": %d}}`,
		base64.StdEncoding.EncodeToString(z.Bytes()), numBits)
}

func samplerV2Result(t *testing.T, reg string, rows [][]byte, numBits int) string {
	return fmt.Sprintf(`{"__type__": "PrimitiveResult", "__value__": {"pub_results": [{"__type__": "SamplerPubResult", "__value__": {"data": {"__type__": "DataBin", "__value__": {"field_names": [%q], "shape": [], "fields": {%q: %s}}}, "metadata": {}}}], "metadata": {"version": 2}}}`,
		reg, reg, encodeNPYBitArray(t, rows, numBits))
}

func TestParseSamplerV2Result_DecodesRuntimeEncodedBitArray(t *testing.T) {
	// 10 bits span two bytes; clbit 0 is the lowest bit of the last byte.
	rows := [][]byte{{0x00, 0x01}, {0x02, 0x00}, {0x00, 0x01}}
	counts, ok, err := parseSamplerV2Result([]byte(samplerV2Result(t, "c", rows, 10)))
	if !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if counts["0000000001"] != 2 || counts["1000000000"] != 1 || len(counts) != 2 {
		t.Errorf("counts = %v", counts)
	}
	if _, ok, _ := parseSamplerV2Result([]byte(`{"results": []}`)); ok {
		t.Error("non-envelope input should report ok=false")
	}
}
