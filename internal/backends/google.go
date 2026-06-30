// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package backends

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/magnobit/quell/internal/config"
)

const (
	googleTokenURL  = "https://oauth2.googleapis.com/token"
	googleQuantumBase = "https://quantumai.googleapis.com/v1alpha1"
	googleScope     = "https://www.googleapis.com/auth/cloud-platform"
)

// RunGoogle submits a circuit to the Google Quantum Computing Service
// (Quantum Engine). Requires a service account key file set via
// google.key_file in quell.config.yml.
func RunGoogle(cfg *config.GCPConfig, qasm3 string) (*RunResult, error) {
	if cfg.Project == "" {
		return nil, fmt.Errorf("google: project is required (google.project in config)")
	}
	if cfg.Processor == "" {
		return nil, fmt.Errorf("google: processor is required (e.g. rainbow, weber)")
	}
	if cfg.KeyFile == "" {
		return nil, fmt.Errorf("google: key_file path is required (service account JSON, google.key_file in config)")
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1000
	}

	// Load service account and get OAuth2 bearer token
	token, email, err := googleAccessToken(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("google: auth: %w", err)
	}
	fmt.Printf("  Google auth OK (service account: %s)\n", email)

	// Upload program (circuit definition)
	programName, err := googleCreateProgram(token, cfg.Project, qasm3)
	if err != nil {
		return nil, fmt.Errorf("google: create program: %w", err)
	}

	// Create and run a job on the program
	jobName, err := googleCreateJob(token, cfg.Project, programName, cfg.Processor, shots)
	if err != nil {
		return nil, fmt.Errorf("google: create job: %w", err)
	}
	fmt.Printf("  Google Quantum job created: %s\n", jobName)

	// Poll until done
	resultRaw, err := googlePoll(token, jobName)
	if err != nil {
		return nil, fmt.Errorf("google: poll: %w", err)
	}

	// Parse measurement results
	counts, err := googleParseResult(resultRaw, shots)
	if err != nil {
		return nil, fmt.Errorf("google: parse results: %w", err)
	}

	return &RunResult{
		JobID:   jobName,
		Backend: "Google Quantum / " + cfg.Processor,
		Shots:   shots,
		Counts:  counts,
	}, nil
}

// --- service account auth ---

type serviceAccountKey struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func googleAccessToken(keyFile string) (token, email string, err error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("read key file %s: %w", keyFile, err)
	}

	var sa serviceAccountKey
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", "", fmt.Errorf("parse service account JSON: %w", err)
	}
	if sa.Type != "service_account" {
		return "", "", fmt.Errorf("key file type is %q, expected service_account", sa.Type)
	}

	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = googleTokenURL
	}

	jwt, err := makeServiceAccountJWT(sa.ClientEmail, tokenURI, googleScope, sa.PrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("create JWT: %w", err)
	}

	// Exchange JWT for access token
	payload := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + jwt
	resp, err := http.Post(tokenURI, "application/x-www-form-urlencoded", strings.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	json.NewDecoder(resp.Body).Decode(&tok)
	if tok.Error != "" {
		return "", "", fmt.Errorf("%s: %s", tok.Error, tok.ErrorDesc)
	}
	return tok.AccessToken, sa.ClientEmail, nil
}

func makeServiceAccountJWT(email, audience, scope, privateKeyPEM string) (string, error) {
	// Parse RSA private key (PKCS#8, as Google service accounts use)
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("no PEM block found in private key")
	}

	var rsaKey *rsa.PrivateKey
	// Try PKCS#8 first (standard for Google service accounts)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Fall back to PKCS#1
		rsaKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("parse private key: %w", err)
		}
	} else {
		var ok bool
		rsaKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("private key is not RSA")
		}
	}

	now := time.Now().Unix()
	header := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}))
	claims := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]any{
		"iss":   email,
		"sub":   email,
		"aud":   audience,
		"scope": scope,
		"iat":   now,
		"exp":   now + 3600,
	}))

	unsigned := header + "." + claims

	h := crypto.SHA256.New()
	h.Write([]byte(unsigned))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest)
	if err != nil {
		return "", err
	}

	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// --- Google Quantum Engine API ---

