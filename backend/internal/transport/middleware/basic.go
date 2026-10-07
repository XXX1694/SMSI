// Package middleware contains HTTP middleware: request id, logging, recovery,
// security headers, authentication, CSRF, rate limiting, CORS and API-key audit.
package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/observability"
	"github.com/socialos/backend/internal/transport/httpx"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// RequestID accepts a sane inbound X-Request-ID / X-Correlation-ID or generates one.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = r.Header.Get("X-Correlation-ID")
		}
		if !validRequestID.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Correlation-ID", id)
		ctx := httpx.WithRequestID(r.Context(), id)
		ctx = observability.WithAttrs(ctx, slog.String("request_id", id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recover turns panics into a 500 error envelope.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				slog.ErrorContext(r.Context(), "panic", slog.Any("panic", p), slog.String("stack", string(debug.Stack())))
				httpx.ErrorCode(w, r, errs.Internal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets conservative API response headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return "unmatched"
}

// actorSlot lets inner middleware (Authenticate) tell the outer access log who
// made the request: request contexts only flow inwards.
type actorSlot struct {
	mu sync.Mutex
	a  *actor.Actor
}

type actorSlotKey struct{}

func (s *actorSlot) set(a actor.Actor) {
	s.mu.Lock()
	s.a = &a
	s.mu.Unlock()
}

func (s *actorSlot) get() (actor.Actor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a == nil {
		return actor.Actor{}, false
	}
	return *s.a, true
}

func publishActor(ctx context.Context, a actor.Actor) {
	if s, ok := ctx.Value(actorSlotKey{}).(*actorSlot); ok {
		s.set(a)
	}
}

// AccessLog logs one line per request (never headers or bodies) and records metrics.
func AccessLog(log *slog.Logger, m *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			slot := &actorSlot{}
			next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), actorSlotKey{}, slot)))
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			route := routePattern(r)
			dur := time.Since(start)
			if m != nil {
				m.HTTPRequests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
				m.HTTPDuration.WithLabelValues(route, r.Method).Observe(dur.Seconds())
			}
			attrs := []any{slog.String("method", r.Method), slog.String("route", route), slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes), slog.Duration("duration", dur)}
			if a, ok := slot.get(); ok {
				attrs = append(attrs, slog.String("actor_type", string(a.Type)), slog.String("user_id", a.UserID.String()))
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			log.Log(r.Context(), level, "http request", attrs...)
		})
	}
}

// ClientIP extracts the caller IP; X-Forwarded-For is honoured only behind a trusted proxy.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
