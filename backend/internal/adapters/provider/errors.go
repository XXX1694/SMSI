package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"time"
)

// Kind classifies a provider failure for retry decisions.
type Kind int

const (
	// KindPermanent: retrying cannot help (bad request, forbidden, unsupported media).
	KindPermanent Kind = iota
	// KindRetryable: transient and nothing was created (429, 5xx, connect failures).
	KindRetryable
	// KindAuth: credentials are invalid/expired; account must be reconnected.
	KindAuth
	// KindUnknown: the request may have been applied (timeout after send).
	KindUnknown
	// KindUnsupported: the provider does not support the operation.
	KindUnsupported
)

func (k Kind) String() string {
	switch k {
	case KindRetryable:
		return "retryable"
	case KindAuth:
		return "auth"
	case KindUnknown:
		return "unknown"
	case KindUnsupported:
		return "unsupported"
	default:
		return "permanent"
	}
}

// Error is a classified provider failure. Message must never contain tokens.
type Error struct {
	Kind       Kind
	Provider   string
	Code       string
	Message    string
	HTTPStatus int
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	s := fmt.Sprintf("%s provider error (%s", e.Provider, e.Kind)
	if e.HTTPStatus != 0 {
		s += fmt.Sprintf(", http %d", e.HTTPStatus)
	}
	s += "): " + e.Message
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// ErrUnsupported is returned by stubs and unsupported operations.
var ErrUnsupported = errors.New("operation not supported by provider")

// FromHTTPStatus builds a classified error from an HTTP response status.
func FromHTTPStatus(providerName string, status int, msg string, retryAfter time.Duration) *Error {
	e := &Error{Provider: providerName, HTTPStatus: status, Message: msg, RetryAfter: retryAfter, Code: fmt.Sprintf("HTTP_%d", status)}
	switch {
	case status == 429:
		e.Kind = KindRetryable
	case status == 401:
		e.Kind = KindAuth
	case status >= 500:
		e.Kind = KindRetryable
	default:
		e.Kind = KindPermanent
	}
	return e
}

// Classify returns the failure kind of any error produced while calling a provider.
func Classify(err error) Kind {
	if err == nil {
		return KindPermanent
	}
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	if errors.Is(err, ErrUnsupported) {
		return KindUnsupported
	}
	if errors.Is(err, context.Canceled) {
		// Worker shutdown mid-request: outcome unknown.
		return KindUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return KindUnknown
	}
	if isDialError(err) {
		return KindRetryable
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return KindUnknown
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return KindRetryable
	}
	return KindPermanent
}

// isDialError reports failures that happen before any byte reaches the provider.
func isDialError(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

// RetryAfterOf extracts a provider-requested delay (e.g. Telegram retry_after).
func RetryAfterOf(err error) time.Duration {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.RetryAfter
	}
	return 0
}

// SafeMessage returns a client-safe message for err.
func SafeMessage(err error) string {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Message
	}
	if errors.Is(err, ErrUnsupported) {
		return ErrUnsupported.Error()
	}
	return "provider request failed"
}

// CodeOf returns the provider error code if present.
func CodeOf(err error) string {
	var pe *Error
	if errors.As(err, &pe) && pe.Code != "" {
		return pe.Code
	}
	return "PROVIDER_ERROR"
}
