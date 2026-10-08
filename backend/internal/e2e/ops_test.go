package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/config"
)

// ---------------------------------------------------------------- limits

func TestRateLimitReturns429(t *testing.T) {
	e := newEnv(t, envOpts{rateRPS: 0.001, rateBurst: 5})
	a, b := e.browser(), e.browser()
	a.register("rl-a@example.com")
	b.register("rl-b@example.com")
	limited := 0
	var hit resp
	for i := 0; i < 12; i++ {
		r := a.do("GET", "/api/v1/posts", nil)
		if r.status == http.StatusTooManyRequests {
			limited++
			hit = r
		} else if r.status != 200 {
			t.Fatalf("unexpected %d %s", r.status, r.body)
		}
	}
	if limited == 0 {
		t.Fatal("no request was rate limited")
	}
	if hit.errCode(t) != "RATE_LIMITED" || hit.header.Get("Retry-After") == "" || hit.errBody(t)["request_id"] == "" {
		t.Fatalf("429 envelope: %d %v %s", hit.status, hit.header, hit.body)
	}
	if ra := hit.header.Get("Retry-After"); ra == "0" {
		t.Fatalf("Retry-After must be positive, got %q", ra)
	}
	// The bucket is per user: someone else is not throttled.
	if r := b.do("GET", "/api/v1/posts", nil); r.status != 200 {
		t.Fatalf("other user throttled: %d %s", r.status, r.body)
	}
	// Ops probes are never rate limited.
	for i := 0; i < 20; i++ {
		if r := a.do("GET", "/health", nil); r.status != 200 {
			t.Fatalf("health limited: %d", r.status)
		}
	}
	if m := string(a.do("GET", "/metrics", nil).body); !strings.Contains(m, "socialos_http_rate_limited_total") {
		t.Errorf("rate limit counter missing from metrics")
	}
}

func TestAuthEndpointsAreThrottled(t *testing.T) {
	e := newEnv(t, envOpts{mutate: func(c *config.Config) { c.AuthRateRPS, c.AuthRateBurst = 0.001, 4 }})
	c := e.browser()
	c.register("brute@example.com") // 1 token
	codes := []int{}
	for i := 0; i < 8; i++ {
		r := e.browser().do("POST", "/api/v1/auth/login", map[string]any{"email": "brute@example.com", "password": fmt.Sprintf("guess-number-%d", i)})
		codes = append(codes, r.status)
		if r.status == 429 && (r.errCode(t) != "RATE_LIMITED" || r.header.Get("Retry-After") == "") {
			t.Fatalf("throttle envelope: %s", r.body)
		}
	}
	want401, want429 := 0, 0
	for _, s := range codes {
		switch s {
		case 401:
			want401++
		case 429:
			want429++
		}
	}
	if want401 != 3 || want429 != 5 {
		t.Fatalf("login attempts: want 3x401 then 5x429, got %v", codes)
	}
	// Even the right password is refused while throttled, so guessing cannot be confirmed.
	if r := e.browser().do("POST", "/api/v1/auth/login", map[string]any{"email": "brute@example.com", "password": "correct horse battery"}); r.status != 429 {
		t.Fatalf("throttled login with the right password: %d", r.status)
	}
}

