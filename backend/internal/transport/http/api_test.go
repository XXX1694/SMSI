package http_test

// HTTP-level tests against the real router with in-memory object storage.
// They need Postgres and Redis (TEST_DATABASE_URL / TEST_REDIS_URL) and skip
// otherwise; the worker is not started, nothing here publishes.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/app"
	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/infrastructure/storage"
	"github.com/socialos/backend/internal/testutil"
)

const pw = "correct horse battery"

type harness struct {
	t   *testing.T
	srv *httptest.Server
	mem *storage.Memory
	app *app.App
}

type opts struct {
	store media.Storage
	tune  func(*config.Config)
}

func newHarness(t *testing.T, o opts) *harness {
	t.Helper()
	db, rds := testutil.FreshDatabase(t), testutil.RedisURL(t)
	mem := storage.NewMemory()
	var store media.Storage = mem
	if o.store != nil {
		store = o.store
	}
	h := &harness{t: t, mem: mem}
	h.srv = httptest.NewUnstartedServer(nil)
	cfg := &config.Config{
		Env: "test", DatabaseURL: db, DBMaxConns: 8, RedisURL: rds, QueueName: "http-" + uuid.NewString()[:8],
		EncryptionKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		APIPublicURL:  "http://" + h.srv.Listener.Addr().String(), WebBaseURL: "http://web.test", MCPPublicURL: "http://mcp.test/mcp",
		CORSOrigins: []string{"http://web.test"}, SessionTTL: time.Hour, MockProviders: true, StorageDriver: "memory",
		RateLimitRPS: 1000, RateLimitBurst: 1000, AuthRateRPS: 1000, AuthRateBurst: 1000, LinkedInVersion: "202606",
		QuotaConfig: config.QuotaConfig{QuotaAccounts: -1, QuotaPostsPerMonth: -1, QuotaMediaMB: -1, QuotaAgentRPM: -1},
	}
	if o.tune != nil {
		o.tune(cfg)
	}
	a, err := app.Build(context.Background(), cfg, testutil.Logger(), app.Overrides{
		Storage: store, Hasher: crypto.NewPasswordHasher(crypto.Argon2Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}, 2, 0),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(a.Close)
	h.app = a
	h.srv.Config.Handler = a.Router()
	h.srv.Start()
	t.Cleanup(h.srv.Close)
	return h
}

type reply struct {
	status int
	header http.Header
	body   []byte
}

func (r reply) obj(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("not JSON (%d): %.200s", r.status, r.body)
	}
	return m
}

// apiErr asserts the uniform error envelope and returns its body.
func (r reply) apiErr(t *testing.T, status int, code string) map[string]any {
	t.Helper()
	if r.status != status {
		t.Fatalf("status %d, want %d: %.300s", r.status, status, r.body)
	}
	if ct := r.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("error content type %q", ct)
	}
	raw := r.obj(t)
	if len(raw) != 1 {
		t.Fatalf("envelope must contain only \"error\": %v", raw)
	}
	e, _ := raw["error"].(map[string]any)
	if e["code"] != code {
		t.Fatalf("error code %v, want %s: %s", e["code"], code, r.body)
	}
	if msg, _ := e["message"].(string); msg == "" {
		t.Fatalf("error without message: %s", r.body)
	}
	rid, _ := e["request_id"].(string)
	if rid == "" || rid != r.header.Get("X-Request-ID") || rid != r.header.Get("X-Correlation-ID") {
		t.Fatalf("request_id %q must equal X-Request-ID %q and X-Correlation-ID %q", rid, r.header.Get("X-Request-ID"), r.header.Get("X-Correlation-ID"))
	}
	for k := range e {
		if k != "code" && k != "message" && k != "request_id" && k != "fields" {
			t.Fatalf("unexpected error key %q: %s", k, r.body)
		}
	}
	return e
}

type agent struct {
	h      *harness
	hc     *http.Client
	csrf   string
	bearer string
}

