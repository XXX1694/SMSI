package e2e

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/infrastructure/postgres"
)

func TestAPIKeyScopes(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	c.register("keys@example.com")
	acc := c.connectMock()
	draft := c.must("POST", "/api/v1/posts", map[string]any{"content": "agent post", "social_account_ids": []string{acc}}, 201)
	postID := draft["id"].(string)

	raw := c.createKey("agent", "posts:read", "posts:write", "social:read")
	if !strings.HasPrefix(raw, "sk_live_") {
		t.Fatalf("bad key %q", raw)
	}
	k := e.apiKeyClient(raw)

	me := k.must("GET", "/api/v1/me", nil, 200)
	if me["auth_type"] != "api_key" || len(me["scopes"].([]any)) != 3 || me["csrf_token"] != nil {
		t.Fatalf("unexpected /me for key: %v", me)
	}
	k.must("GET", "/api/v1/posts", nil, 200)
	// API keys are CSRF-exempt and can write drafts with posts:write.
	k.must("POST", "/api/v1/posts", map[string]any{"content": "from agent", "social_account_ids": []string{acc}}, 201)

	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/posts/" + postID + "/publish"},
		{"DELETE", "/api/v1/posts/" + postID},
		{"DELETE", "/api/v1/social/accounts/" + acc},
		{"POST", "/api/v1/posts/" + postID + "/schedule"},
		{"GET", "/api/v1/analytics"},
	} {
		body := map[string]any{"scheduled_at": "2099-01-01T00:00:00Z"}
		r := k.do(tc.method, tc.path, body)
		if r.status != http.StatusForbidden || r.errCode(t) != "INSUFFICIENT_SCOPE" {
			t.Errorf("%s %s: want 403 INSUFFICIENT_SCOPE, got %d %s", tc.method, tc.path, r.status, r.body)
		}
	}
	// API keys can never manage API keys.
	r := k.do("POST", "/api/v1/developer/api-keys", map[string]any{"name": "escalate", "scopes": []string{"posts:publish"}})
	if r.status != http.StatusForbidden || r.errCode(t) != "FORBIDDEN" {
		t.Fatalf("key minted a key: %d %s", r.status, r.body)
	}
	// A key with posts:publish may publish.
	pub := e.apiKeyClient(c.createKey("publisher", "posts:publish", "posts:read"))
	pub.must("POST", "/api/v1/posts/"+postID+"/publish", map[string]any{"confirm": true}, 202)

	// Every API-key request is audited with actor api_key.
	logs := c.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)["items"].([]any)
	var keyEntries int
	for _, l := range logs {
		m := l.(map[string]any)
		if m["actor_type"] == "api_key" && m["action"] == "api_key.request" {
			keyEntries++
		}
	}
	if keyEntries < 8 {
		t.Fatalf("expected api key request audit entries, got %d", keyEntries)
	}
	usage := c.must("GET", "/api/v1/developer/usage", nil, 200)["items"].([]any)
	if len(usage) != 2 {
		t.Fatalf("usage: %v", usage)
	}

	// Revoked keys stop working immediately.
	keys := c.must("GET", "/api/v1/developer/api-keys", nil, 200)["items"].([]any)
	for _, kk := range keys {
		c.must("DELETE", "/api/v1/developer/api-keys/"+kk.(map[string]any)["id"].(string), nil, 204)
	}
	if r := k.do("GET", "/api/v1/posts", nil); r.status != http.StatusUnauthorized || r.errCode(t) != "UNAUTHENTICATED" {
		t.Fatalf("revoked key still works: %d", r.status)
	}
	if r := e.apiKeyClient("sk_live_notarealkey").do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("garbage key accepted: %d", r.status)
	}
}

func TestMCPConnection(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	c.register("mcp@example.com")
	m := c.must("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": "Claude Desktop", "client_name": "claude"}, 201)
	raw := m["key"].(string)
	cfg := m["config"].(map[string]any)["mcpServers"].(map[string]any)["socialos"].(map[string]any)
	if cfg["url"] != "http://mcp.test/mcp" || !strings.Contains(cfg["headers"].(map[string]any)["Authorization"].(string), raw) {
		t.Fatalf("bad config %v", cfg)
	}
	k := e.apiKeyClient(raw)
	scopes := k.must("GET", "/api/v1/me", nil, 200)["scopes"].([]any)
	for _, s := range scopes {
		if s == "posts:publish" || s == "posts:delete" || s == "social:disconnect" {
			t.Fatalf("default MCP scopes include dangerous %s", s)
		}
	}
	list := c.must("GET", "/api/v1/developer/mcp-connections", nil, 200)["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["last_seen_at"] == nil {
		t.Fatalf("connection not listed / last_seen not updated: %v", list)
	}
	c.must("DELETE", "/api/v1/developer/mcp-connections/"+list[0].(map[string]any)["id"].(string), nil, 204)
	if r := k.do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("revoked MCP key works: %d", r.status)
	}
}

