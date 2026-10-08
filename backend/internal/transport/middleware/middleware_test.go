package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/observability"
	"github.com/socialos/backend/internal/transport/middleware"
)

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

func do(h http.Handler, method, path string, mod ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	for _, m := range mod {
		m(r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error envelope (%d): %s", rec.Code, rec.Body)
	}
	return body.Error.Code
}

func TestRequestID(t *testing.T) {
	var seen string
	h := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Request-ID")
		ok(w, r)
	}))
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	rec := do(h, "GET", "/")
	gen := rec.Header().Get("X-Request-ID")
	if !uuidRe.MatchString(gen) || rec.Header().Get("X-Correlation-ID") != gen {
		t.Fatalf("generated id %q / %q", gen, rec.Header().Get("X-Correlation-ID"))
	}
	_ = seen
	for _, tc := range []struct{ header, in string }{
		{"X-Request-ID", "trace-123_ABC.def:9"}, {"X-Correlation-ID", "corr-1"}, {"X-Request-ID", strings.Repeat("a", 64)},
	} {
		rec := do(h, "GET", "/", func(r *http.Request) { r.Header.Set(tc.header, tc.in) })
		if rec.Header().Get("X-Request-ID") != tc.in || rec.Header().Get("X-Correlation-ID") != tc.in {
			t.Errorf("%s %q not echoed: %q", tc.header, tc.in, rec.Header().Get("X-Request-ID"))
		}
	}
	// X-Request-ID wins over X-Correlation-ID.
	rec = do(h, "GET", "/", func(r *http.Request) {
		r.Header.Set("X-Request-ID", "first")
		r.Header.Set("X-Correlation-ID", "second")
	})
	if rec.Header().Get("X-Request-ID") != "first" {
		t.Errorf("precedence: %q", rec.Header().Get("X-Request-ID"))
	}
	for _, bad := range []string{"has space", "semi;colon", "<script>", strings.Repeat("a", 65), "unicodé", "a/b", "a,b"} {
		rec := do(h, "GET", "/", func(r *http.Request) { r.Header.Set("X-Request-ID", bad) })
		if got := rec.Header().Get("X-Request-ID"); got == bad || !uuidRe.MatchString(got) {
			t.Errorf("unsafe id %q must be replaced by a uuid, got %q", bad, got)
		}
	}
}

func TestRecover(t *testing.T) {
	h := middleware.RequestID(middleware.Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("secret internals: db password") })))
	rec := do(h, "GET", "/boom")
	if rec.Code != 500 || errCode(t, rec) != "INTERNAL" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "secret internals") || strings.Contains(rec.Body.String(), "goroutine") {
		t.Fatalf("panic details leaked: %s", rec.Body)
	}
	// http.ErrAbortHandler is how handlers abort a response on purpose; it must keep propagating.
	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Fatal("ErrAbortHandler must be re-panicked")
		}
	}()
	do(middleware.Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })), "GET", "/")
}

func TestSecurityHeaders(t *testing.T) {
	rec := do(middleware.SecurityHeaders(http.HandlerFunc(ok)), "GET", "/")
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q", k, got)
		}
	}
}

// ---------------------------------------------------------------- authentication

type fakeAuth struct {
	keyCalls, sessCalls atomic.Int32
	keyActor            actor.Actor
	keyErr, sessErr     error
	csrf                string
	lastInfo            auth.ClientInfo
}

func (f *fakeAuth) AuthenticateSession(_ context.Context, raw string, ci auth.ClientInfo) (actor.Actor, string, error) {
	f.sessCalls.Add(1)
	f.lastInfo = ci
	if f.sessErr != nil || raw != "good-session" {
		if f.sessErr != nil {
			return actor.Actor{}, "", f.sessErr
		}
		return actor.Actor{}, "", errs.New(errs.Unauthenticated, "invalid session")
	}
	return actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}, f.csrf, nil
}

func (f *fakeAuth) AuthenticateAPIKey(_ context.Context, raw string, ci auth.ClientInfo) (actor.Actor, error) {
	f.keyCalls.Add(1)
	f.lastInfo = ci
	if f.keyErr != nil {
		return actor.Actor{}, f.keyErr
	}
	if raw != "sk_live_good" {
		return actor.Actor{}, errs.New(errs.Unauthenticated, "invalid api key")
	}
	return f.keyActor, nil
}

func whoami(w http.ResponseWriter, r *http.Request) {
	a, found := actor.From(r.Context())
	_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": found, "type": a.Type, "csrf": middleware.CSRFToken(r.Context())})
}