func (h *harness) anon() *agent {
	jar, _ := cookiejar.New(nil)
	return &agent{h: h, hc: &http.Client{Jar: jar, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
}

func (h *harness) bearer(raw string) *agent {
	a := h.anon()
	a.bearer = raw
	return a
}

func (a *agent) req(method, path string, body any, mod ...func(*http.Request)) reply {
	a.h.t.Helper()
	var rd io.Reader
	if s, ok := body.(string); ok {
		rd = strings.NewReader(s)
	} else if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, a.h.srv.URL+path, rd)
	if err != nil {
		a.h.t.Fatal(err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if a.csrf != "" {
		r.Header.Set("X-CSRF-Token", a.csrf)
	}
	if a.bearer != "" {
		r.Header.Set("Authorization", "Bearer "+a.bearer)
	}
	for _, m := range mod {
		m(r)
	}
	return a.do(r)
}

func (a *agent) do(r *http.Request) reply {
	a.h.t.Helper()
	res, err := a.hc.Do(r)
	if err != nil {
		a.h.t.Fatalf("%s %s: %v", r.Method, r.URL.Path, err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return reply{status: res.StatusCode, header: res.Header, body: b}
}

func (a *agent) register(email string) map[string]any {
	a.h.t.Helper()
	r := a.req("POST", "/api/v1/auth/register", map[string]any{"email": email, "password": pw, "display_name": "T"})
	if r.status != 201 {
		a.h.t.Fatalf("register: %d %s", r.status, r.body)
	}
	m := r.obj(a.h.t)
	a.csrf, _ = m["csrf_token"].(string)
	return m
}

func (a *agent) cookie(name string) *http.Cookie {
	for _, c := range a.hc.Jar.Cookies(mustParse(a.h.t, a.h.srv.URL)) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ------------------------------------------------------------------ auth

func TestRegisterValidation(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	long := strings.Repeat("p", 129)
	for name, body := range map[string]any{
		"empty body":        `{}`,
		"missing email":     map[string]any{"password": pw},
		"invalid email":     map[string]any{"email": "not-an-email", "password": pw},
		"display in email":  map[string]any{"email": "Bob <bob@example.com>", "password": pw},
		"email too long":    map[string]any{"email": strings.Repeat("a", 250) + "@example.com", "password": pw},
		"short password":    map[string]any{"email": "a@example.com", "password": "short"},
		"password 7 chars":  map[string]any{"email": "a@example.com", "password": "1234567"},
		"password too long": map[string]any{"email": "a@example.com", "password": long},
		"long display name": map[string]any{"email": "a@example.com", "password": pw, "display_name": strings.Repeat("n", 101)},
		"wrong field type":  `{"email": 5, "password": "x"}`,
		"malformed json":    `{"email": "a@example.com"`,
	} {
		e := a.req("POST", "/api/v1/auth/register", body).apiErr(t, 400, "VALIDATION_ERROR")
		if strings.Contains(fmt.Sprint(e), pw) {
			t.Errorf("%s: error echoes the password: %v", name, e)
		}
	}
	// Field-level details for forms.
	e := a.req("POST", "/api/v1/auth/register", map[string]any{"email": "nope", "password": "x"}).apiErr(t, 400, "VALIDATION_ERROR")
	if e["fields"] == nil {
		t.Errorf("validation errors should say which field: %v", e)
	}
	if r := a.req("POST", "/api/v1/auth/register", `email=a`, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); r.status != 400 {
		t.Errorf("non-JSON body: %d", r.status)
	}
	// The boundary lengths work: 8 characters, 128 characters.
	for i, p := range []string{"12345678", strings.Repeat("p", 128)} {
		if r := h.anon().req("POST", "/api/v1/auth/register", map[string]any{"email": fmt.Sprintf("ok%d@example.com", i), "password": p}); r.status != 201 {
			t.Errorf("password of %d chars: %d %s", len(p), r.status, r.body)
		}
	}
}

func TestRegisterLoginLogout(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	me := a.register("Mixed.Case@Example.com")
	if me["email"] != "mixed.case@example.com" {
		t.Fatalf("email must be normalised: %v", me["email"])
	}
	for _, k := range []string{"password", "password_hash"} {
		if _, ok := me[k]; ok || strings.Contains(string(mustJSON(me)), "argon2") {
			t.Fatalf("response exposes %s", k)
		}
	}
	// Duplicate email, any casing.
	for _, e := range []string{"mixed.case@example.com", "MIXED.CASE@EXAMPLE.COM"} {
		h.anon().req("POST", "/api/v1/auth/register", map[string]any{"email": e, "password": pw}).apiErr(t, 409, "CONFLICT")
	}

	// Cookies.
	sess, csrf := a.cookie("socialos_session"), a.cookie("socialos_csrf")
	if sess == nil || csrf == nil {
		t.Fatal("cookies not set")
	}
	if csrf.Value != me["csrf_token"] {
		t.Fatalf("csrf cookie %q must equal the csrf_token in the body %v (double submit)", csrf.Value, me["csrf_token"])
	}
	res := h.anon().req("POST", "/api/v1/auth/login", map[string]any{"email": "mixed.case@example.com", "password": pw})
	if res.status != 200 {
		t.Fatalf("login: %d %s", res.status, res.body)
	}
	var sc, cc string
	for _, line := range res.header.Values("Set-Cookie") {
		switch {
		case strings.HasPrefix(line, "socialos_session="):
			sc = line
		case strings.HasPrefix(line, "socialos_csrf="):
			cc = line
		}
	}
	for _, want := range []string{"HttpOnly", "SameSite=Lax", "Path=/", "Max-Age="} {
		if !strings.Contains(sc, want) {
			t.Errorf("session cookie lacks %s: %s", want, sc)
		}
	}
	if strings.Contains(cc, "HttpOnly") || !strings.Contains(cc, "SameSite=Lax") {
		t.Errorf("csrf cookie must be readable by scripts and SameSite=Lax: %s", cc)
	}
	if strings.Contains(sc, "Secure") {
		t.Errorf("Secure must only be set when COOKIE_SECURE is enabled: %s", sc)
	}

	// Login failures are indistinguishable: wrong password, unknown user, malformed email.
	var msgs []string
	for _, c := range []map[string]any{
		{"email": "mixed.case@example.com", "password": "wrong password!"},
		{"email": "ghost@example.com", "password": pw},
		{"email": "not-an-email", "password": pw},
		{"email": "", "password": ""},
	} {
		e := h.anon().req("POST", "/api/v1/auth/login", c).apiErr(t, 401, "UNAUTHENTICATED")
		msgs = append(msgs, fmt.Sprint(e["message"], e["fields"]))
	}
	for _, m := range msgs[1:] {
		if m != msgs[0] {
			t.Errorf("login errors differ, which allows account enumeration: %q vs %q", msgs[0], m)
		}
	}

	// Logout needs the CSRF header, clears both cookies and kills the session server-side.
	b := h.anon()
	b.csrf = "x"
	loginAs(t, b, "mixed.case@example.com")
	stale := b.cookie("socialos_session")
	b.csrf = "wrong"
	b.req("POST", "/api/v1/auth/logout", nil).apiErr(t, 403, "FORBIDDEN")
	b.csrf = b.cookie("socialos_csrf").Value
	out := b.req("POST", "/api/v1/auth/logout", nil)
	if out.status != 204 || len(out.body) != 0 {
		t.Fatalf("logout: %d %s", out.status, out.body)
	}
	cleared := 0
	for _, line := range out.header.Values("Set-Cookie") {
		if strings.Contains(line, "Max-Age=0") || strings.Contains(line, "Max-Age=-1") || strings.Contains(strings.ToLower(line), "expires=thu, 01 jan 1970") {
			cleared++
		}
	}
	if cleared != 2 {
		t.Errorf("logout must clear both cookies: %v", out.header.Values("Set-Cookie"))
	}
	replay := h.anon()
	replay.hc.Jar.SetCookies(mustParse(t, h.srv.URL), []*http.Cookie{stale})
	replay.req("GET", "/api/v1/me", nil).apiErr(t, 401, "UNAUTHENTICATED")
	// Logging out twice (no session any more) is a plain 401, not a crash.
	h.anon().req("POST", "/api/v1/auth/logout", nil).apiErr(t, 401, "UNAUTHENTICATED")
}

func loginAs(t *testing.T, a *agent, email string) {
	t.Helper()
	r := a.req("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": pw})
	if r.status != 200 {
		t.Fatalf("login: %d %s", r.status, r.body)
	}
	a.csrf, _ = r.obj(t)["csrf_token"].(string)
}

// ------------------------------------------------------------------ CSRF

func TestCSRFMatrix(t *testing.T) {
	h := newHarness(t, opts{})
	owner := h.anon()
	owner.register("csrf@example.com")
	other := h.anon()
	other.register("csrf-other@example.com")
	key := owner.req("POST", "/api/v1/developer/api-keys", map[string]any{"name": "k", "scopes": []string{"posts:read", "posts:write", "posts:delete"}})
	raw := key.obj(t)["key"].(string)
	good := owner.csrf

	post := func(a *agent, header string, set bool) reply {
		return a.req("POST", "/api/v1/posts", map[string]any{"content": "c"}, func(r *http.Request) {
			r.Header.Del("X-CSRF-Token")
			if set {
				r.Header.Set("X-CSRF-Token", header)
			}
		})
	}
	// Session + unsafe method.
	post(owner, "", false).apiErr(t, 403, "FORBIDDEN")
	post(owner, "", true).apiErr(t, 403, "FORBIDDEN")
	post(owner, "totally-wrong", true).apiErr(t, 403, "FORBIDDEN")
	post(owner, other.csrf, true).apiErr(t, 403, "FORBIDDEN") // another user's valid token
	post(owner, strings.ToUpper(good), true).apiErr(t, 403, "FORBIDDEN")
	post(owner, good[:len(good)-1], true).apiErr(t, 403, "FORBIDDEN")
	if r := post(owner, good, true); r.status != 201 {
		t.Fatalf("valid token rejected: %d %s", r.status, r.body)
	}
	// The token in the cookie is the same one (frontends read it from there).
	if r := post(owner, owner.cookie("socialos_csrf").Value, true); r.status != 201 {
		t.Fatalf("cookie token rejected: %d", r.status)
	}
	// Every unsafe verb is covered, not only POST.
	pid := owner.req("POST", "/api/v1/posts", map[string]any{"content": "x"}).obj(t)["id"].(string)
	for _, m := range []string{"PATCH", "DELETE", "POST"} {
		path := "/api/v1/posts/" + pid
		if m == "POST" {
			path += "/cancel"
		}
		owner.req(m, path, map[string]any{"content": "y"}, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }).apiErr(t, 403, "FORBIDDEN")
	}
	if r := owner.req("GET", "/api/v1/posts/"+pid, nil, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }); r.status != 200 {
		t.Fatalf("GET needs no CSRF token: %d", r.status)
	}
	// An unsafe request without any credentials is 401, the CSRF check never masks it.
	h.anon().req("POST", "/api/v1/posts", map[string]any{"content": "c"}).apiErr(t, 401, "UNAUTHENTICATED")

	// API keys are not cookie-borne, so they need no CSRF token...
	k := h.bearer(raw)
	if r := k.req("POST", "/api/v1/posts", map[string]any{"content": "from key"}); r.status != 201 {
		t.Fatalf("bearer POST: %d %s", r.status, r.body)
	}
	// (a delete by a key is a dangerous action: it answers 428 and waits for the owner, never 403 for a missing CSRF token)
	k.req("DELETE", "/api/v1/posts/"+pid, nil).apiErr(t, 428, "APPROVAL_REQUIRED")
	// ...and cookies are ignored once a bearer credential is present (no confused deputy).
	mixed := h.anon()
	mixed.hc.Jar.SetCookies(mustParse(t, h.srv.URL), owner.hc.Jar.Cookies(mustParse(t, h.srv.URL)))
	mixed.bearer = "sk_live_" + strings.Repeat("0", 48)
	mixed.req("POST", "/api/v1/posts", map[string]any{"content": "c"}).apiErr(t, 401, "UNAUTHENTICATED")
	// A garbage cookie is simply anonymous.
	junk := h.anon()
	junk.hc.Jar.SetCookies(mustParse(t, h.srv.URL), []*http.Cookie{{Name: "socialos_session", Value: "garbage", Path: "/"}})
	junk.req("GET", "/api/v1/me", nil).apiErr(t, 401, "UNAUTHENTICATED")
	junk.req("GET", "/health", nil)
	// CSRF protects the account-changing OAuth/connect endpoints that mutate state too.
	owner.req("POST", "/api/v1/social/telegram/connect", nil, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }).apiErr(t, 403, "FORBIDDEN")
}

func TestAuthorizationHeaderForms(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("authz@example.com")
	raw := a.req("POST", "/api/v1/developer/api-keys", map[string]any{"name": "k"}).obj(t)["key"].(string)
	for name, v := range map[string]string{
		"scheme only":       "Bearer",
		"empty token":       "Bearer ",
		"basic":             "Basic " + base64.StdEncoding.EncodeToString([]byte("a:b")),
		"raw key no scheme": raw,
		"unknown key":       "Bearer sk_live_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"right id bad pass": "Bearer " + raw[:16] + strings.Repeat("x", len(raw)-16),
		"huge":              "Bearer " + strings.Repeat("a", 8000),
		"truncated":         "Bearer " + raw[:20],
	} {
		h.anon().req("GET", "/api/v1/me", nil, func(r *http.Request) { r.Header.Set("Authorization", v) }).apiErr(t, 401, "UNAUTHENTICATED")
		_ = name
	}
	// Scheme matching is case-insensitive, as the RFC requires.
	if r := h.anon().req("GET", "/api/v1/me", nil, func(r *http.Request) { r.Header.Set("Authorization", "bearer "+raw) }); r.status != 200 {
		t.Fatalf("lower-case scheme: %d %s", r.status, r.body)
	}
	// Keys cannot manage keys, regardless of scopes.
	k := h.bearer(raw)
	for _, p := range [][2]string{{"GET", "/api/v1/developer/api-keys"}, {"POST", "/api/v1/developer/api-keys"}, {"GET", "/api/v1/audit-logs"},
		{"GET", "/api/v1/developer/mcp-connections"}, {"GET", "/api/v1/developer/usage"}} {
		if r := k.req(p[0], p[1], map[string]any{"name": "x"}); r.status != 403 {
			t.Errorf("api key on %s %s: want 403, got %d %s", p[0], p[1], r.status, r.body)
		}
	}
}

// ------------------------------------------------------------------ errors

func TestErrorEnvelopeAndRequestIDs(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("errors@example.com")
	raw := a.req("POST", "/api/v1/developer/api-keys", map[string]any{"name": "ro", "scopes": []string{"posts:read"}}).obj(t)["key"].(string)
	ro := h.bearer(raw)
	ghost := uuid.NewString()

	cases := []struct {
		name   string
		r      reply
		status int
		code   string
	}{
		{"validation", a.req("POST", "/api/v1/posts", map[string]any{"title": strings.Repeat("t", 300)}), 400, "VALIDATION_ERROR"},
		{"malformed json", a.req("POST", "/api/v1/posts", `{"content":`), 400, "VALIDATION_ERROR"},
		{"unauthenticated", h.anon().req("GET", "/api/v1/posts", nil), 401, "UNAUTHENTICATED"},
		{"csrf", a.req("POST", "/api/v1/posts", map[string]any{"content": "x"}, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }), 403, "FORBIDDEN"},
		{"scope", ro.req("POST", "/api/v1/posts", map[string]any{"content": "x"}), 403, "INSUFFICIENT_SCOPE"},
		{"not found resource", a.req("GET", "/api/v1/posts/"+ghost, nil), 404, "NOT_FOUND"},
		{"not found bad id", a.req("GET", "/api/v1/posts/not-a-uuid", nil), 404, "NOT_FOUND"},
		{"not found route", a.req("GET", "/api/v1/nothing-here", nil), 404, "NOT_FOUND"},
		{"method not allowed", a.req("PUT", "/api/v1/posts", map[string]any{}), 404, "NOT_FOUND"},
		{"not implemented", a.req("GET", "/api/v1/social/instagram/connect", nil), 501, "PROVIDER_NOT_AVAILABLE"},
	}
	for _, c := range cases {
		e := c.r.apiErr(t, c.status, c.code)
		if c.name == "validation" && e["fields"] == nil {
			t.Errorf("validation error lacks fields: %v", e)
		}
	}
	// Scope errors name the missing scope.
	if e := cases[4].r.apiErr(t, 403, "INSUFFICIENT_SCOPE"); !strings.Contains(e["message"].(string), "posts:write") {
		t.Errorf("scope message: %v", e["message"])
	}
	// Conflict: invalid transition.
	pid := a.req("POST", "/api/v1/posts", map[string]any{"content": "x"}).obj(t)["id"].(string)
	a.req("POST", "/api/v1/posts/"+pid+"/retry", nil).apiErr(t, 409, "INVALID_STATE_TRANSITION")
	// Nothing internal ever leaks.
	for _, c := range cases {
		for _, bad := range []string{"pgx", "SQLSTATE", "goroutine", "panic", ".go:", "select ", "/home/"} {
			if strings.Contains(strings.ToLower(string(c.r.body)), strings.ToLower(bad)) {
				t.Errorf("%s leaks %q: %s", c.name, bad, c.r.body)
			}
		}
	}

	// Request ids: inbound values are echoed when sane and replaced when not.
	idRe := regexp.MustCompile(`^[0-9a-f-]{36}$`)
	for _, tc := range []struct {
		header, value string
		echoed        bool
	}{
		{"X-Request-ID", "abc-123_DEF.456:x", true},
		{"X-Correlation-ID", "corr-42", true},
		{"X-Request-ID", "has space", false},
		{"X-Request-ID", "inject\nX-Evil: 1", false},
		{"X-Request-ID", strings.Repeat("a", 65), false},
		{"X-Request-ID", "<script>", false},
	} {
		r := a.req("GET", "/api/v1/posts/"+ghost, nil, func(r *http.Request) {
			if strings.Contains(tc.value, "\n") {
				return // Go refuses to send header injection at all
			}
			r.Header.Set(tc.header, tc.value)
		})
		e := r.apiErr(t, 404, "NOT_FOUND")
		got := e["request_id"].(string)
		if tc.echoed && got != tc.value {
			t.Errorf("%s %q not echoed: %q", tc.header, tc.value, got)
		}
		if !tc.echoed && !idRe.MatchString(got) && !strings.Contains(tc.value, "\n") {
			t.Errorf("unsafe request id %q replaced by %q, want a uuid", tc.value, got)
		}
		if r.header.Get("X-Evil") != "" {
			t.Errorf("header injection through request id")
		}
	}
	// Two requests never share an id.
	r1, r2 := a.req("GET", "/health", nil), a.req("GET", "/health", nil)
	if r1.header.Get("X-Request-ID") == "" || r1.header.Get("X-Request-ID") == r2.header.Get("X-Request-ID") {
		t.Errorf("request ids must be unique: %q %q", r1.header.Get("X-Request-ID"), r2.header.Get("X-Request-ID"))
	}
}

type brokenStore struct{ media.Storage }

func (brokenStore) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("dial tcp 10.1.2.3:9000: connect: connection refused (access key AKIA-SECRET)")
}

