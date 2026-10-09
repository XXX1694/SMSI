package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// Discord error codes that matter.
const (
	codeUnknownWebhook = 10015
	codeUnknownMessage = 10008
)

type apiError struct {
	Code       int     `json:"code"`
	Message    string  `json:"message"`
	RetryAfter float64 `json:"retry_after"`
}

// call sends one request and returns the body of a 2xx answer. Every failure is
// a classified *provider.Error whose text holds no URL, because the URL is the secret.
func (a *Adapter) call(ctx context.Context, req *http.Request, w webhook) ([]byte, int, error) {
	resp, err := a.http.Do(req.WithContext(ctx))
	if err != nil {
		return nil, 0, transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// The status was already received but the body was cut: the effect is unknown.
		return nil, resp.StatusCode, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "READ_FAILED", HTTPStatus: resp.StatusCode, Message: "Discord answer was cut off"}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, resp.StatusCode, nil
	}
	return nil, resp.StatusCode, statusError(resp.StatusCode, resp.Header, body, w)
}

// transportError drops the *url.Error (it carries the secret URL) but keeps the
// retry class: a timeout after send stays Unknown, a refused dial stays Retryable.
func transportError(err error) *provider.Error {
	kind := provider.Classify(err)
	msg := "could not reach Discord"
	if kind == provider.KindUnknown {
		msg = "Discord did not answer in time; the post may or may not have been sent"
	}
	return &provider.Error{Kind: kind, Provider: Name, Code: "TRANSPORT_" + strings.ToUpper(kind.String()), Message: msg}
}

// statusError maps a non-2xx answer per docs/PLATFORMS.md.
func statusError(status int, h http.Header, body []byte, w webhook) *provider.Error {
	var ae apiError
	_ = json.Unmarshal(body, &ae)
	switch {
	case status == http.StatusUnauthorized, status == http.StatusNotFound && ae.Code == codeUnknownWebhook:
		e := provider.FromHTTPStatus(Name, status, "Discord no longer accepts this webhook; reconnect the channel", 0)
		e.Kind = provider.KindAuth
		return e
	case status == http.StatusTooManyRequests:
		return provider.FromHTTPStatus(Name, status, "Discord rate limit reached; will retry", retryAfter(h, ae))
	case status >= 500:
		return provider.FromHTTPStatus(Name, status, "Discord is temporarily unavailable", 0)
	}
	e := provider.FromHTTPStatus(Name, status, "Discord rejected the request", 0)
	if ae.Message != "" {
		e.Message += ": " + scrub(ae.Message, w)
	}
	if ae.Code != 0 {
		e.Code = "DISCORD_" + strconv.Itoa(ae.Code)
	}
	return e
}

// retryAfter prefers the precise body value, then the headers; clamped to a sane range.
func retryAfter(h http.Header, ae apiError) time.Duration {
	secs := ae.RetryAfter
	if secs <= 0 {
		for _, k := range []string{"X-RateLimit-Reset-After", "Retry-After"} {
			if f, err := strconv.ParseFloat(h.Get(k), 64); err == nil && f > 0 {
				secs = f
				break
			}
		}
	}
	d := time.Duration(secs * float64(time.Second))
	if d <= 0 {
		return time.Second
	}
	if d > time.Hour {
		return time.Hour
	}
	return d
}

// scrub keeps an echoed credential and long texts out of messages.
func scrub(s string, w webhook) string {
	if w.token != "" {
		s = strings.ReplaceAll(s, w.token, "[redacted]")
	}
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}
