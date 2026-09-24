// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Stable low-cardinality provider failure classes. These are metric labels
// and API/error-class fields — never the raw provider error string.
const (
	ClassAuth                = "auth"
	ClassPermission          = "permission"
	ClassRateLimit           = "rate_limit"
	ClassTimeout             = "timeout"
	ClassProviderUnavailable = "provider_unavailable"
	ClassInvalidRequest      = "invalid_request"
	ClassQuota               = "quota"
	ClassBackendUnavailable  = "backend_unavailable"
	ClassJobNotFound         = "job_not_found"
	ClassCancelUnsupported   = "cancel_unsupported"
	ClassProviderError       = "provider_error"
)

// ProviderError is a classified, redacted adapter failure.
type ProviderError struct {
	Provider string
	Class    string
	Status   int
	Message  string
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	msg := e.Message
	if msg == "" {
		msg = e.Class
	}
	if e.Status > 0 {
		return fmt.Sprintf("%s: %s (HTTP %d): %s", e.Provider, e.Class, e.Status, msg)
	}
	return fmt.Sprintf("%s: %s: %s", e.Provider, e.Class, msg)
}

// ClassOf returns a stable error class for metrics. Unknown errors map to
// provider_unavailable rather than the raw string.
func ClassOf(err error) string {
	if err == nil {
		return ""
	}
	var pe *ProviderError
	if errors.As(err, &pe) && pe.Class != "" {
		return pe.Class
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ClassTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return ClassTimeout
	}
	return ClassProviderUnavailable
}

func classifyHTTP(code int) string {
	switch {
	case code == 401:
		return ClassAuth
	case code == 403:
		return ClassPermission
	case code == 404:
		return ClassJobNotFound
	case code == 402 || code == 413:
		return ClassQuota
	case code == 408 || code == 504:
		return ClassTimeout
	case code == 429:
		return ClassRateLimit
	case code >= 500:
		return ClassProviderUnavailable
	case code >= 400:
		return ClassInvalidRequest
	default:
		return ClassProviderError
	}
}

func classifyNet(err error) string {
	if err == nil {
		return ""
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ClassTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "context canceled") {
		return ClassTimeout
	}
	return ClassProviderUnavailable
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)\S+`),
	regexp.MustCompile(`(?i)(api[_-]?key["\s:=]+)[^\s"]+`),
	regexp.MustCompile(`(?i)(authorization["\s:=]+)[^\s"]+`),
	regexp.MustCompile(`(?i)("(token|api_key|apikey|client_secret|secret_access_key|password|session_token)"\s*:\s*")[^"]+`),
}

func redactSecrets(s string) string {
	out := s
	for _, re := range secretPatterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			loc := re.FindStringSubmatchIndex(m)
			if len(loc) >= 4 {
				return m[:loc[3]-loc[0]] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	if len(out) > 512 {
		return out[:512] + "…"
	}
	return out
}