// Unexpected failures surface as 500 INTERNAL with a generic message.
func TestInternalErrorsAreOpaque(t *testing.T) {
	h := newHarness(t, opts{store: brokenStore{}})
	a := h.anon()
	a.register("internal@example.com")
	r := a.upload("a.png", pngBytes(t, 4, 4))
	e := r.apiErr(t, 500, "INTERNAL")
	if e["message"] != "internal server error" {
		t.Errorf("message: %v", e["message"])
	}
	if strings.Contains(string(r.body), "10.1.2.3") || strings.Contains(string(r.body), "AKIA") {
		t.Errorf("internal details leaked: %s", r.body)
	}
	// Nothing half-created: the failed upload left no media row.
	if items := a.req("GET", "/api/v1/media", nil).obj(t)["items"].([]any); len(items) != 0 {
		t.Errorf("failed upload left %d rows", len(items))
	}
}

// ------------------------------------------------------------------ media

func pngBytes(t testing.TB, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t testing.TB) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 6)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifBytes(t testing.TB) []byte {
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, 5, 7), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func webpBytes(t testing.TB) []byte {
	b, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// An ISO base media file header: ftyp box with the mp42 brand.
var mp4Header = []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}

var movHeader = []byte{0, 0, 0, 0x14, 'f', 't', 'y', 'p', 'q', 't', ' ', ' ', 0, 0, 0, 0, 'q', 't', ' ', ' '}

