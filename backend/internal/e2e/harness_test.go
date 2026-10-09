// Package e2e runs the full API + worker against real Postgres and Redis
// (TEST_DATABASE_URL, TEST_REDIS_URL) with the mock provider and httptest fakes.
package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/app"
	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/infrastructure/queue"
	"github.com/socialos/backend/internal/infrastructure/storage"
	"github.com/socialos/backend/internal/testutil"
	"github.com/socialos/backend/internal/transport/middleware"
)

type env struct {
	t       *testing.T
	app     *app.App
	srv     *httptest.Server
	storage *storage.Memory
}

type envOpts struct {
	providers   []provider.Provider
	startWorker bool
	// retryDelay replaces the 30s·2^n backoff of the real Asynq worker.
	retryDelay func(n int, err error) time.Duration
	// rateRPS/rateBurst override the (very permissive) API rate limit.
	rateRPS   float64
	rateBurst int
	// metricsToken protects /metrics when set.
	metricsToken string
	// mutate lets a test adjust the config before the app is built.
	mutate func(*config.Config)
	// storage replaces the in-memory object store (e.g. a failing one).
	storage media.Storage
	// mailer replaces the log mailer.
	mailer port.Mailer
	// logger replaces the discarding test logger (e.g. to assert nothing sensitive is logged).
	logger *slog.Logger
	// realMailLimit keeps the production mail-endpoint limiter; by default tests get a permissive one.
	realMailLimit bool
}

func newEnv(t *testing.T, o envOpts) *env {
	t.Helper()
	dbURL := testutil.FreshDatabase(t)
	redisURL := testutil.RedisURL(t)
	mem := storage.NewMemory()
	var store media.Storage = mem
	if o.storage != nil {
		store = o.storage
	}
	e := &env{t: t, storage: mem}
	e.srv = httptest.NewUnstartedServer(nil)
	base := "http://" + e.srv.Listener.Addr().String()
	cfg := &config.Config{
		Env: "test", DatabaseURL: dbURL, DBMaxConns: 10, RedisURL: redisURL, QueueName: "e2e-" + uuid.NewString()[:8],
		EncryptionKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)),
		APIPublicURL:  base, WebBaseURL: "http://web.test", MCPPublicURL: "http://mcp.test/mcp",
		CORSOrigins: []string{"http://web.test"}, SessionTTL: time.Hour, MockProviders: true,
		RateLimitRPS: 1000, RateLimitBurst: 1000, AuthRateRPS: 1000, AuthRateBurst: 1000, StorageDriver: "memory",
		AgentMinScheduleLead: 5 * time.Minute, ApprovalTTL: 10 * time.Minute, ApprovalMaxPending: 10,
		TelegramToken: "", LinkedInVersion: "202606", MetricsToken: o.metricsToken,
	}
	if o.rateBurst > 0 {
		cfg.RateLimitRPS, cfg.RateLimitBurst = o.rateRPS, o.rateBurst
	}
	if o.mutate != nil {
		o.mutate(cfg)
	}
	log := testutil.Logger()
	if o.logger != nil {
		log = o.logger
	}
	a, err := app.Build(context.Background(), cfg, log, app.Overrides{
		Storage: store, Providers: o.providers, Mailer: o.mailer,
		Hasher: crypto.NewPasswordHasher(crypto.Argon2Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}),
	})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	t.Cleanup(a.Close)
	e.app = a
	if !o.realMailLimit {
		a.MailLimit = middleware.NewLimiter(1000, 1000)
	}
	e.srv.Config.Handler = a.Router()
	e.srv.Start()
	t.Cleanup(e.srv.Close)
	if o.startWorker {
		w := queue.NewServer(a.Redis.Asynq, queue.ServerConfig{Queue: cfg.QueueName, Concurrency: 4, DelayedCheck: 200 * time.Millisecond,
			RetryDelay: o.retryDelay, Mailer: a.Mailer, Auth: a.Services.Auth}, a.Publisher, testutil.Logger())
		if err := w.Start(); err != nil {
			t.Fatalf("start worker: %v", err)
		}
		t.Cleanup(w.Shutdown)
	}
	return e
}

// client is a browser-like (cookie jar, CSRF) or API-key client.
type client struct {
	e      *env
	http   *http.Client
	csrf   string
	bearer string
}

func (e *env) browser() *client {
	jar, _ := cookiejar.New(nil)
	srvHost := strings.TrimPrefix(e.srv.URL, "http://")
	hc := &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host != srvHost {
			return http.ErrUseLastResponse // do not leave the API (web app / provider)
		}
		return nil
	}}
	return &client{e: e, http: hc}
}

func (e *env) apiKeyClient(raw string) *client {
	return &client{e: e, http: &http.Client{Timeout: 30 * time.Second}, bearer: raw}
}

type resp struct {
	status int
	body   []byte
	header http.Header
}

func (r resp) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("decode %d %s: %v", r.status, r.body, err)
	}
	return m
}

