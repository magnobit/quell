// Copyright 2026 Magnobit, Inc. All rights reserved.

package telemetry

import (
	"net/http"
	"time"
)

// telemetryHTTPClient bounds every provider characterization call.
// http.DefaultClient has no timeout.
var telemetryHTTPClient = &http.Client{Timeout: 20 * time.Second}