func mustParse(t testing.TB, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// zeros streams n zero bytes.
type zeros struct{ n int64 }

func (z *zeros) Read(p []byte) (int, error) {
	if z.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > z.n {
		p = p[:z.n]
	}
	clear(p)
	z.n -= int64(len(p))
	return len(p), nil
}

// uploadStream posts a multipart upload whose file part is head followed by pad zero bytes,
// without ever holding the whole body in memory.
func (a *agent) uploadStream(field, filename string, head []byte, pad int64) reply {
	a.h.t.Helper()
	pr, pwr := io.Pipe()
	mw := multipart.NewWriter(pwr)
	go func() {
		part, err := mw.CreateFormFile(field, filename)
		if err == nil {
			if _, err = part.Write(head); err == nil {
				_, err = io.Copy(part, &zeros{n: pad})
			}
		}
		if err == nil {
			err = mw.Close()
		}
		_ = pwr.CloseWithError(err)
	}()
	r, _ := http.NewRequest("POST", a.h.srv.URL+"/api/v1/media", pr)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("X-CSRF-Token", a.csrf)
	if a.bearer != "" {
		r.Header.Set("Authorization", "Bearer "+a.bearer)
	}
	res, err := a.hc.Do(r)
	if err != nil {
		_ = pr.Close()
		a.h.t.Fatalf("stream upload: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	_ = pr.Close()
	return reply{status: res.StatusCode, header: res.Header, body: b}
}

func (a *agent) upload(name string, data []byte) reply {
	a.h.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	return a.rawMultipart(mw.FormDataContentType(), &body)
}

func (a *agent) rawMultipart(contentType string, body io.Reader) reply {
	a.h.t.Helper()
	return a.req("POST", "/api/v1/media", nil, func(r *http.Request) {
		r.Body = io.NopCloser(body)
		r.Header.Set("Content-Type", contentType)
	})
}

func TestMediaUploadAcceptsSupportedTypes(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	me := a.register("media-ok@example.com")
	uid := me["id"].(string)

	for _, tc := range []struct {
		name, file, mime, kind string
		data                   []byte
		w, h                   float64
	}{
		{"png", "a.png", "image/png", "image", pngBytes(t, 4, 3), 4, 3},
		{"jpeg", "a.jpg", "image/jpeg", "image", jpegBytes(t), 8, 6},
		{"gif", "a.gif", "image/gif", "image", gifBytes(t), 5, 7},
		{"webp", "a.webp", "image/webp", "image", webpBytes(t), 0, 0},
		{"mp4", "clip.mp4", "video/mp4", "video", append(append([]byte{}, mp4Header...), bytes.Repeat([]byte{0}, 64)...), 0, 0},
		{"mov", "clip.mov", "video/quicktime", "video", append(append([]byte{}, movHeader...), bytes.Repeat([]byte{0}, 64)...), 0, 0},
		// The extension and any declared type are irrelevant: the bytes decide.
		{"png named .txt", "notes.txt", "image/png", "image", pngBytes(t, 2, 2), 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := a.upload(tc.file, tc.data)
			if r.status != 201 {
				t.Fatalf("%d %s", r.status, r.body)
			}
			m := r.obj(t)
			if m["mime_type"] != tc.mime || m["kind"] != tc.kind || m["size_bytes"].(float64) != float64(len(tc.data)) || m["status"] != "ready" {
				t.Fatalf("media: %v", m)
			}
			if tc.kind == "image" && tc.w > 0 && (m["width"].(float64) != tc.w || m["height"].(float64) != tc.h) {
				t.Fatalf("dimensions: %v", m)
			}
			if u, _ := m["url"].(string); u == "" {
				t.Fatalf("response lacks a download url: %v", m)
			}
			if len(m["sha256"].(string)) != 64 || m["original_name"] != tc.file {
				t.Fatalf("hash/name: %v", m)
			}
			key := fmt.Sprintf("users/%s/media/%s", uid, m["id"])
			ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp", "video/mp4": ".mp4", "video/quicktime": ".mov"}[tc.mime]
			if !h.mem.Has(key + ext) {
				t.Fatalf("object %s%s missing from storage", key, ext)
			}
			if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT[\d:.]+Z$`).MatchString(m["created_at"].(string)) {
				t.Fatalf("created_at must be RFC 3339 UTC: %v", m["created_at"])
			}
		})
	}
}

func TestMediaUploadRejectsBadFiles(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("media-bad@example.com")

	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script></svg>`)
	for name, tc := range map[string]struct {
		file string
		data []byte
	}{
		"html as png":          {"photo.png", []byte("<!DOCTYPE html><html><script>alert(1)</script></html>")},
		"shell script as jpg":  {"photo.jpg", []byte("#!/bin/sh\nrm -rf /\n")},
		"ELF binary as png":    {"cat.png", append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}, bytes.Repeat([]byte{0}, 200)...)},
		"PDF as mp4":           {"movie.mp4", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")},
		"svg as png":           {"logo.png", svg},
		"zip as png":           {"x.png", append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 64)...)},
		"plain text":           {"a.png", []byte("just some text")},
		"json as jpg":          {"a.jpg", []byte(`{"a":1}`)},
		"png magic, no body":   {"a.png", []byte("\x89PNG\r\n\x1a\n")},
		"truncated jpeg":       {"a.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}},
		"corrupt png":          {"a.png", append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0xAB}, 100)...)},
		"zero-byte file":       {"empty.png", nil},
		"webm video":           {"a.webm", append([]byte{0x1A, 0x45, 0xDF, 0xA3}, bytes.Repeat([]byte{0}, 64)...)},
		"windows executable":   {"a.png", append([]byte("MZ"), bytes.Repeat([]byte{0}, 64)...)},
		"bmp not in allowlist": {"a.png", append([]byte("BM"), bytes.Repeat([]byte{0}, 64)...)},
	} {
		t.Run(name, func(t *testing.T) {
			r := a.upload(tc.file, tc.data)
			e := r.apiErr(t, 400, "VALIDATION_ERROR")
			if name != "zero-byte file" && e["fields"] == nil {
				t.Errorf("no field details: %v", e)
			}
		})
	}
	// The pixel-bomb guard: tiny file, enormous declared dimensions.
	bomb := pngBytes(t, 1, 1)
	copy(bomb[16:20], []byte{0x00, 0x01, 0x00, 0x00}) // width 65536
	copy(bomb[20:24], []byte{0x00, 0x01, 0x00, 0x00}) // height 65536
	a.upload("bomb.png", bomb).apiErr(t, 400, "VALIDATION_ERROR")

	// Nothing was stored or recorded by the rejected uploads.
	if items := a.req("GET", "/api/v1/media", nil).obj(t)["items"].([]any); len(items) != 0 {
		t.Fatalf("rejected uploads left %d media rows", len(items))
	}
}

