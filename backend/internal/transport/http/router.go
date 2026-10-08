// Package http is the REST API transport (chi router, handlers, DTOs).
package http

import (
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/go-chi/chi/v5"

	"github.com/socialos/backend/internal/application/accounts"
	"github.com/socialos/backend/internal/application/analytics"
	"github.com/socialos/backend/internal/application/audit"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/application/developer"
	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/application/posts"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/observability"
	"github.com/socialos/backend/internal/transport/httpx"
	"github.com/socialos/backend/internal/transport/middleware"
)

// Services are the use cases exposed over HTTP.
type Services struct {
	Auth      *auth.Service
	Accounts  *accounts.Service
	Posts     *posts.Service
	Media     *media.Service
	Developer *developer.Service
	Analytics *analytics.Service
	Audit     *audit.Service
}

// Options configure transport behaviour.
type Options struct {
	WebBaseURL   string
	CORSOrigins  []string
	CookieSecure bool
	CookieDomain string
	// TrustedProxies are the proxy networks whose X-Forwarded-For is believed (config TRUSTED_PROXIES); empty means the
	// header is ignored and the TCP peer is the client.
	TrustedProxies []netip.Prefix
	MetricsToken   string
	// GatewaySecret is MCP_GATEWAY_SECRET: with it, X-SocialOS-Client-IP from the MCP server is believed. Empty = never.
	GatewaySecret string
	Logger        *slog.Logger
	Metrics       *observability.Metrics
	Ready         []ReadyCheck
	APILimiter    *middleware.Limiter
	AuthLimiter   *middleware.Limiter
	// TelegramWebhookSecret enables POST /webhooks/telegram (webhook intake
	// mode); with an empty secret the route does not exist.
	TelegramWebhookSecret string
}

// API holds handler dependencies.
type API struct {
	svc     Services
	opt     Options
	trusted middleware.TrustedProxies
}

// NewRouter builds the HTTP handler.
func NewRouter(svc Services, opt Options) http.Handler {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.Metrics == nil {
		opt.Metrics = observability.NewMetrics()
	}
	if opt.APILimiter == nil {
		opt.APILimiter = middleware.NewLimiter(10, 40)
	}
	if opt.AuthLimiter == nil {
		opt.AuthLimiter = middleware.NewLimiter(0.2, 10)
	}
	a := &API{svc: svc, opt: opt, trusted: middleware.TrustedProxies(opt.TrustedProxies)}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recover, middleware.AccessLog(opt.Logger, opt.Metrics), middleware.SecurityHeaders,
		middleware.CORS(opt.CORSOrigins))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { httpx.ErrorCode(w, r, errs.NotFound, "route not found") })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.ErrorCode(w, r, errs.NotFound, "method not allowed on this route")
	})
	a.mountOps(r)
	r.Route("/api/v1", func(r chi.Router) {
		a.mountOps(r)
		if opt.TelegramWebhookSecret != "" {
			// Machine-to-machine: authenticated by the shared secret header only, so it sits
			// outside the cookie/CSRF/per-client rate-limit group below.
			r.Post("/webhooks/telegram", a.telegramWebhook)
		}
		r.Group(func(r chi.Router) {
			r.Use(middleware.Gateway(opt.GatewaySecret), middleware.Authenticate(svc.Auth, a.trusted), middleware.APIKeyAudit(svc.Audit, opt.Logger),
				middleware.RateLimit(opt.APILimiter, a.trusted, opt.Metrics, "api:"), middleware.CSRF)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RateLimit(opt.AuthLimiter, a.trusted, opt.Metrics, "auth:"))
				r.Post("/auth/register", a.register)
				r.Post("/auth/login", a.login)
			})
			r.Get("/social/{provider}/callback", a.callback)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAuth)
				a.mountAuthenticated(r)
			})
		})
	})
	return r
}

func (a *API) mountOps(r chi.Router) {
	r.Get("/health", a.health)
	r.Get("/ready", a.ready)
	r.Handle("/metrics", a.metricsHandler())
}

func (a *API) mountAuthenticated(r chi.Router) {
	r.Post("/auth/logout", a.logout)
	r.Get("/me", a.me)

	r.Get("/social/providers", a.listProviders)
	r.Get("/social/accounts", a.listAccounts)
	r.Get("/social/accounts/{id}", a.getAccount)
	r.Delete("/social/accounts/{id}", a.disconnectAccount)
	r.With(middleware.RateLimit(a.opt.AuthLimiter, a.trusted, a.opt.Metrics, "link:")).
		Post("/social/telegram/connect", a.startTelegramLink)
	r.Get("/social/telegram/connect/{id}", a.telegramLinkStatus)
	r.Get("/social/{provider}/connect", a.connect)

	r.Post("/posts", a.createPost)
	r.Get("/posts", a.listPosts)
	r.Get("/posts/{id}", a.getPost)
	r.Patch("/posts/{id}", a.updatePost)
	r.Delete("/posts/{id}", a.deletePost)
	r.Get("/posts/{id}/status", a.postStatus)
	r.Post("/posts/{id}/publish", a.publishPost)
	r.Post("/posts/{id}/schedule", a.schedulePost)
	r.Post("/posts/{id}/unschedule", a.unschedulePost)
	r.Post("/posts/{id}/cancel", a.cancelPost)
	r.Post("/posts/{id}/retry", a.retryPost)

	r.Post("/media", a.uploadMedia)
	r.Get("/media", a.listMedia)
	r.Get("/media/{id}", a.getMedia)
	r.Delete("/media/{id}", a.deleteMedia)

	r.Get("/analytics", a.analytics)
	r.Get("/dashboard/summary", a.dashboard)
	r.Get("/audit-logs", a.auditLogs)

	r.Get("/developer/api-keys", a.listKeys)
	r.Post("/developer/api-keys", a.createKey)
	r.Delete("/developer/api-keys/{id}", a.revokeKey)
	r.Get("/developer/mcp-connections", a.listMCP)
	r.Post("/developer/mcp-connections", a.createMCP)
	r.Delete("/developer/mcp-connections/{id}", a.revokeMCP)
	r.Get("/developer/usage", a.usage)
}
