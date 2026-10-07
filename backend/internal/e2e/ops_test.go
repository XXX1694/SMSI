package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/telegram"
	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/config"
)

// ---------------------------------------------------------------- Telegram

const fakeBotToken = "123456:AAH-SECRET-bot-token"

// fakeBotAPI is a minimal Telegram Bot API. It records sendMessage calls and
// can be switched to reject them.
type fakeBotAPI struct {
	srv      *httptest.Server
	mu       sync.Mutex
	texts    []string
	failSend bool
}

func newFakeBotAPI(t *testing.T) *fakeBotAPI {
	f := &fakeBotAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBotAPI) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...)
}

func (f *fakeBotAPI) handle(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	prefix := "/bot" + fakeBotToken + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		reply(401, map[string]any{"ok": false, "error_code": 401, "description": "Unauthorized"})
		return
	}
	var params map[string]any
	_ = json.NewDecoder(r.Body).Decode(&params)
	switch strings.TrimPrefix(r.URL.Path, prefix) {
	case "getMe":
		reply(200, map[string]any{"ok": true, "result": map[string]any{"id": 42, "is_bot": true, "username": "socialos_bot"}})
	case "getChat":
		if params["chat_id"] != "@e2e_channel" {
			reply(400, map[string]any{"ok": false, "error_code": 400, "description": "Bad Request: chat not found"})
			return
		}
		reply(200, map[string]any{"ok": true, "result": map[string]any{"id": -1009876543210, "type": "channel", "title": "E2E Channel", "username": "e2e_channel"}})
	case "getChatMember":
		reply(200, map[string]any{"ok": true, "result": map[string]any{"status": "administrator", "can_post_messages": true}})
	case "sendMessage":
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failSend {
			reply(403, map[string]any{"ok": false, "error_code": 403, "description": "Forbidden: bot was kicked from the channel chat"})
			return
		}
		f.texts = append(f.texts, fmt.Sprint(params["text"]))
		reply(200, map[string]any{"ok": true, "result": map[string]any{"message_id": 100 + len(f.texts)}})
	default:
		reply(404, map[string]any{"ok": false, "error_code": 404, "description": "Not Found"})
	}
}

func TestTelegramConnectAndPublish(t *testing.T) {
	bot := newFakeBotAPI(t)
	tg := telegram.New(telegram.Config{BotToken: fakeBotToken, APIBaseURL: bot.srv.URL})
	e := newEnv(t, envOpts{startWorker: true, providers: []provider.Provider{tg}})
	c := e.browser()
	c.register("tg@example.com")

	var bodies []string
	seen := func(r resp) resp { bodies = append(bodies, string(r.body)); return r }

	p := seen(c.do("GET", "/api/v1/social/providers", nil))
	for _, it := range p.json(t)["items"].([]any) {
		if m := it.(map[string]any); m["id"] == "telegram" && (m["configured"] != true || m["status"] != "supported") {
			t.Fatalf("telegram must be configured once a bot token is present: %v", m)
		}
	}

	// Validation and verification failures.
	for chat, want := range map[string]int{"": 400, "not a chat!": 400, "@someone_else": 400} {
		if r := seen(c.do("POST", "/api/v1/social/telegram/connect", map[string]any{"chat": chat})); r.status != want {
			t.Errorf("chat %q: want %d got %d %s", chat, want, r.status, r.body)
		}
	}
	// A key may not connect accounts.
	k := e.apiKeyClient(c.createKey("tg key", "social:read", "posts:read", "posts:write", "posts:schedule", "posts:publish"))
	if r := k.do("POST", "/api/v1/social/telegram/connect", map[string]any{"chat": "@e2e_channel"}); r.status != 403 {
		t.Errorf("api key connected a chat: %d %s", r.status, r.body)
	}

	acc := seen(c.do("POST", "/api/v1/social/telegram/connect", map[string]any{"chat": "https://t.me/e2e_channel"}))
	if acc.status != 201 {
		t.Fatalf("connect: %d %s", acc.status, acc.body)
	}
	a := acc.json(t)
	if a["provider"] != "telegram" || a["username"] != "e2e_channel" || a["display_name"] != "E2E Channel" || a["status"] != "active" {
		t.Fatalf("telegram account: %v", a)
	}
	accID := a["id"].(string)
	// Connecting the same chat again does not duplicate the account.
	again := seen(c.do("POST", "/api/v1/social/telegram/connect", map[string]any{"chat": "@e2e_channel"}))
	if again.status != 201 && again.status != 200 || again.json(t)["id"] != accID {
		t.Fatalf("reconnect: %d %s", again.status, again.body)
	}
	if n := len(c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)); n != 1 {
		t.Fatalf("reconnect duplicated the chat: %d accounts", n)
	}

	// Publish through the real worker; the bot receives exactly the text.
	post := seen(c.do("POST", "/api/v1/posts", map[string]any{"content": "hello telegram", "social_account_ids": []string{accID}}))
	id := post.json(t)["id"].(string)
	seen(c.do("POST", "/api/v1/posts/"+id+"/publish", nil))
	st := c.waitStatus(id, "published", 20*time.Second)
	tgt := st["targets"].([]any)[0].(map[string]any)
	if got := bot.sent(); len(got) != 1 || got[0] != "hello telegram" {
		t.Fatalf("bot received %v", got)
	}
	if tgt["external_post_id"] != "-1009876543210:101" || tgt["external_url"] != "https://t.me/e2e_channel/101" {
		t.Fatalf("external reference: %v", tgt)
	}

	// A rejected post fails with a classified, sanitized error.
	bot.mu.Lock()
	bot.failSend = true
	bot.mu.Unlock()
	bad := c.must("POST", "/api/v1/posts", map[string]any{"content": "will fail", "social_account_ids": []string{accID}}, 201)
	c.must("POST", "/api/v1/posts/"+bad["id"].(string)+"/publish", nil, 202)
	failed := c.waitStatus(bad["id"].(string), "failed", 20*time.Second)
	ft := failed["targets"].([]any)[0].(map[string]any)
	if ft["error_code"] == nil || ft["error_code"] == "" || strings.Contains(fmt.Sprint(ft), fakeBotToken) {
		t.Fatalf("failure details: %v", ft)
	}
	bodies = append(bodies, fmt.Sprint(failed), fmt.Sprint(c.must("GET", "/api/v1/posts/"+bad["id"].(string), nil, 200)))
	bodies = append(bodies, fmt.Sprint(c.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)))

	// The bot token is configuration: it must be in no response and in no table.
	for _, b := range bodies {
		if strings.Contains(b, fakeBotToken) || strings.Contains(b, "AAH-SECRET") {
			t.Fatalf("bot token leaked in an API response: %.300s", b)
		}
	}
	for table, col := range map[string]string{"social_accounts": "metadata::text", "audit_logs": "metadata::text",
		"publication_attempts": "coalesce(error_message,'')", "post_targets": "coalesce(error_message,'')"} {
		var n int
		q := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s LIKE '%%AAH-SECRET%%'", table, col)
		if err := e.app.DB.Pool.QueryRow(context.Background(), q).Scan(&n); err != nil || n != 0 {
			t.Errorf("bot token stored in %s (n=%d err=%v)", table, n, err)
		}
	}
	var credRows int
	_ = e.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_credentials WHERE social_account_id = $1`, accID).Scan(&credRows)
	if credRows != 0 {
		t.Errorf("telegram accounts need no stored credentials, found %d", credRows)
	}
}

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
