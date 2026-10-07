package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/cors"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
)

// APIKeyAudit writes an audit entry for every request made with an API key.
func APIKeyAudit(rec port.AuditRecorder, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sr := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(sr, r)
			a, ok := actor.From(r.Context())
			if !ok || a.Type != actor.TypeAPIKey {
				return
			}
			if sr.status == 0 {
				sr.status = http.StatusOK
			}
			meta := map[string]any{"method": r.Method, "route": routePattern(r), "status": sr.status}
			if err := rec.Record(context.WithoutCancel(r.Context()), a, audit.ActionAPIRequest, "api_key", a.APIKeyID.String(), meta); err != nil {
				log.WarnContext(r.Context(), "api key audit failed", slog.Any("error", err))
			}
		})
	}
}

// CORS allows credentialed requests only from the configured origins.
func CORS(origins []string) func(http.Handler) http.Handler {
	var allowed []string
	for _, o := range origins {
		if o = strings.TrimSpace(o); o != "" && o != "*" { // never a wildcard together with credentials
			allowed = append(allowed, o)
		}
	}
	if len(allowed) == 0 { // go-chi/cors would otherwise default to "*"
		return func(next http.Handler) http.Handler { return next }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowed,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", CSRFHeader, "X-Request-ID", "X-Correlation-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           600,
	})
}
