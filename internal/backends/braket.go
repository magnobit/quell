// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package backends

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/magnobit/quell/internal/config"
)

// RunBraket submits a circuit to AWS Braket and returns measurement counts.
// AWS credentials are read from cfg or from the standard AWS env vars.
func RunBraket(cfg *config.AWSConfig, qasm3 string) (*RunResult, error) {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("braket: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY env vars are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Device == "" {
		cfg.Device = "arn:aws:braket:::device/quantum-simulator/amazon/sv1"
	}
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("braket: s3_bucket is required in quell.config.yml (aws.s3_bucket)")
	}
	if cfg.S3Prefix == "" {
		cfg.S3Prefix = "quell-results"
	}
	shots := cfg.Shots
	if shots == 0 {
		shots = 1000
	}

	creds := awsCreds{accessKey, secretKey, sessionToken}
	taskArn, s3Bucket, s3Dir, err := braketSubmit(cfg, creds, qasm3, shots)
	if err != nil {
		return nil, fmt.Errorf("braket: submit: %w", err)
	}
	fmt.Printf("  Braket task submitted: %s\n", taskArn)

	if err := braketPoll(cfg, creds, taskArn); err != nil {
		return nil, fmt.Errorf("braket: %w", err)
	}

	counts, err := braketResults(cfg.Region, creds, s3Bucket, s3Dir)
	if err != nil {
		return nil, fmt.Errorf("braket: results: %w", err)
	}

	return &RunResult{
		JobID:   taskArn,
		Backend: "AWS Braket / " + cfg.Device,
		Shots:   shots,
		Counts:  counts,
	}, nil
}

type awsCreds struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

func braketSubmit(cfg *config.AWSConfig, creds awsCreds, qasm3 string, shots int) (taskArn, s3Bucket, s3Dir string, err error) {
	endpoint := fmt.Sprintf("https://braket.%s.amazonaws.com", cfg.Region)

	// The action field must be a JSON-encoded string (doubly serialised)
	action := map[string]any{
		"braketSchemaHeader": map[string]string{
			"name":    "braket.ir.openqasm.program",
			"version": "1",
		},
		"source": qasm3,
		"inputs": map[string]any{},
	}
	actionJSON, _ := json.Marshal(action)

	payload := map[string]any{
		"deviceArn":         cfg.Device,
		"shots":             shots,
		"outputS3Bucket":    cfg.S3Bucket,
		"outputS3KeyPrefix": cfg.S3Prefix,
		"action":            string(actionJSON),
	}
	body, _ := json.Marshal(payload)

	resp, err := awsDo("POST", endpoint+"/quantum-task", cfg.Region, "braket", creds, body)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	var r struct {
		QuantumTaskArn  string `json:"quantumTaskArn"`
		OutputS3Bucket  string `json:"outputS3Bucket"`
		OutputS3Directory string `json:"outputS3Directory"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", "", err
	}
	if r.QuantumTaskArn == "" {
		return "", "", "", fmt.Errorf("no task ARN in response")
	}
	return r.QuantumTaskArn, r.OutputS3Bucket, r.OutputS3Directory, nil
}

func braketPoll(cfg *config.AWSConfig, creds awsCreds, taskArn string) error {
	// URL-encode the ARN for the path
	endpoint := fmt.Sprintf("https://braket.%s.amazonaws.com/quantum-task/%s",
		cfg.Region, url.PathEscape(taskArn))

	for {
		resp, err := awsDo("GET", endpoint, cfg.Region, "braket", creds, nil)
		if err != nil {
			return err
		}
		var r struct {
			Status string `json:"status"`
			FailureReason string `json:"failureReason"`
		}
		json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()

		switch r.Status {
		case "COMPLETED":
			fmt.Print("\n")
			return nil
		case "FAILED", "CANCELLED":
			msg := r.FailureReason
			if msg == "" {
				msg = r.Status
			}
			return fmt.Errorf("task %s: %s", taskArn, msg)
		default:
			fmt.Printf("\r  Braket task status: %-10s", r.Status)
			time.Sleep(5 * time.Second)
		}
	}
}

func braketResults(region string, creds awsCreds, bucket, directory string) (map[string]int, error) {
	// Results are in s3://{bucket}/{directory}/results.json
	s3URL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/results.json",
		bucket, region, strings.TrimPrefix(directory, "/"))

	resp, err := awsDo("GET", s3URL, region, "s3", creds, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result struct {
		Measurements [][]int `json:"measurements"`
		MeasuredQubits []int `json:"measuredQubits"`
		MeasurementProbabilities map[string]float64 `json:"measurementProbabilities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse S3 result: %w", err)
	}

	counts := make(map[string]int)

	// Use exact measurements array (preferred — gives exact counts)
	if len(result.Measurements) > 0 {
		for _, shot := range result.Measurements {
			bits := make([]byte, len(shot))
			for i, b := range shot {
				if b == 1 {
					bits[i] = '1'
				} else {
					bits[i] = '0'
				}
			}
			counts[string(bits)]++
		}
		return counts, nil
	}

	// Fallback: use measurementProbabilities × totalShots
	if len(result.MeasurementProbabilities) > 0 {
		// sum probabilities to get total shots implicitly
		total := len(result.Measurements)
		for k, p := range result.MeasurementProbabilities {
			counts[k] = int(p * float64(total))
		}
		return counts, nil
	}

	return nil, fmt.Errorf("no measurements in Braket result: %s", string(raw))
}

// awsDo makes a SigV4-signed HTTP request to an AWS service.
func awsDo(method, rawURL, region, service string, creds awsCreds, body []byte) (*http.Response, error) {
	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	bodyHash := sha256hexBytes(body)

	req.Header.Set("host", u.Host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", bodyHash)
	if creds.SessionToken != "" {
		req.Header.Set("x-amz-security-token", creds.SessionToken)
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}

	// Build sorted signed headers
	var headerKeys []string
	for k := range req.Header {
		headerKeys = append(headerKeys, strings.ToLower(k))
	}
	sort.Strings(headerKeys)

	var canonLines []string
	for _, k := range headerKeys {
		// http.Header canonicalises keys, so we must look up carefully
		for hk, hvs := range req.Header {
			if strings.ToLower(hk) == k {
				canonLines = append(canonLines, k+":"+strings.TrimSpace(hvs[0]))
				break
			}
		}
	}
	canonHeaders := strings.Join(canonLines, "\n") + "\n"
	signedHeaders := strings.Join(headerKeys, ";")

	// Canonical query string (already sorted by url.Parse)
	canonQuery := u.RawQuery

	canonReq := strings.Join([]string{
		method,
		u.EscapedPath(),
		canonQuery,
		canonHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	credScope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	strToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credScope + "\n" + sha256hexStr(canonReq)

	kDate := hmacSHA256Bytes([]byte("AWS4"+creds.SecretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256Bytes(kDate, []byte(region))
	kService := hmacSHA256Bytes(kRegion, []byte(service))
	kSigning := hmacSHA256Bytes(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256Bytes(kSigning, []byte(strToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credScope, signedHeaders, sig)
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from AWS %s: %s", resp.StatusCode, service, string(b))
	}
	return resp, nil
}

func hmacSHA256Bytes(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256hexBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sha256hexStr(s string) string {
	return sha256hexBytes([]byte(s))
}
