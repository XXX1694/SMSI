package mastodon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/safehttp"
)

const maxBody = 1 << 20

// reply is one HTTP answer, already read and bounded.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// call sends one request. A transport failure is returned as is (the worker
// classifies timeouts as Unknown and dial failures as Retryable) except for
// refusals of the SSRF guard, which can never succeed and are Permanent.
func (a *Adapter) call(req *http.Request, token string) (reply, error) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return reply{}, blockedAsPermanent(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return reply{}, blockedAsPermanent(err)
	}
	return reply{status: resp.StatusCode, header: resp.Header, body: b}, nil
}

func blockedAsPermanent(err error) error {
	for _, target := range []error{safehttp.ErrBlockedAddress, safehttp.ErrBlockedHost, safehttp.ErrInsecureScheme, safehttp.ErrRedirectNotAllow} {
		if errors.Is(err, target) {
			return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "INSTANCE_NOT_ALLOWED",
				Message: "Mastodon: this instance address is not allowed (https and public addresses only)"}
		}
	}
	if errors.Is(err, safehttp.ErrResponseTooLarge) {
		return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "RESPONSE_TOO_LARGE", Message: "Mastodon: the server answered with too much data"}
	}
	return err
}

func (a *Adapter) get(ctx context.Context, base, path, token string) (reply, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return reply{}, err
	}
	return a.call(req, token)
}

func (a *Adapter) postJSON(ctx context.Context, base, path, token string, payload any, idempotencyKey string) (reply, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return reply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(b))
	if err != nil {
		return reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return a.call(req, token)
}

// failure converts a non-2xx answer into a classified error. Only the server's
// error text is kept, with the token removed in case the server echoes it.
func (a *Adapter) failure(r reply, token string) *provider.Error {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(r.body, &body)
	msg := strings.TrimSpace(body.Error)
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	if msg == "" {
		msg = http.StatusText(r.status)
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	pe := provider.FromHTTPStatus(Name, r.status, "Mastodon: "+msg, 0)
	switch r.status {
	case http.StatusForbidden:
		// 403 means the token lacks a scope or was restricted: reconnect with the right scopes.
		pe.Kind = provider.KindAuth
	case http.StatusTooManyRequests:
		pe.RetryAfter = a.retryAfter(r.header)
	case http.StatusUnprocessableEntity:
		pe.Kind = provider.KindPermanent
	}
	return pe
}

// retryAfter prefers Mastodon's X-RateLimit-Reset (an RFC 3339 time) and falls
// back to Retry-After seconds. Zero lets the worker use its own backoff.
func (a *Adapter) retryAfter(h http.Header) time.Duration {
	if s := h.Get("X-RateLimit-Reset"); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			if d := t.Sub(a.cfg.Now()); d > 0 {
				return d
			}
		}
	}
	if n, err := strconv.Atoi(h.Get("Retry-After")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

// instanceBase turns the submitted URL into "https://host[:port]". Anything but
// a bare origin is refused, so a path or credentials cannot steer the request.
func instanceBase(raw string) (base, host string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", "", &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "INVALID_INSTANCE_URL",
			Message: "Mastodon: the instance URL must be an https address such as https://mastodon.social"}
	}
	host = strings.ToLower(u.Host)
	return "https://" + host, host, nil
}