func TestCSRFAndAuthErrors(t *testing.T) {
	e := newEnv(t, envOpts{})
	anon := e.browser()
	r := anon.do("GET", "/api/v1/posts", nil)
	if r.status != 401 || r.errCode(t) != "UNAUTHENTICATED" || r.json(t)["error"].(map[string]any)["request_id"] == "" {
		t.Fatalf("anon access: %d %s", r.status, r.body)
	}
	c := e.browser()
	c.register("csrf@example.com")
	token := c.csrf
	c.csrf = ""
	if r := c.do("POST", "/api/v1/posts", map[string]any{"content": "x"}); r.status != 403 || r.errCode(t) != "FORBIDDEN" {
		t.Fatalf("missing CSRF accepted: %d %s", r.status, r.body)
	}
	c.csrf = "wrong"
	if r := c.do("POST", "/api/v1/posts", map[string]any{"content": "x"}); r.status != 403 {
		t.Fatalf("wrong CSRF accepted: %d", r.status)
	}
	c.csrf = token
	c.must("POST", "/api/v1/posts", map[string]any{"content": "draft without accounts"}, 201)

	if r := c.do("POST", "/api/v1/auth/register", map[string]any{"email": "csrf@example.com", "password": "another long password"}); r.status != 409 {
		t.Fatalf("duplicate email: %d %s", r.status, r.body)
	}
	if r := anon.do("POST", "/api/v1/auth/login", map[string]any{"email": "csrf@example.com", "password": "wrong password!!"}); r.status != 401 {
		t.Fatalf("bad login: %d", r.status)
	}
	if r := anon.do("POST", "/api/v1/auth/register", map[string]any{"email": "bad", "password": "short"}); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
		t.Fatalf("validation: %d %s", r.status, r.body)
	}
	c.must("POST", "/api/v1/auth/logout", nil, 204)
	if r := c.do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("session survived logout: %d", r.status)
	}
	login := anon.must("POST", "/api/v1/auth/login", map[string]any{"email": "CSRF@example.com", "password": "correct horse battery"}, 200)
	if login["user"].(map[string]any)["email"] != "csrf@example.com" {
		t.Fatalf("login: %v", login)
	}
	if r := anon.do("GET", "/health", nil); r.status != 200 || r.header.Get("X-Request-ID") == "" {
		t.Fatalf("health: %d", r.status)
	}
	if r := anon.do("GET", "/ready", nil); r.status != 200 {
		t.Fatalf("ready: %d %s", r.status, r.body)
	}
	if r := anon.do("GET", "/metrics", nil); r.status != 200 || !strings.Contains(string(r.body), "socialos_http_requests_total") {
		t.Fatalf("metrics: %d", r.status)
	}
}

func TestTenantIsolationHTTP(t *testing.T) {
	e := newEnv(t, envOpts{})
	alice, bob := e.browser(), e.browser()
	alice.register("alice@tenant.test")
	bob.register("bob@tenant.test")
	acc := alice.connectMock()
	p := alice.must("POST", "/api/v1/posts", map[string]any{"content": "private", "social_account_ids": []string{acc}}, 201)
	id := p["id"].(string)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/posts/" + id}, {"PATCH", "/api/v1/posts/" + id}, {"DELETE", "/api/v1/posts/" + id},
		{"POST", "/api/v1/posts/" + id + "/publish"}, {"GET", "/api/v1/social/accounts/" + acc}, {"DELETE", "/api/v1/social/accounts/" + acc},
	} {
		if r := bob.do(tc.method, tc.path, map[string]any{"content": "hijack"}); r.status != 404 {
			t.Errorf("bob %s %s: want 404 got %d %s", tc.method, tc.path, r.status, r.body)
		}
	}
	if r := bob.do("POST", "/api/v1/posts", map[string]any{"content": "x", "social_account_ids": []string{acc}}); r.status != 400 {
		t.Fatalf("bob used alice's account: %d %s", r.status, r.body)
	}
	if n := len(bob.must("GET", "/api/v1/posts", nil, 200)["items"].([]any)); n != 0 {
		t.Fatalf("bob sees %d posts", n)
	}
	if n := len(bob.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)); n != 0 {
		t.Fatalf("bob sees %d accounts", n)
	}
}

