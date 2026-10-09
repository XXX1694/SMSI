package bluesky

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// defaultBlock is how long createSession is skipped after a 429 without a reset hint.
const defaultBlock = time.Minute

type session struct {
	accessJwt, refreshJwt string
	accessExp, refreshExp time.Time
	did, handle           string
}

type sessionResponse struct {
	AccessJwt  string `json:"accessJwt"`
	RefreshJwt string `json:"refreshJwt"`
	Handle     string `json:"handle"`
	DID        string `json:"did"`
}

// sessionEntry serialises session work for one account so parallel publishes do
// not burn the createSession budget (30 per 5 minutes) [V].
type sessionEntry struct {
	mu           sync.Mutex
	sess         *session
	pwMAC        []byte // keyed fingerprint of the password this session was made with
	blockedUntil time.Time
}

type sessionCache struct {
	mu      sync.Mutex
	entries map[string]*sessionEntry
	macKey  []byte // random per process, never persisted
}

func newSessionCache() *sessionCache {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		panic("bluesky: no randomness for the session cache: " + err.Error())
	}
	return &sessionCache{entries: map[string]*sessionEntry{}, macKey: k}
}

// fingerprint is an HMAC-SHA256 of the password under a random in-memory key. It
// only detects that the stored password changed; it is not a stored credential
// hash and cannot be reproduced outside this process.
func (c *sessionCache) fingerprint(password string) []byte {
	m := hmac.New(sha256.New, c.macKey)
	m.Write([]byte(password))
	return m.Sum(nil)
}

func (c *sessionCache) entry(key string) *sessionEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[key]
	if e == nil {
		e = &sessionEntry{}
		c.entries[key] = e
	}
	return e
}

// target is what a session is made for.
type target struct {
	key        string // account id; entries drop when the password changes
	base       string
	identifier string // DID or handle
	password   string
}

func (a *Adapter) create(ctx context.Context, t target) (*session, error) {
	var r sessionResponse
	err := a.do(ctx, call{base: t.base, method: http.MethodPost, nsid: "com.atproto.server.createSession",
		body: map[string]string{"identifier": t.identifier, "password": t.password}}, &r)
	if err != nil {
		return nil, err
	}
	return a.sessionFrom(r)
}

func (a *Adapter) refresh(ctx context.Context, t target, old *session) (*session, error) {
	var r sessionResponse
	err := a.do(ctx, call{base: t.base, method: http.MethodPost, nsid: "com.atproto.server.refreshSession", bearer: old.refreshJwt}, &r)
	if err != nil {
		return nil, err
	}
	return a.sessionFrom(r)
}

func (a *Adapter) sessionFrom(r sessionResponse) (*session, error) {
	if r.AccessJwt == "" || r.RefreshJwt == "" || r.DID == "" {
		return nil, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "BAD_RESPONSE", Message: "Bluesky sent an incomplete session"}
	}
	now := a.cfg.Now()
	return &session{accessJwt: r.AccessJwt, refreshJwt: r.RefreshJwt, did: r.DID, handle: r.Handle,
		accessExp: jwtExpiry(r.AccessJwt, now.Add(5*time.Minute)), refreshExp: jwtExpiry(r.RefreshJwt, now.Add(time.Hour))}, nil
}

// jwtExpiry reads the exp claim without verifying anything; it only schedules a refresh.
func jwtExpiry(token string, fallback time.Time) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fallback
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fallback
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(b, &claims) != nil || claims.Exp <= 0 {
		return fallback
	}
	return time.Unix(claims.Exp, 0)
}

// acquire returns a usable session: the cached one, a refreshed one, or a new
// one, in that order. force skips the cached access token (it was rejected).
func (a *Adapter) acquire(ctx context.Context, t target, force bool) (*session, error) {
	e := a.sessions.entry(t.key)
	e.mu.Lock()
	defer e.mu.Unlock()
	now := a.cfg.Now()
	h := a.sessions.fingerprint(t.password)
	if e.sess != nil && !hmac.Equal(h, e.pwMAC) {
		e.sess = nil
	}
	if e.sess != nil && !force && now.Add(30*time.Second).Before(e.sess.accessExp) {
		return e.sess, nil
	}
	if e.sess != nil && now.Add(30*time.Second).Before(e.sess.refreshExp) {
		s, err := a.refresh(ctx, t, e.sess)
		if err == nil {
			e.sess, e.pwMAC = s, h
			return s, nil
		}
		if !isAuth(err) && !isRateLimited(err) {
			return nil, err
		}
		if isRateLimited(err) {
			a.block(e, err)
			return nil, err
		}
	}
	if now.Before(e.blockedUntil) {
		return nil, rateLimited(e.blockedUntil.Sub(now))
	}
	s, err := a.create(ctx, t)
	if err != nil {
		if isRateLimited(err) {
			a.block(e, err)
		}
		if isAuth(err) {
			e.sess = nil
		}
		return nil, err
	}
	e.sess, e.pwMAC = s, h
	return s, nil
}

func (a *Adapter) block(e *sessionEntry, err error) {
	d := provider.RetryAfterOf(err)
	if d <= 0 {
		d = defaultBlock
	}
	e.blockedUntil = a.cfg.Now().Add(d)
}

func isRateLimited(err error) bool {
	var pe *provider.Error
	return errors.As(err, &pe) && pe.HTTPStatus == http.StatusTooManyRequests
}

func rateLimited(d time.Duration) *provider.Error {
	return &provider.Error{Kind: provider.KindRetryable, Provider: Name, HTTPStatus: http.StatusTooManyRequests,
		Code: "RATE_LIMITED", Message: "Bluesky rate limit reached", RetryAfter: d}
}

// withSession runs fn with a valid access token. A rejected token is refreshed
// once; if the retry is rejected too, the Auth error surfaces (reconnect needed).
func (a *Adapter) withSession(ctx context.Context, t target, fn func(*session) error) error {
	s, err := a.acquire(ctx, t, false)
	if err != nil {
		return err
	}
	if err = fn(s); !isAuth(err) {
		return err
	}
	if s, err = a.acquire(ctx, t, true); err != nil {
		return err
	}
	return fn(s)
}
