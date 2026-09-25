// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// runtimeValue is qiskit-ibm-runtime's RuntimeEncoder envelope.
type runtimeValue struct {
	Type  string          `json:"__type__"`
	Value json.RawMessage `json:"__value__"`
}

// parseSamplerV2Result decodes the first pub of a Sampler V2 PrimitiveResult
// as returned by GET /api/v1/jobs/{id}/results. Bitstrings follow Qiskit's
// convention: the highest classical bit is leftmost. ok is false when raw is
// not a PrimitiveResult envelope.
func parseSamplerV2Result(raw []byte) (counts map[string]int, ok bool, err error) {
	var env runtimeValue
	if json.Unmarshal(raw, &env) != nil || env.Type != "PrimitiveResult" {
		return nil, false, nil
	}
	var prim struct {
		PubResults []runtimeValue `json:"pub_results"`
	}
	if err := json.Unmarshal(env.Value, &prim); err != nil || len(prim.PubResults) == 0 {
		return nil, true, fmt.Errorf("PrimitiveResult has no pub results")
	}
	var pub struct {
		Data runtimeValue `json:"data"`
	}
	if err := json.Unmarshal(prim.PubResults[0].Value, &pub); err != nil || pub.Data.Type != "DataBin" {
		return nil, true, fmt.Errorf("pub result has no DataBin")
	}
	var bin struct {
		FieldNames []string                `json:"field_names"`
		Fields     map[string]runtimeValue `json:"fields"`
	}
	if err := json.Unmarshal(pub.Data.Value, &bin); err != nil {
		return nil, true, fmt.Errorf("decode DataBin: %w", err)
	}
	names := bin.FieldNames
	if len(names) == 0 {
		for k := range bin.Fields {
			names = append(names, k)
		}
	}
	var regs []map[string]int
	for _, name := range names {
		f, present := bin.Fields[name]
		if !present || f.Type != "BitArray" {
			continue
		}
		c, err := decodeRuntimeBitArray(f.Value)
		if err != nil {
			return nil, true, fmt.Errorf("register %s: %w", name, err)
		}
		regs = append(regs, c)
	}
	if len(regs) == 0 {
		return nil, true, fmt.Errorf("DataBin has no BitArray registers")
	}
	if len(regs) == 1 {
		return regs[0], true, nil
	}
	return nil, true, fmt.Errorf("multiple classical registers (%d) are not supported; measure into one register", len(regs))
}

func decodeRuntimeBitArray(raw json.RawMessage) (map[string]int, error) {
	var ba struct {
		Array   runtimeValue `json:"array"`
		NumBits int          `json:"num_bits"`
	}
	if err := json.Unmarshal(raw, &ba); err != nil {
		return nil, err
	}
	if ba.Array.Type != "ndarray" {
		return nil, fmt.Errorf("array is %q, want ndarray", ba.Array.Type)
	}
	var b64 string
	if err := json.Unmarshal(ba.Array.Value, &b64); err != nil {
		return nil, fmt.Errorf("ndarray value is not an encoded string")
	}
	shape, data, err := decodeNPY(b64)
	if err != nil {
		return nil, err
	}
	if len(shape) != 2 {
		return nil, fmt.Errorf("BitArray shape %v, want (shots, bytes)", shape)
	}
	shots, nbytes := shape[0], shape[1]
	if len(data) != shots*nbytes {
		return nil, fmt.Errorf("BitArray has %d bytes, want %d", len(data), shots*nbytes)
	}
	width := ba.NumBits
	if width <= 0 || width > nbytes*8 {
		return nil, fmt.Errorf("num_bits %d out of range", ba.NumBits)
	}
	counts := make(map[string]int)
	bits := make([]byte, width)
	for s := 0; s < shots; s++ {
		row := data[s*nbytes : (s+1)*nbytes]
		// Rows are big-endian: clbit i is bit i of the row read as one integer.
		for i := 0; i < width; i++ {
			b := row[nbytes-1-i/8] >> uint(i%8) & 1
			bits[width-1-i] = '0' + b
		}
		counts[string(bits)]++
	}
	return counts, nil
}

var npyShapeRe = regexp.MustCompile(`'shape':\s*\(([^)]*)\)`)

// decodeNPY reads a zlib-compressed, base64 NumPy .npy payload holding a
// C-ordered uint8 array.
func decodeNPY(b64 string) ([]int, []byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, nil, fmt.Errorf("base64: %w", err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, nil, fmt.Errorf("zlib: %w", err)
	}
	npy, err := io.ReadAll(zr)
	if err != nil {
		return nil, nil, fmt.Errorf("zlib: %w", err)
	}
	if len(npy) < 10 || string(npy[:6]) != "\x93NUMPY" {
		return nil, nil, fmt.Errorf("not an .npy payload")
	}
	var headerLen, start int
	switch npy[6] {
	case 1:
		headerLen, start = int(binary.LittleEndian.Uint16(npy[8:10])), 10
	case 2, 3:
		if len(npy) < 12 {
			return nil, nil, fmt.Errorf("truncated .npy header")
		}
		headerLen, start = int(binary.LittleEndian.Uint32(npy[8:12])), 12
	default:
		return nil, nil, fmt.Errorf(".npy version %d unsupported", npy[6])
	}
	if len(npy) < start+headerLen {
		return nil, nil, fmt.Errorf("truncated .npy header")
	}
	header := string(npy[start : start+headerLen])
	if !strings.Contains(header, "'|u1'") && !strings.Contains(header, "'u1'") {
		return nil, nil, fmt.Errorf(".npy dtype is not uint8: %s", strings.TrimSpace(header))
	}
	if strings.Contains(header, "'fortran_order': True") {
		return nil, nil, fmt.Errorf(".npy fortran order unsupported")
	}
	m := npyShapeRe.FindStringSubmatch(header)
	if m == nil {
		return nil, nil, fmt.Errorf(".npy header has no shape")
	}
	var shape []int
	for _, part := range strings.Split(m[1], ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, nil, fmt.Errorf(".npy shape %q", m[1])
		}
		shape = append(shape, n)
	}
	return shape, npy[start+headerLen:], nil
}