func TestMediaUploadRequestShape(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("media-shape@example.com")

	// Not multipart at all.
	a.req("POST", "/api/v1/media", `{"file":"abc"}`).apiErr(t, 400, "VALIDATION_ERROR")
	a.req("POST", "/api/v1/media", nil).apiErr(t, 400, "VALIDATION_ERROR")
	a.rawMultipart("multipart/form-data", strings.NewReader("garbage")).apiErr(t, 400, "VALIDATION_ERROR")
	a.rawMultipart("multipart/form-data; boundary=zzz", strings.NewReader("--zzz\r\nbroken")).apiErr(t, 400, "VALIDATION_ERROR")
	// Wrong field name, or a text field instead of a file.
	for name, field := range map[string]string{"image": "image", "empty name": "", "FILE casing": "File"} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile(field, "a.png")
		_, _ = fw.Write(pngBytes(t, 2, 2))
		_ = mw.Close()
		e := a.rawMultipart(mw.FormDataContentType(), &body).apiErr(t, 400, "VALIDATION_ERROR")
		if f, _ := e["fields"].(map[string]any); f["file"] == nil {
			t.Errorf("%s: should point at the 'file' field: %v", name, e)
		}
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("file", "i am text, not a file")
	_ = mw.Close()
	a.rawMultipart(mw.FormDataContentType(), &body).apiErr(t, 400, "VALIDATION_ERROR")

	// With several file parts only the first named "file" is used, and the others are ignored (never stored).
	body.Reset()
	mw = multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "first.png")
	_, _ = fw.Write(pngBytes(t, 2, 2))
	fw, _ = mw.CreateFormFile("file", "second.png")
	_, _ = fw.Write(pngBytes(t, 3, 3))
	_ = mw.Close()
	r := a.rawMultipart(mw.FormDataContentType(), &body)
	if r.status != 201 || r.obj(t)["original_name"] != "first.png" {
		t.Fatalf("multiple parts: %d %s", r.status, r.body)
	}
}

