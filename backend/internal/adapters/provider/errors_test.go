package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"
	"time"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestFromHTTPStatus(t *testing.T) {
	cases := map[int]Kind{429: KindRetryable, 500: KindRetryable, 503: KindRetryable, 401: KindAuth, 400: KindPermanent, 403: KindPermanent, 422: KindPermanent}
	for status, want := range cases {
		if got := FromHTTPStatus("x", status, "m", 0).Kind; got != want {
			t.Errorf("status %d: got %s want %s", status, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	dial := &url.Error{Op: "Post", URL: "u", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}
	cases := []struct {
		name string
		err  error
		want Kind
	}{
		{"provider retryable", FromHTTPStatus("x", 502, "bad gateway", 0), KindRetryable},
		{"wrapped provider auth", fmt.Errorf("wrap: %w", FromHTTPStatus("x", 401, "expired", 0)), KindAuth},
		{"dial refused", dial, KindRetryable},
		{"dns", &net.DNSError{Err: "no such host"}, KindRetryable},
		{"deadline", context.DeadlineExceeded, KindUnknown},
		{"canceled", context.Canceled, KindUnknown},
		{"net timeout", &url.Error{Op: "Post", URL: "u", Err: timeoutErr{}}, KindUnknown},
		{"reset", fmt.Errorf("read: %w", syscall.ECONNRESET), KindUnknown},
		{"eof", io.ErrUnexpectedEOF, KindUnknown},
		{"unsupported", ErrUnsupported, KindUnsupported},
		{"other", errors.New("boom"), KindPermanent},
	}
	for _, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestRetryAfterAndMessages(t *testing.T) {
	e := FromHTTPStatus("telegram", 429, "Too Many Requests", 7*time.Second)
	if RetryAfterOf(fmt.Errorf("x: %w", e)) != 7*time.Second {
		t.Fatal("retry after lost")
	}
	if SafeMessage(errors.New("secret token abc")) != "provider request failed" {
		t.Fatal("raw error leaked")
	}
	if CodeOf(e) != "HTTP_429" || CodeOf(errors.New("x")) != "PROVIDER_ERROR" {
		t.Fatal("code wrong")
	}
}