// errBody returns the decoded {"error":{...}} object.
func (r resp) errBody(t *testing.T) map[string]any {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	if e == nil {
		t.Fatalf("not an error envelope: %d %s", r.status, r.body)
	}
	return e
}

func (r resp) errCode(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func (c *client) do(method, path string, body any) resp { return c.doWith(method, path, body, nil) }

// doWith is do with extra request headers (e.g. X-Approval-Id).
func (c *client) doWith(method, path string, body any, headers map[string]string) resp {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.e.srv.URL+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.send(req)
}

func (c *client) send(req *http.Request) resp {
	c.e.t.Helper()
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		checkResponseHygiene(c.e.t, req, b)
	}
	return resp{status: res.StatusCode, body: b, header: res.Header}
}

var rfc3339Re = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

// leakedKeys must never appear as JSON keys in any API response.
var leakedKeys = map[string]bool{"password_hash": true, "key_hash": true, "token_hash": true, "access_token_enc": true,
	"refresh_token_enc": true, "access_token": true, "refresh_token": true, "code_verifier": true, "state_hash": true}

// checkResponseHygiene runs on every JSON response of every test: timestamps
// must be RFC 3339 UTC ("…Z") and no secret-bearing field may be serialised.
func checkResponseHygiene(t *testing.T, req *http.Request, body []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Errorf("%s %s: invalid JSON response: %v", req.Method, req.URL.Path, err)
		return
	}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, vv := range x {
				if leakedKeys[k] {
					t.Errorf("%s %s: response leaks %q at %s", req.Method, req.URL.Path, k, path)
				}
				walk(path+"."+k, vv)
			}
		case []any:
			for i, vv := range x {
				walk(fmt.Sprintf("%s[%d]", path, i), vv)
			}
		case string:
			if rfc3339Re.MatchString(x) && !strings.HasSuffix(x, "Z") {
				t.Errorf("%s %s: non-UTC timestamp %q at %s", req.Method, req.URL.Path, x, path)
			}
		}
	}
	walk("$", v)
}

func (c *client) must(method, path string, body any, want int) map[string]any {
	c.e.t.Helper()
	r := c.do(method, path, body)
	if r.status != want {
		c.e.t.Fatalf("%s %s: status %d want %d: %s", method, path, r.status, want, r.body)
	}
	if len(r.body) == 0 {
		return nil
	}
	return r.json(c.e.t)
}

// register creates a user and stores its CSRF token.
func (c *client) register(email string) map[string]any {
	c.e.t.Helper()
	m := c.must("POST", "/api/v1/auth/register", map[string]any{"email": email, "password": "correct horse battery", "display_name": "Tester"}, 201)
	c.csrf, _ = m["csrf_token"].(string)
	if c.csrf == "" {
		c.e.t.Fatal("no csrf token returned")
	}
	return m
}

// connectMock runs the full OAuth flow against the mock provider.
func (c *client) connectMock() string {
	c.e.t.Helper()
	r := c.do("GET", "/api/v1/social/mock/connect?redirect=/accounts", nil)
	if r.status != http.StatusFound {
		c.e.t.Fatalf("connect: %d %s", r.status, r.body)
	}
	loc, _ := url.Parse(r.header.Get("Location"))
	if loc.Host != "web.test" || loc.Query().Get("connected") != "mock" {
		c.e.t.Fatalf("unexpected final redirect %s", loc)
	}
	accs := c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)
	for _, a := range accs {
		acc := a.(map[string]any)
		if acc["provider"] == "mock" {
			return acc["id"].(string)
		}
	}
	c.e.t.Fatal("mock account not connected")
	return ""
}

// createTrustedKey mints a key whose dangerous actions skip the owner's approval (dangerous_policy "trusted").
func (c *client) createTrustedKey(name string, scopes ...string) string {
	c.e.t.Helper()
	m := c.must("POST", "/api/v1/developer/api-keys", map[string]any{"name": name, "scopes": scopes, "dangerous_policy": "trusted"}, 201)
	return m["key"].(string)
}

func (c *client) createKey(name string, scopes ...string) string {
	c.e.t.Helper()
	m := c.must("POST", "/api/v1/developer/api-keys", map[string]any{"name": name, "scopes": scopes}, 201)
	return m["key"].(string)
}

// waitStatus polls the post status until it equals want.
func (c *client) waitStatus(postID, want string, timeout time.Duration) map[string]any {
	c.e.t.Helper()
	deadline := time.Now().Add(timeout)
	var last map[string]any
	for time.Now().Before(deadline) {
		last = c.must("GET", "/api/v1/posts/"+postID+"/status", nil, 200)
		if last["status"] == want {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.e.t.Fatalf("post %s did not reach %s: %v", postID, want, last)
	return nil
}

func auditActions(t *testing.T, c *client) map[string]int {
	t.Helper()
	m := c.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)
	out := map[string]int{}
	for _, it := range m["items"].([]any) {
		out[it.(map[string]any)["action"].(string)]++
	}
	return out
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

var _ = fmt.Sprintf