// A post target is only ever reached through its post, and every route is keyed by the post id. Bob must not be able
// to read, change or lock Alice's target by any of them, nor by presenting her target id where a post id is expected.
func TestTargetIsolation(t *testing.T) {
	e := newEnv(t, envOpts{})
	alice, bob := e.browser(), e.browser()
	aliceID := alice.register("alice@target.test")["user"].(map[string]any)["id"].(string)
	bobID := bob.register("bob@target.test")["user"].(map[string]any)["id"].(string)
	acc := alice.connectMock()
	p := alice.must("POST", "/api/v1/posts", map[string]any{"content": "alice only", "social_account_ids": []string{acc}}, 201)
	postID := p["id"].(string)
	targetID := p["targets"].([]any)[0].(map[string]any)["id"].(string)

	for _, id := range []string{postID, targetID} { // Alice's post id, and her target id passed as if it were a post id
		for _, tc := range []struct{ method, path string }{
			{"GET", "/api/v1/posts/" + id}, {"PATCH", "/api/v1/posts/" + id}, {"DELETE", "/api/v1/posts/" + id},
			{"GET", "/api/v1/posts/" + id + "/status"}, {"POST", "/api/v1/posts/" + id + "/publish"},
			{"POST", "/api/v1/posts/" + id + "/schedule"}, {"POST", "/api/v1/posts/" + id + "/unschedule"},
			{"POST", "/api/v1/posts/" + id + "/cancel"}, {"POST", "/api/v1/posts/" + id + "/retry"},
		} {
			// A valid body, so that a 404 can only come from the ownership check.
			body := map[string]any{"content": "hijack", "scheduled_at": time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339), "confirm": true}
			if r := bob.do(tc.method, tc.path, body); r.status != http.StatusNotFound {
				t.Errorf("bob %s %s: want 404 got %d %s", tc.method, tc.path, r.status, r.body)
			}
		}
	}
	// None of that touched Alice's post or its target.
	got := alice.must("GET", "/api/v1/posts/"+postID, nil, 200)
	gt := got["targets"].([]any)[0].(map[string]any)
	if got["status"] != "draft" || got["content"] != "alice only" || gt["id"] != targetID || gt["status"] != "pending" || gt["content"] != "alice only" {
		t.Fatalf("alice's post changed: %v", got)
	}
	if n := len(bob.must("GET", "/api/v1/posts", nil, 200)["items"].([]any)); n != 0 {
		t.Fatalf("bob lists %d posts", n)
	}

	// The same holds below the HTTP layer, for every repository method that takes a user.
	ctx, repo := context.Background(), postgres.NewPosts(e.app.DB)
	alicePost, aliceUser, bobUser := uuid.MustParse(postID), uuid.MustParse(aliceID), uuid.MustParse(bobID)
	tid := uuid.MustParse(targetID)
	if ts, err := repo.ListTargets(ctx, bobUser, alicePost); err != nil || len(ts) != 0 {
		t.Fatalf("bob lists alice's targets: %v %v", ts, err)
	}
	if as, err := repo.Attempts(ctx, bobUser, alicePost); err != nil || len(as) != 0 {
		t.Fatalf("bob lists alice's attempts: %v %v", as, err)
	}
	if err := repo.LockTargetWait(ctx, bobUser, tid); !errs.Is(err, errs.NotFound) {
		t.Fatalf("bob locked alice's target: %v", err)
	}
	if err := repo.LockTargetWait(ctx, aliceUser, tid); err != nil {
		t.Fatalf("the owner must be able to lock: %v", err)
	}
	ts, err := repo.ListTargets(ctx, aliceUser, alicePost)
	if err != nil || len(ts) != 1 {
		t.Fatalf("alice's targets: %v %v", ts, err)
	}
	forged := ts[0]
	forged.UserID, forged.Content, forged.Status = bobUser, "forged", post.TargetPublished
	if err := repo.UpdateTarget(ctx, &forged); !errs.Is(err, errs.NotFound) {
		t.Fatalf("bob updated alice's target: %v", err)
	}
	var content, status string
	if err := e.app.DB.Pool.QueryRow(ctx, `SELECT content, status FROM post_targets WHERE id = $1`, tid).Scan(&content, &status); err != nil ||
		content != "alice only" || status != "pending" {
		t.Fatalf("alice's target row: %q %q %v", content, status, err)
	}
}

func pngBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (c *client) upload(name string, data []byte) resp {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.e.srv.URL+"/api/v1/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.send(req)
}

func TestMediaUploadAndPublish(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("media@example.com")
	acc := c.connectMock()

	r := c.upload("photo.png", pngBytes(t))
	if r.status != 201 {
		t.Fatalf("upload: %d %s", r.status, r.body)
	}
	m := r.json(t)
	if m["mime_type"] != "image/png" || m["width"].(float64) != 4 || m["height"].(float64) != 3 || len(m["sha256"].(string)) != 64 {
		t.Fatalf("media meta: %v", m)
	}
	// MIME is sniffed, not trusted from the filename.
	if r := c.upload("evil.png", []byte("#!/bin/sh\necho pwned\n")); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
		t.Fatalf("script accepted: %d %s", r.status, r.body)
	}
	if r := c.upload("fake.jpg", append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 100)...)); r.status != 400 {
		t.Fatalf("corrupt jpeg accepted: %d %s", r.status, r.body)
	}
	mid := m["id"].(string)
	got := c.must("GET", "/api/v1/media/"+mid, nil, 200)
	if !strings.HasPrefix(got["url"].(string), "memory://") {
		t.Fatalf("no url: %v", got)
	}
	p := c.must("POST", "/api/v1/posts", map[string]any{"content": "with image", "social_account_ids": []string{acc}, "media_ids": []string{mid}}, 201)
	if r := c.do("DELETE", "/api/v1/media/"+mid, nil); r.status != 409 {
		t.Fatalf("referenced media deleted: %d", r.status)
	}
	c.must("POST", "/api/v1/posts/"+p["id"].(string)+"/publish", nil, 202)
	c.waitStatus(p["id"].(string), "published", 15*time.Second)
}