func TestAuthenticate(t *testing.T) {
	f := &fakeAuth{csrf: "csrf-1", keyActor: actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k"}}
	h := middleware.RequestID(middleware.Authenticate(f, nil)(http.HandlerFunc(whoami)))
	type who struct {
		Authenticated bool
		Type, CSRF    string
	}
	get := func(mod ...func(*http.Request)) (*httptest.ResponseRecorder, who) {
		rec := do(h, "GET", "/", mod...)
		var w who
		_ = json.Unmarshal(rec.Body.Bytes(), &struct {
			A *bool   `json:"authenticated"`
			T *string `json:"type"`
			C *string `json:"csrf"`
		}{&w.Authenticated, &w.Type, &w.CSRF})
		return rec, w
	}
	bearer := func(v string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("Authorization", v) } }
	cookie := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: v}) }
	}

	if rec, w := get(); rec.Code != 200 || w.Authenticated {
		t.Fatalf("anonymous must pass through unauthenticated: %d %+v", rec.Code, w)
	}
	if rec, w := get(bearer("Bearer sk_live_good")); rec.Code != 200 || !w.Authenticated || w.Type != "api_key" || w.CSRF != "" {
		t.Fatalf("api key: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := get(bearer("bEaReR sk_live_good")); rec.Code != 200 {
		t.Fatalf("scheme is case-insensitive: %d", rec.Code)
	}
	for _, v := range []string{"Bearer", "Bearer ", "Basic abc", "sk_live_good", "Token sk_live_good"} {
		rec, _ := get(bearer(v))
		if rec.Code != 401 || errCode(t, rec) != "UNAUTHENTICATED" {
			t.Errorf("Authorization %q: %d %s", v, rec.Code, rec.Body)
		}
	}
	if rec, _ := get(bearer("Bearer sk_live_wrong")); rec.Code != 401 {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	if rec, w := get(cookie("good-session")); rec.Code != 200 || w.Type != "user" || w.CSRF != "csrf-1" {
		t.Fatalf("session: %d %s", rec.Code, rec.Body)
	}
	// An invalid cookie is anonymous, not an error (so public routes still work for logged-out browsers).
	if rec, w := get(cookie("stale")); rec.Code != 200 || w.Authenticated {
		t.Fatalf("stale cookie: %d %s", rec.Code, rec.Body)
	}
	// A bad bearer is an error even when a valid cookie accompanies it, and the cookie is never consulted.
	before := f.sessCalls.Load()
	if rec, _ := get(bearer("Bearer sk_live_wrong"), cookie("good-session")); rec.Code != 401 || f.sessCalls.Load() != before {
		t.Fatalf("bearer must take precedence over the cookie: %d (session calls %d -> %d)", rec.Code, before, f.sessCalls.Load())
	}
	if rec, w := get(bearer("Bearer sk_live_good"), cookie("good-session")); w.Type != "api_key" || rec.Code != 200 {
		t.Fatalf("valid bearer plus cookie: %s", rec.Body)
	}
	// Infrastructure failures are reported, not treated as "anonymous".
	f.sessErr = errs.New(errs.Internal, "db down")
	if rec, _ := get(cookie("good-session")); rec.Code != 500 {
		t.Fatalf("session backend failure: %d", rec.Code)
	}
	f.keyErr = errs.New(errs.Internal, "db down")
	if rec, _ := get(bearer("Bearer sk_live_good")); rec.Code != 500 {
		t.Fatalf("key backend failure: %d", rec.Code)
	}
	f.keyErr = errs.New(errs.Forbidden, "key revoked")
	if rec, _ := get(bearer("Bearer sk_live_good")); rec.Code != 403 {
		t.Fatalf("typed errors keep their status: %d", rec.Code)
	}
}

func TestAuthenticatePassesClientInfo(t *testing.T) {
	f := &fakeAuth{}
	h := middleware.RequestID(middleware.Authenticate(f, privateProxies)(http.HandlerFunc(whoami)))
	do(h, "GET", "/", func(r *http.Request) {
		r.RemoteAddr = "10.0.0.1:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
		r.Header.Set("User-Agent", "curl/8")
		r.Header.Set("X-Request-ID", "rid-9")
		r.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: "good-session"})
	})
	if f.lastInfo.IP != "203.0.113.7" || f.lastInfo.UserAgent != "curl/8" || f.lastInfo.RequestID != "rid-9" {
		t.Fatalf("%+v", f.lastInfo)
	}
}

