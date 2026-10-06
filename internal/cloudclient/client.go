// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package cloudclient calls the QubitLabs public job API.
// It does not compile or simulate. The server does that.
package cloudclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin HTTP client for /api/v1/jobs.
type Client struct {
	Endpoint string
	APIKey   string
	HTTP     *http.Client
}

// Error is the server's machine-readable error.
type Error struct {
	Status    int
	Code      string
	Message   string
	RequestID string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Job struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Language string         `json:"language"`
	Backend  string         `json:"backend"`
	Result   map[string]any `json:"result"`
	Error    string         `json:"error"`
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c *Client) ListBackends() ([]map[string]any, error) {
	var out struct {
		Backends []map[string]any `json:"backends"`
	}
	if err := c.do(http.MethodGet, "/api/v1/backends", nil, &out); err != nil {
		return nil, err
	}
	return out.Backends, nil
}

func (c *Client) Submit(source, language, backend string, shots int, idempotencyKey string) (*Job, error) {
	if backend == "" {
		backend = "auto"
	}
	body := map[string]any{
		"language": language,
		"source":   source,
		"shots":    shots,
		"target":   map[string]string{"backend": backend},
	}
	headers := map[string]string{}
	if idempotencyKey != "" {
		body["client_request_id"] = idempotencyKey
		headers["Idempotency-Key"] = idempotencyKey
	}
	var job Job
	if err := c.do(http.MethodPost, "/api/v1/jobs", body, &job, headers); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) Get(jobID string) (*Job, error) {
	var job Job
	if err := c.do(http.MethodGet, "/api/v1/jobs/"+jobID, nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) Result(jobID string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(http.MethodGet, "/api/v1/jobs/"+jobID+"/result", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Provenance(jobID string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(http.MethodGet, "/api/v1/jobs/"+jobID+"/provenance", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Verify(jobID string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(http.MethodGet, "/api/v1/jobs/"+jobID+"/verify", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Cancel(jobID string) error {
	return c.do(http.MethodPost, "/api/v1/jobs/"+jobID+"/cancel", map[string]any{}, nil)
}

func (c *Client) do(method, path string, body any, dest any, headerPairs ...map[string]string) error {
	if strings.TrimSpace(c.APIKey) == "" {
		return &Error{Code: "AUTH_INVALID", Message: "QUBITLABS_API_KEY is required"}
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return &Error{Code: "INVALID_REQUEST", Message: "QUBITLABS_ENDPOINT is required"}
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, strings.TrimRight(c.Endpoint, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range headerPairs {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return parseError(resp.StatusCode, raw)
	}
	if dest == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func parseError(status int, raw []byte) error {
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &body)
	if body.Error.Code == "" {
		body.Error.Code = "INTERNAL_ERROR"
		body.Error.Message = strings.TrimSpace(string(raw))
	}
	return &Error{Status: status, Code: body.Error.Code, Message: body.Error.Message, RequestID: body.Error.RequestID}
}