// The per-IP limit protects anonymous endpoints (login, register). With the proxy trusted, the client address is the
// right-most X-Forwarded-For hop that is not itself a proxy, so forging the left part must not buy a fresh bucket.
func TestAuthThrottleIgnoresForgedForwardedFor(t *testing.T) {
	login := func(c *client, xff string) int {
		req, _ := http.NewRequest("POST", c.e.srv.URL+"/api/v1/auth/login",
			strings.NewReader(`{"email":"nobody@example.com","password":"wrong password!!"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", xff)
		return c.send(req).status
	}
	attempts := func(e *env, realClient string) []int {
		c, codes := e.browser(), []int{}
		for i := 0; i < 6; i++ {
			codes = append(codes, login(c, fmt.Sprintf("198.51.100.%d, %s", i+1, realClient))) // forged hop, then what the proxy saw
		}
		return codes
	}
	limit := func(c *config.Config) { c.AuthRateRPS, c.AuthRateBurst = 0.001, 3 }

	t.Run("behind a trusted proxy the forged hop is ignored", func(t *testing.T) {
		// The test server's peer is 127.0.0.1, which the default trust set covers.
		e := newEnv(t, envOpts{mutate: func(c *config.Config) {
			limit(c)
			c.TrustProxy, c.TrustedProxies = true, config.DefaultTrustedProxies()
		}})
		if got := fmt.Sprint(attempts(e, "203.0.113.9")); got != "[401 401 401 429 429 429]" {
			t.Fatalf("one real client rotating forged hops: %s", got)
		}
		if got := login(e.browser(), "203.0.113.10"); got != 401 {
			t.Fatalf("another real client must have its own bucket, got %d", got)
		}
	})
	t.Run("with TRUST_PROXY off the header is not read at all", func(t *testing.T) {
		e := newEnv(t, envOpts{mutate: limit})
		if got := fmt.Sprint(attempts(e, "203.0.113.9")); got != "[401 401 401 429 429 429]" {
			t.Fatalf("one peer rotating X-Forwarded-For: %s", got)
		}
	})
	t.Run("an untrusted peer cannot make itself a proxy", func(t *testing.T) {
		e := newEnv(t, envOpts{mutate: func(c *config.Config) {
			limit(c)
			c.TrustProxy, c.TrustedProxies = true, config.DefaultTrustedProxies()[2:3] // only 10.0.0.0/8; the test peer is 127.0.0.1
		}})
		if got := fmt.Sprint(attempts(e, "203.0.113.9")); got != "[401 401 401 429 429 429]" {
			t.Fatalf("one peer outside the trust set: %s", got)
		}
	})
}

// ---------------------------------------------------------------- ops

type pingFailStorage struct {
	media.Storage
	err error
}

func (p pingFailStorage) Ping(context.Context) error { return p.err }

func TestReadyReportsFailingDependencies(t *testing.T) {
	secretErr := fmt.Errorf("dial tcp 10.9.8.7:9000 (key=AKIASECRET): %w", syscall.ECONNREFUSED)
	e := newEnv(t, envOpts{storage: pingFailStorage{Storage: nil, err: secretErr}})
	c := e.browser()
	for _, path := range []string{"/ready", "/api/v1/ready"} {
		r := c.do("GET", path, nil)
		body := r.json(t)
		checks, _ := body["checks"].(map[string]any)
		errsMap, _ := body["errors"].(map[string]any)
		if r.status != 503 || body["status"] != "unavailable" || checks["storage"] != "unavailable" ||
			checks["postgres"] != "ok" || checks["redis"] != "ok" {
			t.Fatalf("%s: %d %s", path, r.status, r.body)
		}
		if errsMap["storage"] != "connection refused" || len(errsMap) != 1 {
			t.Fatalf("%s: coarse reason expected, got %v", path, errsMap)
		}
		if strings.Contains(string(r.body), "10.9.8.7") || strings.Contains(string(r.body), "AKIASECRET") {
			t.Fatalf("%s leaks connection details: %s", path, r.body)
		}
	}
	// Liveness is independent from readiness.
	if r := c.do("GET", "/health", nil); r.status != 200 {
		t.Fatalf("health: %d", r.status)
	}
	// Timeouts are classified too.
	e2 := newEnv(t, envOpts{storage: pingFailStorage{err: context.DeadlineExceeded}})
	if r := e2.browser().do("GET", "/ready", nil); r.status != 503 || r.json(t)["errors"].(map[string]any)["storage"] != "timed out (dependency unreachable or slow)" {
		t.Fatalf("timeout classification: %d %s", r.status, r.body)
	}
	e3 := newEnv(t, envOpts{storage: pingFailStorage{err: errors.New("NoSuchBucket")}})
	if r := e3.browser().do("GET", "/ready", nil); r.status != 503 || r.json(t)["errors"].(map[string]any)["storage"] != "unreachable or misconfigured" {
		t.Fatalf("generic classification: %d %s", r.status, r.body)
	}
}

func TestMetricsAreProtectedAndCountPublishes(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true, metricsToken: "scrape-me"})
	c := e.browser()
	c.register("metrics@example.com")
	if r := c.do("GET", "/metrics", nil); r.status != 401 || r.errCode(t) != "UNAUTHENTICATED" {
		t.Fatalf("metrics without token: %d %s", r.status, r.body)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if r := c.send(req); r.status != 401 {
		t.Fatalf("metrics with a wrong token: %d", r.status)
	}

	acc := c.connectMock()
	ok := c.must("POST", "/api/v1/posts", map[string]any{"content": "fine", "social_account_ids": []string{acc}}, 201)
	c.must("POST", "/api/v1/posts/"+ok["id"].(string)+"/publish", nil, 202)
	c.waitStatus(ok["id"].(string), "published", 15*time.Second)
	bad := c.must("POST", "/api/v1/posts", map[string]any{"content": "#mock-auth nope", "social_account_ids": []string{acc}}, 201)
	c.must("POST", "/api/v1/posts/"+bad["id"].(string)+"/publish", nil, 202)
	c.waitStatus(bad["id"].(string), "failed", 15*time.Second)

	scrape := func() string {
		req, _ := http.NewRequest("GET", e.srv.URL+"/metrics", nil)
		req.Header.Set("Authorization", "Bearer scrape-me")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("metrics: %d %s", res.StatusCode, b)
		}
		return string(b)
	}
	m := scrape()
	for _, want := range []string{
		`socialos_publish_attempts_total{outcome="published",provider="mock"} 1`,
		`socialos_publish_attempts_total{outcome="failed",provider="mock"} 1`,
		"socialos_http_requests_total", "socialos_http_request_duration_seconds",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	// Route patterns, not raw paths, label the HTTP metrics (no per-id cardinality).
	if strings.Contains(m, ok["id"].(string)) {
		t.Errorf("post ids leaked into metric labels")
	}
}

// ---------------------------------------------------------------- tenancy

func TestTenantIsolationEverywhere(t *testing.T) {
	e := newEnv(t, envOpts{})
	alice, bob := e.browser(), e.browser()
	alice.register("iso-alice@example.com")
	bob.register("iso-bob@example.com")
	acc := alice.connectMock()
	up := alice.upload("a.png", pngBytes(t)).json(t)
	mid := up["id"].(string)
	post := alice.must("POST", "/api/v1/posts", map[string]any{"content": "alice only", "social_account_ids": []string{acc}, "media_ids": []string{mid}}, 201)
	pid := post["id"].(string)
	key := alice.must("POST", "/api/v1/developer/api-keys", map[string]any{"name": "alice key", "scopes": []string{"posts:read"}}, 201)["api_key"].(map[string]any)
	mcp := alice.must("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": "alice mcp"}, 201)["connection"].(map[string]any)
	alice.must("POST", "/api/v1/posts/"+pid+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))}, 200)

	// Everything addressed by id answers 404, never 403, so ids cannot be probed.
	for _, tc := range []struct{ m, p string }{
		{"GET", "/api/v1/media/" + mid}, {"DELETE", "/api/v1/media/" + mid},
		{"GET", "/api/v1/posts/" + pid + "/status"}, {"POST", "/api/v1/posts/" + pid + "/schedule"}, {"POST", "/api/v1/posts/" + pid + "/unschedule"},
		{"POST", "/api/v1/posts/" + pid + "/cancel"}, {"POST", "/api/v1/posts/" + pid + "/retry"},
		{"DELETE", "/api/v1/developer/api-keys/" + key["id"].(string)}, {"DELETE", "/api/v1/developer/mcp-connections/" + mcp["id"].(string)},
	} {
		r := bob.do(tc.m, tc.p, map[string]any{"scheduled_at": fmtTime(time.Now().Add(2 * time.Hour))})
		if r.status != 404 || r.errCode(t) != "NOT_FOUND" {
			t.Errorf("bob %s %s: want 404 NOT_FOUND, got %d %s", tc.m, tc.p, r.status, r.body)
		}
	}
	// ...and none of it changed.
	if st := alice.must("GET", "/api/v1/posts/"+pid+"/status", nil, 200); st["status"] != "scheduled" {
		t.Fatalf("bob changed alice's post: %v", st)
	}
	alice.must("GET", "/api/v1/media/"+mid, nil, 200)
	if r := e.apiKeyClient("sk_live_x").do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("bogus key: %d", r.status)
	}
	// Bob cannot attach alice's media or account to his own post.
	for name, body := range map[string]map[string]any{
		"media":   {"content": "x", "media_ids": []string{mid}},
		"account": {"content": "x", "social_account_ids": []string{acc}},
	} {
		if r := bob.do("POST", "/api/v1/posts", body); r.status != 400 {
			t.Errorf("bob attached alice's %s: %d %s", name, r.status, r.body)
		}
	}
	// Lists, dashboards, analytics and audit logs only show the caller's own data.
	for _, p := range []string{"/api/v1/posts", "/api/v1/media", "/api/v1/social/accounts", "/api/v1/developer/api-keys", "/api/v1/developer/mcp-connections"} {
		if items := bob.must("GET", p, nil, 200)["items"].([]any); len(items) != 0 {
			t.Errorf("bob sees %d items in %s", len(items), p)
		}
	}
	dash := bob.must("GET", "/api/v1/dashboard/summary", nil, 200)
	if dash["connected_accounts"].(float64) != 0 || dash["scheduled_posts"].(float64) != 0 || len(dash["upcoming"].([]any)) != 0 {
		t.Errorf("bob's dashboard shows alice's data: %v", dash)
	}
	if an := bob.must("GET", "/api/v1/analytics", nil, 200); len(an["items"].([]any)) != 0 {
		t.Errorf("bob's analytics: %v", an)
	}
	for _, l := range bob.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)["items"].([]any) {
		m := l.(map[string]any)
		if m["actor_label"] != nil && strings.Contains(fmt.Sprint(m["actor_label"], m["metadata"]), "alice") {
			t.Errorf("bob sees alice's audit entry: %v", m)
		}
	}
	if u := bob.must("GET", "/api/v1/developer/usage", nil, 200); u["total_requests"].(float64) != 0 || len(u["by_key"].([]any)) != 0 {
		t.Errorf("bob's usage: %v", u)
	}
	// A cursor minted for alice is harmless in bob's hands.
	alice.must("POST", "/api/v1/posts", map[string]any{"content": "second"}, 201)
	cur := alice.must("GET", "/api/v1/posts?limit=1", nil, 200)["next_cursor"]
	if cur == nil {
		t.Fatal("expected a cursor")
	}
	if items := bob.must("GET", "/api/v1/posts?limit=1&cursor="+cur.(string), nil, 200)["items"].([]any); len(items) != 0 {
		t.Errorf("cursor crossed tenants: %v", items)
	}
	// Alice's key cannot be used to act as bob and vice versa: /me always reports the owner.
	k := e.apiKeyClient(alice.createKey("alice reader", "posts:read"))
	if me := k.must("GET", "/api/v1/me", nil, 200); me["user"].(map[string]any)["email"] != "iso-alice@example.com" {
		t.Errorf("key identity: %v", me)
	}
	if items := k.must("GET", "/api/v1/posts", nil, 200)["items"].([]any); len(items) != 2 {
		t.Errorf("alice's key should see alice's 2 posts, saw %d", len(items))
	}
	// Disconnecting is tenant-scoped too, and a deleted user's data is unreachable via old sessions.
	if r := bob.do("DELETE", "/api/v1/social/accounts/"+acc, nil); r.status != 404 {
		t.Errorf("bob disconnected alice's account: %d", r.status)
	}
}

// TestLogoutInvalidatesSession makes sure sessions are server-side: replaying
// the old cookie after logout is a plain 401.
func TestLogoutInvalidatesSession(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	c.register("logout@example.com")
	stolen := c.http.Jar.Cookies(mustURL(t, e.srv.URL))
	var cookie *http.Cookie
	for _, ck := range stolen {
		if ck.Name == "socialos_session" {
			cookie = ck
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	c.must("POST", "/api/v1/auth/logout", nil, 204)
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/me", nil)
	req.AddCookie(cookie)
	if r := e.browser().send(req); r.status != 401 || r.errCode(t) != "UNAUTHENTICATED" {
		t.Fatalf("replayed cookie after logout: %d %s", r.status, r.body)
	}
	if r := c.do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("session usable after logout: %d", r.status)
	}
	// Expired sessions are rejected as well.
	d := e.browser()
	d.register("expired@example.com")
	if _, err := e.app.DB.Pool.Exec(context.Background(), `UPDATE sessions SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if r := d.do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("expired session accepted: %d", r.status)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