func TestRequireAuth(t *testing.T) {
	h := middleware.RequestID(middleware.RequireAuth(http.HandlerFunc(ok)))
	rec := do(h, "GET", "/")
	if rec.Code != 401 || errCode(t, rec) != "UNAUTHENTICATED" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = do(h, "GET", "/", func(r *http.Request) {
		*r = *r.WithContext(actor.With(r.Context(), actor.Actor{UserID: uuid.New(), Type: actor.TypeUser}))
	})
	if rec.Code != 204 {
		t.Fatalf("authenticated: %d", rec.Code)
	}
}

func TestCSRF(t *testing.T) {
	session := actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}
	key := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey}
	f := &fakeAuth{csrf: "tok-123", keyActor: key}
	h := middleware.RequestID(middleware.Authenticate(f, nil)(middleware.CSRF(http.HandlerFunc(ok))))
	_ = session
	withSession := func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: "good-session"})
	}
	hdr := func(v string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set(middleware.CSRFHeader, v) }
	}

	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		if rec := do(h, m, "/", withSession); rec.Code != 204 {
			t.Errorf("%s is safe and needs no token: %d", m, rec.Code)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if rec := do(h, m, "/", withSession); rec.Code != 403 || errCode(t, rec) != "FORBIDDEN" {
			t.Errorf("%s without a token: %d %s", m, rec.Code, rec.Body)
		}
		if rec := do(h, m, "/", withSession, hdr("wrong")); rec.Code != 403 {
			t.Errorf("%s with a wrong token: %d", m, rec.Code)
		}
		if rec := do(h, m, "/", withSession, hdr("tok-12")); rec.Code != 403 {
			t.Errorf("%s with a prefix of the token: %d", m, rec.Code)
		}
		if rec := do(h, m, "/", withSession, hdr("tok-123")); rec.Code != 204 {
			t.Errorf("%s with the right token: %d", m, rec.Code)
		}
	}
	// No credentials at all: CSRF is not the layer that rejects (auth is).
	if rec := do(h, "POST", "/"); rec.Code != 204 {
		t.Errorf("anonymous POST passes to the next layer: %d", rec.Code)
	}
	// Bearer keys are immune.
	if rec := do(h, "POST", "/", func(r *http.Request) { r.Header.Set("Authorization", "Bearer sk_live_good") }); rec.Code != 204 {
		t.Errorf("api key POST: %d", rec.Code)
	}
	// A session whose stored CSRF token is empty can never pass, not even with an empty header.
	f.csrf = ""
	if rec := do(h, "POST", "/", withSession); rec.Code != 403 {
		t.Errorf("empty server token: %d", rec.Code)
	}
}

// ---------------------------------------------------------------- rate limiting

