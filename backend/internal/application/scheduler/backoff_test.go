package scheduler

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

func TestBackoffExponential(t *testing.T) {
	mid := func() float64 { return 0.5 } // no jitter offset
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}
	for n, w := range want {
		if got := Backoff(n, 0, mid); got != w {
			t.Errorf("n=%d got %s want %s", n, got, w)
		}
	}
	if got := Backoff(30, 0, mid); got != BackoffCap {
		t.Errorf("cap: got %s", got)
	}
	if got := Backoff(-1, 0, mid); got != 30*time.Second {
		t.Errorf("negative: got %s", got)
	}
}

func TestBackoffJitterBounds(t *testing.T) {
	lo := Backoff(2, 0, func() float64 { return 0 })
	hi := Backoff(2, 0, func() float64 { return 0.9999 })
	if lo != 96*time.Second || hi < 143*time.Second || hi > 144*time.Second {
		t.Fatalf("jitter bounds lo=%s hi=%s", lo, hi)
	}
	for i := 0; i < 100; i++ {
		d := Backoff(0, 0, nil)
		if d < 24*time.Second || d > 36*time.Second {
			t.Fatalf("random jitter out of bounds: %s", d)
		}
	}
}

func TestBackoffHonoursRetryAfter(t *testing.T) {
	if got := Backoff(0, 10*time.Minute, func() float64 { return 0.5 }); got != 10*time.Minute {
		t.Fatalf("retry-after ignored: %s", got)
	}
	err := &RetryableError{Err: provider.FromHTTPStatus("telegram", 429, "slow down", 5*time.Minute)}
	if got := RetryDelay(0, err); got != 5*time.Minute {
		t.Fatalf("RetryDelay got %s", got)
	}
}

func TestRetryableClassification(t *testing.T) {
	if !IsRetryable(fmt.Errorf("x: %w", &RetryableError{Err: errors.New("503")})) {
		t.Fatal("wrapped retryable not detected")
	}
	if IsRetryable(errors.New("permanent")) {
		t.Fatal("plain error must not be retryable")
	}
	if !(RetryInfo{Retried: 5, MaxRetry: 5}).Exhausted() || (RetryInfo{Retried: 4, MaxRetry: 5}).Exhausted() {
		t.Fatal("exhausted wrong")
	}
}
