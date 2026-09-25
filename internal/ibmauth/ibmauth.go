// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package ibmauth holds what every IBM Quantum Platform caller needs: the
// regional API host for an instance CRN, and an IAM bearer token traded for
// the user's API key. Job submission (internal/backends) and calibration
// fetches (telemetry) both use it.
package ibmauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// APIVersion is sent as IBM-API-Version on every Quantum Platform call.
const APIVersion = "2026-04-15"

// DefaultIAMURL is IBM Cloud's token endpoint for API-key grants.
const DefaultIAMURL = "https://iam.cloud.ibm.com/identity/token"

var regionHosts = map[string]string{
	"us-east": "https://quantum.cloud.ibm.com",
	"eu-de":   "https://eu-de.quantum.cloud.ibm.com",
}

// IsCRN reports whether s looks like an IBM Quantum Platform instance CRN.
func IsCRN(s string) bool {
	parts := strings.Split(s, ":")
	return len(parts) >= 6 && parts[0] == "crn" && parts[1] == "v1" && parts[4] == "quantum-computing"
}

// Region returns the region segment of an instance CRN.
func Region(crn string) string {
	parts := strings.Split(crn, ":")
	if len(parts) < 6 {
		return ""
	}
	return parts[5]
}

// HostForCRN returns the API host (without the /api/v1 suffix) that serves
// the instance's region.
func HostForCRN(crn string) (string, error) {
	if !IsCRN(crn) {
		return "", fmt.Errorf("instance must be the instance CRN (crn:v1:…:quantum-computing:<region>:…) from quantum.cloud.ibm.com/instances")
	}
	region := Region(crn)
	host, ok := regionHosts[region]
	if !ok {
		return "", fmt.Errorf("instance region %q is not supported (us-east or eu-de)", region)
	}
	return host, nil
}

// TokenSource trades an API key for IAM bearer tokens and caches each one
// until shortly before it expires. Safe for concurrent use.
type TokenSource struct {
	APIKey string
	URL    string // DefaultIAMURL when empty
	Client *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// refreshMargin renews a token this long before IAM says it expires, so a
// request never goes out with a token that lapses in flight.
const refreshMargin = 5 * time.Minute

// Token returns a valid bearer token, fetching a new one when needed.
// Errors never include the API key.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Add(refreshMargin).Before(s.expiry) {
		return s.token, nil
	}
	endpoint := s.URL
	if endpoint == "" {
		endpoint = DefaultIAMURL
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ibm:params:oauth:grant-type:apikey")
	form.Set("apikey", s.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("iam: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("iam: token request failed: %s", redact(err.Error(), s.APIKey))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var tok struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		ErrorMessage string `json:"errorMessage"`
	}
	_ = json.Unmarshal(body, &tok)
	if resp.StatusCode >= 400 || tok.AccessToken == "" {
		msg := tok.ErrorMessage
		if msg == "" {
			msg = resp.Status
		}
		return "", &Error{Status: resp.StatusCode, Message: "iam: " + redact(msg, s.APIKey)}
	}
	ttl := time.Duration(tok.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	s.token = tok.AccessToken
	s.expiry = time.Now().Add(ttl)
	return s.token, nil
}

// Error is an IAM rejection. Status is the HTTP status IAM returned.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// SetHeaders adds the auth, instance, and version headers IBM expects.
func SetHeaders(h http.Header, bearer, crn string) {
	h.Set("Authorization", "Bearer "+bearer)
	h.Set("Service-CRN", crn)
	h.Set("IBM-API-Version", APIVersion)
}

func redact(msg, secret string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "[redacted]")
}