func TestLimiterBurstRefillAndSweep(t *testing.T) {
	l := middleware.NewLimiter(1000, 3)
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d within the burst was denied", i)
		}
	}
	allowed, wait := l.Allow("a")
	if allowed || wait <= 0 {
		t.Fatalf("burst exhausted: allowed=%v wait=%v", allowed, wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("keys are independent")
	}
	time.Sleep(10 * time.Millisecond) // 1000 rps refills quickly
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("tokens must refill")
	}
	slow := middleware.NewLimiter(0.001, 1)
	slow.Allow("k")
	if ok, wait := slow.Allow("k"); ok || wait < time.Minute {
		t.Fatalf("slow limiter: %v %v", ok, wait)
	}
	l.Sweep() // must not drop recently used buckets or panic
	if ok, _ := l.Allow("fresh"); !ok {
		t.Fatal("sweep broke the limiter")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	m := observability.NewMetrics()
	l := middleware.NewLimiter(0.001, 2)
	h := middleware.RequestID(middleware.RateLimit(l, nil, m, "t:")(http.HandlerFunc(ok)))
	from := func(ip string) func(*http.Request) { return func(r *http.Request) { r.RemoteAddr = ip + ":1234" } }

	for i := 0; i < 2; i++ {
		if rec := do(h, "GET", "/", from("192.0.2.1")); rec.Code != 204 {
			t.Fatalf("within burst: %d", rec.Code)
		}
	}
	rec := do(h, "GET", "/", from("192.0.2.1"))
	if rec.Code != 429 || errCode(t, rec) != "RATE_LIMITED" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After %q", ra)
	}
	if rec := do(h, "GET", "/", from("192.0.2.2")); rec.Code != 204 {
		t.Fatalf("another client is unaffected: %d", rec.Code)
	}
	// X-Forwarded-For is ignored unless the proxy is trusted: it cannot be used to dodge the limit.
	for i := 0; i < 5; i++ {
		rec := do(h, "GET", "/", from("192.0.2.1"), func(r *http.Request) { r.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('1'+i))) })
		if rec.Code != 429 {
			t.Fatalf("spoofed XFF bypassed the limit: %d", rec.Code)
		}
	}
	// Authenticated callers are limited per actor, whatever their IP.
	userA := actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, ID: "user-a"}
	asA := func(ip string) func(*http.Request) {
		return func(r *http.Request) {
			r.RemoteAddr = ip + ":1"
			*r = *r.WithContext(actor.With(r.Context(), userA))
		}
	}
	codes := []int{}
	for i := 0; i < 4; i++ {
		codes = append(codes, do(h, "GET", "/", asA("203.0.113."+string(rune('1'+i)))).Code)
	}
	if codes[0] != 204 || codes[1] != 204 || codes[2] != 429 || codes[3] != 429 {
		t.Fatalf("per-actor limiting across IPs: %v", codes)
	}
	if testing.Verbose() {
		t.Logf("codes %v", codes)
	}
	// The counter is exposed.
	fams, _ := m.Registry.Gather()
	var total float64
	for _, f := range fams {
		if f.GetName() == "socialos_http_rate_limited_total" {
			total = f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	if total != 8 {
		t.Fatalf("rate limited counter %v, want 8", total)
	}
	// Different prefixes are different buckets (auth vs api limiter).
	other := middleware.RequestID(middleware.RateLimit(l, nil, nil, "other:")(http.HandlerFunc(ok)))
	if rec := do(other, "GET", "/", from("192.0.2.1")); rec.Code != 204 {
		t.Fatalf("prefix isolation: %d", rec.Code)
	}
}

// ---------------------------------------------------------------- logging, metrics, CORS

func TestAccessLogRecordsRouteTemplatesNotRawPaths(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	m := observability.NewMetrics()
	r := chi.NewRouter()
	r.Use(middleware.AccessLog(log, m))
	r.Get("/posts/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	r.Get("/boom", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "x", 500) })
	r.Get("/implicit", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) })

	for _, p := range []string{"/posts/aaa", "/posts/bbb", "/boom", "/implicit", "/nothing"} {
		do(r, "GET", p, func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer sk_live_topsecret")
			req.Header.Set("Cookie", "socialos_session=sessionsecret")
		})
	}
	fams, _ := m.Registry.Gather()
	got := map[string]float64{}
	for _, f := range fams {
		if f.GetName() != "socialos_http_requests_total" {
			continue
		}
		for _, mt := range f.GetMetric() {
			lbl := map[string]string{}
			for _, l := range mt.GetLabel() {
				lbl[l.GetName()] = l.GetValue()
			}
			got[lbl["route"]+" "+lbl["method"]+" "+lbl["status"]] = mt.GetCounter().GetValue()
		}
	}
	for k, want := range map[string]float64{"/posts/{id} GET 418": 2, "/boom GET 500": 1, "/implicit GET 200": 1, "unmatched GET 404": 1} {
		if got[k] != want {
			t.Errorf("series %q = %v, want %v (have %v)", k, got[k], want, got)
		}
	}
	out := logs.String()
	for _, secret := range []string{"sk_live_topsecret", "sessionsecret", "/posts/aaa"} {
		if strings.Contains(out, secret) {
			t.Errorf("access log contains %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, `"route":"/posts/{id}"`) || !strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("access log content/levels: %s", out)
	}
}

func TestAccessLogIncludesTheAuthenticatedActor(t *testing.T) {
	var logs bytes.Buffer
	f := &fakeAuth{csrf: "c", keyActor: actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k"}}
	// Same order as the real router: the access log is outermost, authentication is inside it.
	h := middleware.AccessLog(slog.New(slog.NewJSONHandler(&logs, nil)), nil)(middleware.Authenticate(f, nil)(http.HandlerFunc(ok)))
	do(h, "GET", "/", func(r *http.Request) { r.Header.Set("Authorization", "Bearer sk_live_good") })
	do(h, "GET", "/", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: "good-session"})
	})
	do(h, "GET", "/")
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("one line per request expected: %s", logs.String())
	}
	if !strings.Contains(lines[0], `"actor_type":"api_key"`) || !strings.Contains(lines[0], f.keyActor.UserID.String()) {
		t.Errorf("api key request not attributed: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"actor_type":"user"`) {
		t.Errorf("session request not attributed: %s", lines[1])
	}
	if strings.Contains(lines[2], "actor_type") || strings.Contains(lines[2], "user_id") {
		t.Errorf("anonymous request must not carry an actor: %s", lines[2])
	}
	if strings.Contains(logs.String(), "sk_live_good") || strings.Contains(logs.String(), "good-session") {
		t.Errorf("credentials leaked into the access log: %s", logs.String())
	}
}

