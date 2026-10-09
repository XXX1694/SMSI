package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

const maxBody = 1 << 20

// xrpcError is the AT Protocol error body.
type xrpcError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// call is one XRPC request. bearer is the accessJwt (or refreshJwt for
// refreshSession); it is set as a header only and never copied into an error.
type call struct {
	base    string
	method  string
	nsid    string
	bearer  string
	query   url.Values
	body    any    // JSON body
	raw     []byte // raw body (uploadBlob)
	rawType string
}

func (a *Adapter) clientFor(base string) *http.Client {
	if base == a.pds {
		return a.fixed
	}
	return a.custom
}

// do runs the request and decodes a 2xx JSON response into out (may be nil).
func (a *Adapter) do(ctx context.Context, c call, out any) error {
	u := c.base + "/xrpc/" + c.nsid
	if len(c.query) > 0 {
		u += "?" + c.query.Encode()
	}
	var rdr io.Reader
	ctype := ""
	switch {
	case c.raw != nil:
		rdr, ctype = bytes.NewReader(c.raw), c.rawType
	case c.body != nil:
		b, err := json.Marshal(c.body)
		if err != nil {
			return permanent("BAD_REQUEST", "could not encode the request")
		}
		rdr, ctype = bytes.NewReader(b), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, c.method, u, rdr)
	if err != nil {
		return permanent("BAD_REQUEST", "invalid Bluesky server URL")
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("Accept", "application/json")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	resp, err := a.clientFor(c.base).Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode/100 != 2 {
		return a.errorFromResponse(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "BAD_RESPONSE", Message: "Bluesky sent an unreadable response", Err: err}
	}
	return nil
}

func permanent(code, msg string) *provider.Error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: code, Message: msg}
}

// isAuth reports an expired or rejected token or password.
func isAuth(err error) bool { return provider.Classify(err) == provider.KindAuth }

// errorFromResponse maps a non-2xx response. Only the protocol error name and a
// truncated server message are kept; request headers never are.
func (a *Adapter) errorFromResponse(resp *http.Response) *provider.Error {
	var xe xrpcError
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&xe)
	msg := xe.Message
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	pe := provider.FromHTTPStatus(Name, resp.StatusCode, "Bluesky: "+msg, a.retryDelay(resp))
	if xe.Error != "" {
		pe.Code = xe.Error
	}
	switch xe.Error {
	case "AuthenticationRequired", "ExpiredToken", "InvalidToken":
		pe.Kind = provider.KindAuth
		pe.Message = "Bluesky rejected the credentials: reconnect the account"
	case "InvalidRequest", "RecordNotFound":
		pe.Kind = provider.KindPermanent
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		pe.Kind = provider.KindRetryable
		pe.Message = "Bluesky rate limit reached"
	}
	return pe
}

// retryDelay reads RateLimit-Reset (epoch seconds, IETF draft) or Retry-After.
func (a *Adapter) retryDelay(resp *http.Response) time.Duration {
	if s := strings.TrimSpace(resp.Header.Get("RateLimit-Reset")); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			if n > 1e9 {
				return max(time.Unix(n, 0).Sub(a.cfg.Now()), time.Second)
			}
			return time.Duration(n) * time.Second
		}
	}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 0
}