func googleCreateProgram(token, project, qasm3 string) (string, error) {
	// POST /projects/{project}/programs
	url := fmt.Sprintf("%s/projects/%s/programs", googleQuantumBase, project)

	body, _ := json.Marshal(map[string]any{
		"code": map[string]any{
			"@type": "type.googleapis.com/cirq.google.api.v2.Program",
			"languageSpec": map[string]string{
				"gateset": "OPENQASM3",
			},
			"code": qasm3,
		},
	})

	resp, err := googleDo("POST", url, token, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var r struct {
		Name string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	if r.Name == "" {
		return "", fmt.Errorf("no program name in response")
	}
	return r.Name, nil
}

func googleCreateJob(token, project, programName, processor string, shots int) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/programs/%s/jobs",
		googleQuantumBase, project, lastSegment(programName))

	body, _ := json.Marshal(map[string]any{
		"processorName": fmt.Sprintf("projects/%s/processors/%s", project, processor),
		"runContext": map[string]any{
			"@type": "type.googleapis.com/cirq.google.api.v2.RunContext",
			"parameter_sweeps": []map[string]any{
				{
					"repetitions": shots,
					"sweep": map[string]any{
						"@type": "type.googleapis.com/cirq.google.api.v2.Sweep",
					},
				},
			},
		},
	})

	resp, err := googleDo("POST", url, token, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var r struct {
		Name string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	if r.Name == "" {
		return "", fmt.Errorf("no job name in response")
	}
	return r.Name, nil
}

func googlePoll(token, jobName string) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s", googleQuantumBase, jobName)

	for {
		resp, err := googleDo("GET", url, token, nil)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var r struct {
			ExecutionStatus struct {
				State string `json:"state"`
			} `json:"executionStatus"`
			Result json.RawMessage `json:"result"`
			Failure struct {
				Error string `json:"error"`
			} `json:"failure"`
		}
		json.Unmarshal(raw, &r)

		switch r.ExecutionStatus.State {
		case "SUCCESS":
			fmt.Print("\n")
			return r.Result, nil
		case "FAILURE", "CANCELLED":
			return nil, fmt.Errorf("job %s: %s", jobName, r.Failure.Error)
		default:
			fmt.Printf("\r  Google Quantum job: %-12s", r.ExecutionStatus.State)
			time.Sleep(5 * time.Second)
		}
	}
}

func googleParseResult(raw json.RawMessage, shots int) (map[string]int, error) {
	// Google Quantum Engine result format:
	// {"@type": "...", "sweepResults": [{"parameterizedResults": [{"measurementResults": [
	//   {"key": "c", "qubitMeasurementResults": [
	//     {"qubitId": {"id": "q<0>"}, "results": "<base64 packed bits>"}
	//   ]}
	// ]}]}]}
	var result struct {
		SweepResults []struct {
			ParameterizedResults []struct {
				MeasurementResults []struct {
					Key                    string `json:"key"`
					QubitMeasurementResults []struct {
						Results string `json:"results"` // base64 packed bits, 1 bit/shot
					} `json:"qubitMeasurementResults"`
				} `json:"measurementResults"`
				NumRepetitions int `json:"numRepetitions"`
			} `json:"parameterizedResults"`
		} `json:"sweepResults"`
	}

	if err := json.Unmarshal(raw, &result); err != nil || len(result.SweepResults) == 0 {
		// Unknown format — return raw string in debug key
		return map[string]int{"parse_error": 0}, fmt.Errorf("unrecognised Google result: %s", string(raw))
	}

	paramResult := result.SweepResults[0].ParameterizedResults[0]
	numReps := paramResult.NumRepetitions
	if numReps == 0 {
		numReps = shots
	}

	// Collect per-qubit bit arrays; then assemble per-shot strings
	var allQubitBits [][]byte
	for _, mr := range paramResult.MeasurementResults {
		for _, qr := range mr.QubitMeasurementResults {
			bits, err := googleDecodeBits(qr.Results, numReps)
			if err != nil {
				return nil, fmt.Errorf("decode qubit bits: %w", err)
			}
			allQubitBits = append(allQubitBits, bits)
		}
	}

	if len(allQubitBits) == 0 {
		return nil, fmt.Errorf("no qubit measurement results in response")
	}

	counts := make(map[string]int)
	numQubits := len(allQubitBits)
	bits := make([]byte, numQubits)
	for shot := 0; shot < numReps; shot++ {
		for q := 0; q < numQubits; q++ {
			bits[q] = allQubitBits[q][shot]
		}
		counts[string(bits)]++
	}
	return counts, nil
}

// googleDecodeBits decodes Google's base64-encoded packed bit string.
// Each byte holds 8 measurement results, MSB first.
func googleDecodeBits(b64 string, numShots int) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(b64)
		if err != nil {
			return nil, err
		}
	}

	bits := make([]byte, numShots)
	for i := 0; i < numShots; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if byteIdx < len(data) && data[byteIdx]&(1<<uint(bitIdx)) != 0 {
			bits[i] = '1'
		} else {
			bits[i] = '0'
		}
	}
	return bits, nil
}

func googleDo(method, url, token string, body []byte) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from Google: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}

func lastSegment(name string) string {
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}