func TestMediaFilenameSanitising(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	me := a.register("media-name@example.com")
	png := pngBytes(t, 2, 2)
	for in, want := range map[string]string{
		"../../etc/passwd.png":            "passwd.png",
		`C:\Users\me\photo.png`:           "photo.png",
		"/abs/path/pic.png":               "pic.png",
		"tab\there.png":                   "tabhere.png",
		"héllo wörld 日本.png":              "héllo wörld 日本.png",
		strings.Repeat("n", 300) + ".png": strings.Repeat("n", 200),
	} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, in))
		hdr.Set("Content-Type", "image/png")
		part, _ := mw.CreatePart(hdr)
		_, _ = part.Write(png)
		_ = mw.Close()
		r := a.rawMultipart(mw.FormDataContentType(), &body)
		if r.status != 201 {
			t.Fatalf("%q: %d %s", in, r.status, r.body)
		}
		m := r.obj(t)
		got := m["original_name"].(string)
		if got != want && (len(in) <= 250 || len([]rune(got)) > 200) {
			t.Errorf("filename %q sanitised to %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, "/\\\x00\t\n") {
			t.Errorf("unsafe characters survived in %q", got)
		}
		// The object key never contains user input.
		if !h.mem.Has(fmt.Sprintf("users/%s/media/%s.png", me["id"], m["id"])) {
			t.Errorf("storage key must be derived from ids only")
		}
	}
}

func TestMediaSizeLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("streams 100 MB bodies")
	}
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("media-size@example.com")
	const mb = 1 << 20
	pngHead := pngBytes(t, 4, 4)

	// Images: 10 MB is the limit, byte-exact.
	r := a.uploadStream("file", "limit.png", pngHead, 10*mb-int64(len(pngHead)))
	if r.status != 201 || r.obj(t)["size_bytes"].(float64) != 10*mb {
		t.Fatalf("image of exactly 10 MB: %d %.200s", r.status, r.body)
	}
	e := a.uploadStream("file", "big.png", pngHead, 10*mb-int64(len(pngHead))+1).apiErr(t, 400, "VALIDATION_ERROR")
	if !strings.Contains(e["message"].(string), "10 MB") {
		t.Errorf("message should state the limit: %v", e["message"])
	}
	// Videos: 100 MB limit.
	e = a.uploadStream("file", "big.mp4", mp4Header, 100*mb-int64(len(mp4Header))+1).apiErr(t, 400, "VALIDATION_ERROR")
	if !strings.Contains(e["message"].(string), "100 MB") {
		t.Errorf("message should state the limit: %v", e["message"])
	}
	// Far beyond any limit the request is cut off instead of being buffered.
	e = a.uploadStream("file", "huge.mp4", mp4Header, 130*mb).apiErr(t, 400, "VALIDATION_ERROR")
	if f, _ := e["fields"].(map[string]any); f["file"] != "too large" {
		t.Errorf("oversize body: %v", e)
	}
	// Rejected oversize uploads leave nothing behind; only the 10 MB image exists.
	if items := a.req("GET", "/api/v1/media", nil).obj(t)["items"].([]any); len(items) != 1 {
		t.Fatalf("expected only the accepted image, got %d rows", len(items))
	}
}

func TestMediaScopesAndLifecycle(t *testing.T) {
	h := newHarness(t, opts{})
	owner, other := h.anon(), h.anon()
	owner.register("media-own@example.com")
	other.register("media-other@example.com")
	mkKey := func(scopes ...string) *agent {
		raw := owner.req("POST", "/api/v1/developer/api-keys", map[string]any{"name": "k", "scopes": scopes}).obj(t)["key"].(string)
		return h.bearer(raw)
	}
	reader, writer := mkKey("posts:read"), mkKey("media:write", "posts:read")
	noMedia := mkKey("social:read")

	// Upload needs media:write.
	e := reader.upload("a.png", pngBytes(t, 2, 2)).apiErr(t, 403, "INSUFFICIENT_SCOPE")
	if !strings.Contains(e["message"].(string), "media:write") {
		t.Errorf("message: %v", e["message"])
	}
	reader.req("DELETE", "/api/v1/media/"+uuid.NewString(), nil).apiErr(t, 403, "INSUFFICIENT_SCOPE")
	// The scope check comes before the body is parsed or any bytes are stored.
	reader.rawMultipart("multipart/form-data; boundary=x", strings.NewReader("junk")).apiErr(t, 400, "VALIDATION_ERROR")

	up := writer.upload("k.png", pngBytes(t, 3, 3))
	if up.status != 201 {
		t.Fatalf("key upload: %d %s", up.status, up.body)
	}
	id := up.obj(t)["id"].(string)

	// Reading needs posts:read.
	noMedia.req("GET", "/api/v1/media", nil).apiErr(t, 403, "INSUFFICIENT_SCOPE")
	noMedia.req("GET", "/api/v1/media/"+id, nil).apiErr(t, 403, "INSUFFICIENT_SCOPE")
	if r := reader.req("GET", "/api/v1/media/"+id, nil); r.status != 200 || r.obj(t)["url"] == "" {
		t.Fatalf("read via key: %d %s", r.status, r.body)
	}

	// Tenant isolation: another user gets 404 everywhere and cannot delete the object.
	other.req("GET", "/api/v1/media/"+id, nil).apiErr(t, 404, "NOT_FOUND")
	other.req("DELETE", "/api/v1/media/"+id, nil).apiErr(t, 404, "NOT_FOUND")
	if items := other.req("GET", "/api/v1/media", nil).obj(t)["items"].([]any); len(items) != 0 {
		t.Fatalf("other tenant lists %d media", len(items))
	}
	other.req("GET", "/api/v1/media/not-a-uuid", nil).apiErr(t, 404, "NOT_FOUND")

	// Pagination of the media list.
	for i := 0; i < 2; i++ {
		owner.upload(fmt.Sprintf("p%d.png", i), pngBytes(t, 2+i, 2))
	}
	page := owner.req("GET", "/api/v1/media?limit=2", nil).obj(t)
	if len(page["items"].([]any)) != 2 || page["next_cursor"] == nil {
		t.Fatalf("first page: %v", page)
	}
	next := owner.req("GET", "/api/v1/media?limit=2&cursor="+page["next_cursor"].(string), nil).obj(t)
	if len(next["items"].([]any)) != 1 || next["next_cursor"] != nil {
		t.Fatalf("second page: %v", next)
	}
	owner.req("GET", "/api/v1/media?limit=0", nil).apiErr(t, 400, "VALIDATION_ERROR")
	owner.req("GET", "/api/v1/media?cursor=garbage", nil).apiErr(t, 400, "VALIDATION_ERROR")

	// Deleting removes the row and the stored object; media attached to a post is protected.
	uid := owner.req("GET", "/api/v1/me", nil).obj(t)["id"].(string)
	key := fmt.Sprintf("users/%s/media/%s.png", uid, id)
	if !h.mem.Has(key) {
		t.Fatalf("object %s should exist", key)
	}
	post := owner.req("POST", "/api/v1/posts", map[string]any{"content": "x", "media_ids": []string{id}})
	if post.status != 201 {
		t.Fatalf("post with media: %d %s", post.status, post.body)
	}
	owner.req("DELETE", "/api/v1/media/"+id, nil).apiErr(t, 409, "CONFLICT")
	if !h.mem.Has(key) {
		t.Fatal("a refused delete must keep the object")
	}
	owner.req("DELETE", "/api/v1/posts/"+post.obj(t)["id"].(string), nil)
	if r := owner.req("DELETE", "/api/v1/media/"+id, nil); r.status != 204 {
		t.Fatalf("delete after the post is gone: %d %s", r.status, r.body)
	}
	if h.mem.Has(key) {
		t.Fatal("deleting media must remove the stored object")
	}
	owner.req("GET", "/api/v1/media/"+id, nil).apiErr(t, 404, "NOT_FOUND")
	owner.req("DELETE", "/api/v1/media/"+id, nil).apiErr(t, 404, "NOT_FOUND")
}

