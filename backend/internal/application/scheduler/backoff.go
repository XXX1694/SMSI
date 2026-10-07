package scheduler

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// Retry policy (ARCHITECTURE.md §6): 30s·2^n with jitter, max 5 retries.
const (
	MaxRetry    = 5
	BackoffBase = 30 * time.Second
	BackoffCap  = time.Hour
	jitterRatio = 0.2
)

// Backoff returns the delay before retry number n (0-based). A provider
// Retry-After hint is honoured when larger. jitter returns a value in [0,1).
func Backoff(n int, retryAfter time.Duration, jitter func() float64) time.Duration {
	if n < 0 {
		n = 0
	}
	d := BackoffBase
	for i := 0; i < n && d < BackoffCap; i++ {
		d *= 2
	}
	if d > BackoffCap {
		d = BackoffCap
	}
	if jitter == nil {
		jitter = rand.Float64
	}
	// Spread ±20% to avoid thundering herds.
	d = time.Duration(float64(d) * (1 - jitterRatio + 2*jitterRatio*jitter()))
	if retryAfter > d {
		d = retryAfter
	}
	return d
}

// RetryableError tells the queue to retry; it carries the provider cause.
type RetryableError struct{ Err error }

func (e *RetryableError) Error() string { return "retryable: " + e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// IsRetryable reports whether err asks for a retry.
func IsRetryable(err error) bool {
	var re *RetryableError
	return errors.As(err, &re)
}

// RetryDelay is the queue's retry delay function.
func RetryDelay(n int, err error) time.Duration {
	return Backoff(n, provider.RetryAfterOf(err), nil)
}
