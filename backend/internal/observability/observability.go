// Package observability provides the structured logger and Prometheus metrics.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// sensitiveKeys are attribute names whose values are always redacted.
var sensitiveKeys = []string{"token", "secret", "password", "authorization", "cookie", "api_key", "apikey", "code_verifier"}

// NewLogger builds a slog logger that redacts sensitive attributes.
func NewLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: redact}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(contextHandler{h})
}

func redact(_ []string, a slog.Attr) slog.Attr {
	k := strings.ToLower(a.Key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return slog.String(a.Key, "[REDACTED]")
		}
	}
	return a
}

type ctxKey struct{}

// WithAttrs attaches log attributes (request id, actor) to ctx.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	merged := append(append([]slog.Attr{}, prev...), attrs...)
	return context.WithValue(ctx, ctxKey{}, merged)
}

// contextHandler adds ctx attributes to every record.
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

// Metrics holds Prometheus collectors.
type Metrics struct {
	Registry     *prometheus.Registry
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
	RateLimited  prometheus.Counter
	// PublishOutcomes counts settled publish attempts by provider and outcome
	// (published | failed | retry | needs_review).
	PublishOutcomes *prometheus.CounterVec
}

// NewMetrics registers default collectors.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "socialos_http_requests_total", Help: "HTTP requests by route, method and status.",
		}, []string{"route", "method", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "socialos_http_request_duration_seconds", Help: "HTTP request latency.", Buckets: prometheus.DefBuckets,
		}, []string{"route", "method"}),
		RateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "socialos_http_rate_limited_total", Help: "Requests rejected by rate limiting.",
		}),
		PublishOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "socialos_publish_attempts_total", Help: "Settled publish attempts by provider and outcome.",
		}, []string{"provider", "outcome"}),
	}
	reg.MustRegister(m.HTTPRequests, m.HTTPDuration, m.RateLimited, m.PublishOutcomes,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}