// ------------------------------------------------------------------ headers, CORS, bodies

func TestSecurityHeadersAndCORS(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("headers@example.com")
	for _, p := range []string{"/health", "/api/v1/me", "/api/v1/nope"} {
		r := a.req("GET", p, nil)
		for k, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store"} {
			if got := r.header.Get(k); got != want {
				t.Errorf("%s %s: %s = %q, want %q", "GET", p, k, got, want)
			}
		}
		if !strings.Contains(r.header.Get("Content-Security-Policy"), "default-src 'none'") {
			t.Errorf("%s: CSP %q", p, r.header.Get("Content-Security-Policy"))
		}
	}
	preflight := func(origin, method string) reply {
		return a.req("OPTIONS", "/api/v1/posts", nil, func(r *http.Request) {
			r.Header.Set("Origin", origin)
			r.Header.Set("Access-Control-Request-Method", method)
			r.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token,authorization")
		})
	}
	ok := preflight("http://web.test", "POST")
	if ok.status != 200 && ok.status != 204 {
		t.Fatalf("preflight: %d", ok.status)
	}
	if ok.header.Get("Access-Control-Allow-Origin") != "http://web.test" || ok.header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed origin: %v", ok.header)
	}
	if allow := strings.ToLower(ok.header.Get("Access-Control-Allow-Headers")); !strings.Contains(allow, "x-csrf-token") || !strings.Contains(allow, "authorization") {
		t.Errorf("allowed headers: %q", allow)
	}
	for _, evil := range []string{"http://evil.example", "http://web.test.evil.example", "null", "https://web.test"} {
		r := preflight(evil, "POST")
		if got := r.header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %q must not be allowed, got %q", evil, got)
		}
	}
	if got := preflight("http://web.test", "POST").header.Get("Access-Control-Allow-Origin"); got == "*" {
		t.Error("wildcard origin with credentials")
	}
	// Actual requests expose the headers the frontend reads.
	r := a.req("GET", "/api/v1/me", nil, func(r *http.Request) { r.Header.Set("Origin", "http://web.test") })
	if !strings.Contains(r.header.Get("Access-Control-Expose-Headers"), "X-Request-Id") {
		t.Errorf("expose headers: %q", r.header.Get("Access-Control-Expose-Headers"))
	}
}

func TestJSONBodyLimits(t *testing.T) {
	h := newHarness(t, opts{})
	a := h.anon()
	a.register("body@example.com")
	// 1 MiB cap on JSON bodies.
	big := `{"content":"` + strings.Repeat("x", 2<<20) + `"}`
	if r := a.req("POST", "/api/v1/posts", big); r.status != 400 {
		t.Errorf("2 MiB body accepted: %d %.100s", r.status, r.body)
	}
	// The post content limit (10,000) is enforced by characters, not bytes.
	if r := a.req("POST", "/api/v1/posts", map[string]any{"content": strings.Repeat("日", 10000)}); r.status != 201 {
		t.Errorf("10000 multibyte characters should fit: %d %.150s", r.status, r.body)
	}
	a.req("POST", "/api/v1/posts", map[string]any{"content": strings.Repeat("日", 10001)}).apiErr(t, 400, "VALIDATION_ERROR")
	// Unicode and control characters survive a round trip untouched.
	txt := "emoji 🚀 – zero\u200bwidth – RTL שלום – \"quotes\" <b>html</b> & ampersand"
	p := a.req("POST", "/api/v1/posts", map[string]any{"content": txt}).obj(t)
	got := a.req("GET", "/api/v1/posts/"+p["id"].(string), nil).obj(t)["content"]
	if got != txt {
		t.Errorf("content altered: %q != %q", got, txt)
	}
	if ct := a.req("GET", "/api/v1/posts/"+p["id"].(string), nil).header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type %q", ct)
	}
}
