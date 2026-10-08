package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/observability"
	"github.com/socialos/backend/internal/transport/httpx"
)

// Limiter is an in-memory token bucket per key (per API instance).
type Limiter struct {
	rps   rate.Limit
	burst int
	mu    sync.Mutex
	keys  map[string]*bucket
	idle  time.Duration
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// NewLimiter creates a limiter with rps tokens/second and burst capacity.
func NewLimiter(rps float64, burst int) *Limiter {
	return &Limiter{rps: rate.Limit(rps), burst: burst, keys: map[string]*bucket{}, idle: 10 * time.Minute}
}

// Allow consumes a token for key; it returns the suggested retry delay when denied.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	b, ok := l.keys[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.rps, l.burst)}
		l.keys[key] = b
	}
	b.seen = now
	l.mu.Unlock()
	r := b.lim.ReserveN(now, 1)
	if !r.OK() {
		return false, time.Minute
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

// Sweep drops idle buckets; call periodically.
func (l *Limiter) Sweep() {
	cutoff := time.Now().Add(-l.idle)
	l.mu.Lock()
	for k, b := range l.keys {
		if b.seen.Before(cutoff) {
			delete(l.keys, k)
		}
	}
	l.mu.Unlock()
}

// RateLimit applies the limiter keyed by actor (user/API key) or, for anonymous callers, the client IP as resolved by
// ClientIP (so a forged X-Forwarded-For cannot pick the bucket).
func RateLimit(l *Limiter, trusted TrustedProxies, m *observability.Metrics, prefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := prefix + "ip:" + ClientIP(r, trusted)
			if a, ok := actor.From(r.Context()); ok {
				key = prefix + string(a.Type) + ":" + a.ID
			}
			if ok, wait := l.Allow(key); !ok {
				if m != nil {
					m.RateLimited.Inc()
				}
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
				httpx.ErrorCode(w, r, errs.RateLimited, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
