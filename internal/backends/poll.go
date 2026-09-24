// Copyright 2026 Magnobit, Inc. All rights reserved.

package backends

import (
	"fmt"
	"time"
)

// pollInterval / pollTimeout bound every provider status loop. Tests may
// shorten them; live jobs default to a 30-minute ceiling so a stuck QPU
// cannot pin a worker. A timeout never claims the provider cancelled.
var (
	pollInterval         = 4 * time.Second
	pollTimeout          = 30 * time.Minute
	pollTransientRetries = 3
	pollRateLimitRetries = 3
)

type pollTick struct {
	Done    bool
	Failed  bool
	Status  string
	Message string
}

func pollUntil(provider, jobID string, tick func() (pollTick, error)) error {
	deadline := time.Now().Add(pollTimeout)
	transients := 0
	rateLimits := 0
	for {
		if time.Now().After(deadline) {
			return &ProviderError{
				Provider: provider,
				Class:    ClassTimeout,
				Message:  "poll deadline exceeded; provider job may still be running",
			}
		}
		result, err := tick()
		if err != nil {
			switch ClassOf(err) {
			case ClassRateLimit:
				rateLimits++
				if rateLimits > pollRateLimitRetries {
					return err
				}
				sleepPoll(pollBackoff(rateLimits))
				continue
			case ClassTimeout, ClassProviderUnavailable:
				transients++
				if transients > pollTransientRetries {
					return err
				}
				sleepPoll(pollInterval)
				continue
			default:
				return err
			}
		}
		transients = 0
		rateLimits = 0
		if result.Done {
			fmt.Print("\n")
			return nil
		}
		if result.Failed {
			msg := result.Message
			if msg == "" {
				msg = result.Status
			}
			return &ProviderError{
				Provider: provider,
				Class:    ClassProviderError,
				Message:  redactSecrets("job " + jobID + ": " + msg),
			}
		}
		status := result.Status
		if status == "" {
			status = "pending"
		}
		fmt.Printf("\r  %s job status: %-12s", provider, status)
		sleepPoll(pollInterval)
	}
}

func pollBackoff(attempt int) time.Duration {
	d := pollInterval * time.Duration(1<<uint(attempt-1))
	if d > 30*time.Second {
		return 30 * time.Second
	}
	if d < pollInterval {
		return pollInterval
	}
	return d
}

func sleepPoll(d time.Duration) {
	if d <= 0 {
		return
	}
	time.Sleep(d)
}