type auditEntry struct {
	actor  actor.Actor
	action string
	meta   map[string]any
	ctxErr error
}

type memAudit struct {
	entries []auditEntry
	err     error
}

func (m *memAudit) Record(ctx context.Context, a actor.Actor, action, _, _ string, meta map[string]any) error {
	m.entries = append(m.entries, auditEntry{a, action, meta, ctx.Err()})
	return m.err
}

func TestAPIKeyAudit(t *testing.T) {
	keyActor := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k1", APIKeyID: uuid.New(), Label: "agent"}
	f := &fakeAuth{csrf: "c", keyActor: keyActor}
	rec := &memAudit{}
	r := chi.NewRouter()
	r.Use(middleware.Authenticate(f, nil), middleware.APIKeyAudit(rec, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))))
	r.Post("/posts/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	r.Get("/missing", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope", http.StatusNotFound) })
	bearer := func(req *http.Request) { req.Header.Set("Authorization", "Bearer sk_live_good") }

	do(r, "POST", "/posts/1", bearer)
	do(r, "GET", "/missing", bearer)
	do(r, "GET", "/missing") // anonymous
	do(r, "GET", "/missing", func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: "good-session"})
	})
	if len(rec.entries) != 2 {
		t.Fatalf("only API key requests are audited, got %d entries", len(rec.entries))
	}
	e := rec.entries[0]
	if e.action != "api_key.request" || e.actor.APIKeyID != keyActor.APIKeyID || e.actor.Label != "agent" ||
		e.meta["method"] != "POST" || e.meta["route"] != "/posts/{id}" || e.meta["status"] != http.StatusCreated {
		t.Fatalf("entry: %+v", e)
	}
	if rec.entries[1].meta["status"] != http.StatusNotFound || rec.entries[1].meta["route"] != "/missing" {
		t.Fatalf("failed requests are audited with their status: %+v", rec.entries[1])
	}
	// The raw path (which may hold ids) is never stored, only the route template.
	if strings.Contains(string(mustJSON(rec.entries[0].meta)), "/posts/1") {
		t.Fatalf("raw path stored: %v", rec.entries[0].meta)
	}
	// The write happens even when the client has already gone away.
	rec.entries = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/missing", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer sk_live_good")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if len(rec.entries) != 1 || rec.entries[0].ctxErr != nil {
		t.Fatalf("audit must survive request cancellation: %+v", rec.entries)
	}
	// A failing recorder never breaks the response.
	rec.err = errors.New("audit db down")
	if out := do(r, "GET", "/missing", bearer); out.Code != 404 {
		t.Fatalf("response changed by an audit failure: %d", out.Code)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestCORS(t *testing.T) {
	h := middleware.CORS([]string{"https://app.example.com"})(http.HandlerFunc(ok))
	pre := func(origin string) *httptest.ResponseRecorder {
		return do(h, "OPTIONS", "/api/v1/posts", func(r *http.Request) {
			r.Header.Set("Origin", origin)
			r.Header.Set("Access-Control-Request-Method", "DELETE")
			r.Header.Set("Access-Control-Request-Headers", "X-CSRF-Token, Content-Type")
		})
	}
	rec := pre("https://app.example.com")
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed origin: %v", rec.Header())
	}
	if !strings.Contains(strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers")), "x-csrf-token") ||
		!strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Fatalf("preflight headers: %v", rec.Header())
	}
	for _, evil := range []string{"https://evil.example", "https://app.example.com.evil.example", "http://app.example.com", "null"} {
		if got := pre(evil).Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%q allowed as %q", evil, got)
		}
	}
	for name, list := range map[string][]string{"nil": nil, "empty entries": {"", "  "}, "wildcard": {"*"}} {
		rec := do(middleware.CORS(list)(http.HandlerFunc(ok)), "GET", "/", func(r *http.Request) { r.Header.Set("Origin", "https://x.test") })
		if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("%s allow-list must allow nothing: %v", name, rec.Header())
		}
	}
}
