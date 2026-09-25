// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	accessKey := cfg.AccessKeyID
	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	secretKey := cfg.SecretAccessKey
	if secretKey == "" {
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	sessionToken := cfg.SessionToken
	if sessionToken == "" {
		sessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}

	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("braket: access_key_id/secret_access_key are required (aws.access_key_id/aws.secret_access_key in quell.config.yml, or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY env vars)")
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
	taskArn, err := braketSubmit(cfg, creds, qasm3, shots)
	if err != nil {
		return nil, fmt.Errorf("braket: submit: %w", err)
	}
	notifySubmitted(cfg.OnSubmitted, taskArn)
	fmt.Printf("  Braket task submitted: %s\n", taskArn)

	s3Bucket, s3Dir, err := braketPoll(cfg, creds, taskArn)
	if err != nil {
		return nil, fmt.Errorf("braket: %w", err)
	}

	counts, err := braketResults(cfg, creds, s3Bucket, s3Dir, shots)
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

// braketEndpoint returns the Braket API base URL: cfg.BaseURL when set
// (contract tests), otherwise the real per-region Braket host.
func braketEndpoint(cfg *config.AWSConfig) string {
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return fmt.Sprintf("https://braket.%s.amazonaws.com", cfg.Region)
}

type awsCreds struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

func braketSubmit(cfg *config.AWSConfig, creds awsCreds, qasm3 string, shots int) (string, error) {
	endpoint := braketEndpoint(cfg)

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

	clientToken, err := newUUID()
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"clientToken":       clientToken,
		"deviceArn":         cfg.Device,
		"shots":             shots,
		"outputS3Bucket":    cfg.S3Bucket,
		"outputS3KeyPrefix": cfg.S3Prefix,
		"action":            string(actionJSON),
	}
	mergeExtra(payload, cfg.Extra)
	body, _ := json.Marshal(payload)

	resp, err := awsDo("POST", endpoint+"/quantum-task", cfg.Region, "braket", creds, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// CreateQuantumTask returns only the ARN; the results location comes
	// from GetQuantumTask while polling.
	var r struct {
		QuantumTaskArn string `json:"quantumTaskArn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.QuantumTaskArn == "" {
		return "", fmt.Errorf("no task ARN in response")
	}
	return r.QuantumTaskArn, nil
}

// braketPoll waits for the task to finish and returns where its results
// were written.
func braketPoll(cfg *config.AWSConfig, creds awsCreds, taskArn string) (bucket, directory string, err error) {
	endpoint := braketEndpoint(cfg) + "/quantum-task/" + awsEscapeSegment(taskArn)
	err = pollUntil("aws", taskArn, func() (pollTick, error) {
		resp, err := awsDo("GET", endpoint, cfg.Region, "braket", creds, nil)
		if err != nil {
			return pollTick{}, err
		}
		var r struct {
			Status            string `json:"status"`
			FailureReason     string `json:"failureReason"`
			OutputS3Bucket    string `json:"outputS3Bucket"`
			OutputS3Directory string `json:"outputS3Directory"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if decErr != nil {
			return pollTick{}, &ProviderError{Provider: "aws", Class: ClassInvalidRequest, Message: "malformed job status"}
		}
		switch r.Status {
		case "COMPLETED":
			bucket, directory = r.OutputS3Bucket, r.OutputS3Directory
			return pollTick{Done: true, Status: r.Status}, nil
		case "FAILED", "CANCELLED":
			return pollTick{Failed: true, Status: r.Status, Message: r.FailureReason}, nil
		default:
			return pollTick{Status: r.Status}, nil
		}
	})
	if err == nil && (bucket == "" || directory == "") {
		bucket, directory = cfg.S3Bucket, strings.TrimSuffix(cfg.S3Prefix, "/")+"/"+taskIDFromArn(taskArn)
	}
	return bucket, directory, err
}

// taskIDFromArn returns the id after "quantum-task/" in a task ARN; Braket
// names the results directory {prefix}/{id} by default.
func taskIDFromArn(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

func braketResults(cfg *config.AWSConfig, creds awsCreds, bucket, directory string, shots int) (map[string]int, error) {
	// Results are in s3://{bucket}/{directory}/results.json
	var s3URL string
	if cfg.BaseURL != "" {
		s3URL = fmt.Sprintf("%s/%s/%s/results.json", cfg.BaseURL, bucket, strings.TrimPrefix(directory, "/"))
	} else {
		s3URL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/results.json",
			bucket, cfg.Region, strings.TrimPrefix(directory, "/"))
	}

	resp, err := awsDo("GET", s3URL, cfg.Region, "s3", creds, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result struct {
		Measurements             [][]int            `json:"measurements"`
		MeasuredQubits           []int              `json:"measuredQubits"`
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

	// Fallback: measurementProbabilities × requested shots.
	if len(result.MeasurementProbabilities) > 0 {
		for k, p := range result.MeasurementProbabilities {
			counts[k] = int(math.Round(p * float64(shots)))
		}
		return counts, nil
	}

	return nil, fmt.Errorf("no measurements in Braket result: %s", string(raw))
}

// CancelBraket asks AWS Braket to cancel an in-flight quantum task.
func CancelBraket(cfg *config.AWSConfig, providerJobID string) error {
	if cfg == nil {
		return fmt.Errorf("braket: config is required to cancel")
	}
	if providerJobID == "" {
		return fmt.Errorf("braket: provider job id is required to cancel")
	}
	accessKey := cfg.AccessKeyID
	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	secretKey := cfg.SecretAccessKey
	if secretKey == "" {
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	sessionToken := cfg.SessionToken
	if sessionToken == "" {
		sessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("braket: access_key_id/secret_access_key are required to cancel")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	creds := awsCreds{accessKey, secretKey, sessionToken}
	endpoint := braketEndpoint(cfg) + "/quantum-task/" + awsEscapeSegment(providerJobID) + "/cancel"
	clientToken, err := newUUID()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"clientToken": clientToken})
	resp, err := awsDo("PUT", endpoint, cfg.Region, "braket", creds, body)
	if err != nil {
		return fmt.Errorf("braket: cancel: %w", err)
	}
	resp.Body.Close()
	return nil
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
		awsCanonicalPath(u.EscapedPath(), service),
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

	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return nil, &ProviderError{Provider: "aws", Class: classifyNet(err), Message: redactSecrets(err.Error())}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, httpStatusError("aws", resp.StatusCode, b)
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

// awsEscapeSegment URI-encodes one path segment the way SigV4 does: every
// byte except A-Z a-z 0-9 - _ . ~ becomes %XX. Use it for values such as
// task ARNs that contain ':' and '/'.
func awsEscapeSegment(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') || ('0' <= c && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// awsCanonicalPath builds the SigV4 canonical URI from the path as sent.
// Every service except S3 encodes each segment twice; S3 encodes once.
func awsCanonicalPath(escapedPath, service string) string {
	if escapedPath == "" {
		return "/"
	}
	segs := strings.Split(escapedPath, "/")
	for i, seg := range segs {
		dec, err := url.PathUnescape(seg)
		if err != nil {
			dec = seg
		}
		enc := awsEscapeSegment(dec)
		if service != "s3" {
			enc = awsEscapeSegment(enc)
		}
		segs[i] = enc
	}
	return strings.Join(segs, "/")
}
