// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
)

// providerHTTPClient is shared by every remote adapter. http.DefaultClient
// has no timeout; a hung provider would otherwise pin a worker forever.
var providerHTTPClient = &http.Client{Timeout: 30 * time.Second}

func doJSON(provider, method, rawURL, authorization string, extra http.Header, body []byte) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, r)
	if err != nil {
		return nil, err
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}

	resp, err := providerHTTPClient.Do(req)
	if err != nil {
		return nil, &ProviderError{
			Provider: provider,
			Class:    classifyNet(err),
			Message:  redactSecrets(err.Error()),
		}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, httpStatusError(provider, resp.StatusCode, b)
	}
	return resp, nil
}

func httpStatusError(provider string, code int, body []byte) error {
	msg := redactSecrets(strings.TrimSpace(string(body)))
	if msg == "" {
		msg = http.StatusText(code)
	}
	return &ProviderError{
		Provider: provider,
		Class:    classifyHTTP(code),
		Status:   code,
		Message:  msg,
	}
}
