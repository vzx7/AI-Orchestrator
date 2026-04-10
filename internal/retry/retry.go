// Package retry implements a global retry policy with exponential backoff.
//
// The retry policy is applied uniformly across:
// - RPC calls
// - Task execution
// - Queue processing
package retry

import (
	"context"
	"time"
)

// Policy defines a retry strategy with exponential backoff.
type Policy struct {
	MaxAttempts int           // Maximum number of attempts (including the first)
	Backoff     time.Duration // Initial backoff duration
	Multiplier  float64       // Multiplier for exponential growth
	MaxBackoff  time.Duration // Upper bound on backoff duration
}

// DefaultPolicy returns a production-ready retry policy.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts: 5,
		Backoff:     1 * time.Second,
		Multiplier:  2.0,
		MaxBackoff:  30 * time.Second,
	}
}

// NoRetryPolicy returns a policy that never retries.
func NoRetryPolicy() Policy {
	return Policy{
		MaxAttempts: 1,
		Backoff:     0,
		Multiplier:  1.0,
		MaxBackoff:  0,
	}
}

// Do executes fn with retry according to the policy.
// Returns the result of the last attempt.
func (p Policy) Do(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Check if retryable
		if !IsRetryable(lastErr) {
			return lastErr
		}

		// Don't wait after the last attempt
		if attempt >= p.MaxAttempts {
			break
		}

		// Calculate backoff with exponential growth
		backoff := p.Backoff * time.Duration(uint(1)<<(attempt-1))
		if backoff > p.MaxBackoff {
			backoff = p.MaxBackoff
		}

		// Wait with context awareness
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	return lastErr
}

// IsRetryable determines if an error is transient and worth retrying.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are not retryable
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	msg := err.Error()

	// Retryable errors (transient failures)
	retryable := []string{
		"timeout",
		"deadline exceeded",
		"connection refused",
		"connection reset",
		"rate limit",
		"too many requests",
		"temporary",
		"unavailable",
		"unavailable",
		"no workers",
	}

	for _, keyword := range retryable {
		if containsFold(msg, keyword) {
			return true
		}
	}

	return false
}

// IsFatal is the inverse of IsRetryable.
func IsFatal(err error) bool {
	return err != nil && !IsRetryable(err)
}

func containsFold(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	sLower := toLower(s)
	substrLower := toLower(substr)
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}
