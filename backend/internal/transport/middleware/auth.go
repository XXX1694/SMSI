package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/transport/httpx"
)

// Cookie and header names.
const (
	SessionCookie = "socialos_session"
	CSRFCookie    = "socialos_csrf"
	CSRFHeader    = "X-CSRF-Token"
)

// Authenticator resolves credentials into actors.
type Authenticator interface {
	AuthenticateSession(ctx context.Context, raw string, ci auth.ClientInfo) (actor.Actor, string, error)
	AuthenticateAPIKey(ctx context.Context, raw string, ci auth.ClientInfo) (actor.Actor, error)
}

type csrfKey struct{}

// CSRFToken returns the session CSRF token for the request (empty for API keys).
func CSRFToken(ctx context.Context) string {
	s, _ := ctx.Value(csrfKey{}).(string)
	return s
}

// ClientInfoFrom builds auth.ClientInfo for a request. The address is the one the MCP gateway vouched for (see
// Gateway) or, failing that, ClientIP.
func ClientInfoFrom(r *http.Request, trusted TrustedProxies) auth.ClientInfo {
	ip, ok := gatewayIP(r.Context())
	if !ok {
		ip = ClientIP(r, trusted)
	}
	return auth.ClientInfo{UserAgent: r.UserAgent(), IP: ip, RequestID: httpx.RequestID(r.Context())}
}

// Authenticate resolves a Bearer API key or the session cookie. Invalid bearer
// credentials are rejected immediately; an invalid cookie means anonymous.
func Authenticate(a Authenticator, trusted TrustedProxies) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ci := ClientInfoFrom(r, trusted)
			ctx := r.Context()
			if h := r.Header.Get("Authorization"); h != "" {
				scheme, token, _ := strings.Cut(h, " ")
				if !strings.EqualFold(scheme, "Bearer") || token == "" {
					httpx.ErrorCode(w, r, errs.Unauthenticated, "malformed Authorization header")
					return
				}
				act, err := a.AuthenticateAPIKey(ctx, strings.TrimSpace(token), ci)
				if err != nil {
					httpx.Error(w, r, err)
					return
				}
				publishActor(ctx, act)
				next.ServeHTTP(w, r.WithContext(actor.With(ctx, act)))
				return
			}
			if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
				act, csrf, err := a.AuthenticateSession(ctx, c.Value, ci)
				if err == nil {
					publishActor(ctx, act)
					ctx = context.WithValue(actor.With(ctx, act), csrfKey{}, csrf)
				} else if !errs.Is(err, errs.Unauthenticated) {
					httpx.Error(w, r, err)
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth rejects anonymous requests.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := actor.From(r.Context()); !ok {
			httpx.ErrorCode(w, r, errs.Unauthenticated, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CSRF enforces the double-submit header for session-authenticated unsafe methods.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		a, ok := actor.From(r.Context())
		if ok && a.IsSession() {
			want := CSRFToken(r.Context())
			got := r.Header.Get(CSRFHeader)
			if want == "" || got == "" || !crypto.ConstantTimeEqual(got, want) {
				httpx.ErrorCode(w, r, errs.Forbidden, "missing or invalid CSRF token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
