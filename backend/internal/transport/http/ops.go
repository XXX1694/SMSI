package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/socialos/backend/internal/buildinfo"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/transport/httpx"
)

// ReadyCheck is a dependency probe for /ready.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// version reports the running build (tag version, short commit, build time). It is public and constant for the life of
// the process, so it may be cached briefly; the short max-age keeps a fresh deploy visible within a minute.
func (a *API) version(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.JSON(w, http.StatusOK, buildinfo.Get())
}

// ready probes every dependency. Per-check status is "ok" or "unavailable";
// `errors` carries a coarse, non-sensitive reason for each failing check
// (the full error is logged).
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	results := make(map[string]string, len(a.opt.Ready))
	reasons := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	healthy := true
	for _, c := range a.opt.Ready {
		wg.Add(1)
		go func(c ReadyCheck) {
			defer wg.Done()
			status, reason := "ok", ""
			if err := c.Check(ctx); err != nil {
				status, reason = "unavailable", reasonOf(err)
				a.opt.Logger.WarnContext(r.Context(), "readiness check failed", slog.String("check", c.Name), slog.Any("error", err))
			}
			mu.Lock()
			results[c.Name] = status
			if reason != "" {
				reasons[c.Name] = reason
			}
			healthy = healthy && status == "ok"
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	code, overall := http.StatusOK, "ok"
	if !healthy {
		code, overall = http.StatusServiceUnavailable, "unavailable"
	}
	body := map[string]any{"status": overall, "checks": results}
	if len(reasons) > 0 {
		body["errors"] = reasons
	}
	httpx.JSON(w, code, body)
}

// reasonOf maps a probe error to a coarse reason that is safe to expose.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out (dependency unreachable or slow)"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "unreachable or misconfigured"
	}
}

func (a *API) metricsHandler() http.Handler {
	h := promhttp.HandlerFor(a.opt.Metrics.Registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.opt.MetricsToken != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !crypto.ConstantTimeEqual(got, a.opt.MetricsToken) {
				httpx.ErrorCode(w, r, errs.Unauthenticated, "metrics token required")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
