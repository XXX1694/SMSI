package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	tokenPath   = "/api/v1/social/accounts/token"
	goodKey     = "mt_e2e_secret_0123456789"   // gitleaks:allow (fake test value)
	revokedKey  = "mt_revoked_e2e_secret_0123" // gitleaks:allow (fake test value)
	mockTokenID = "mocktoken"
)

func tokenBody(key string) map[string]any {
	return map[string]any{"provider": mockTokenID, "fields": map[string]string{"api_key": key, "handle": "e2e"}}
}

// connectToken connects the mock token provider and returns the account id.
func (c *client) connectToken(key string) string {
	c.e.t.Helper()
	m := c.must("POST", tokenPath, tokenBody(key), 201)
	id, _ := m["id"].(string)
	if id == "" || m["provider"] != mockTokenID || m["status"] != "active" {
		c.e.t.Fatalf("unexpected account: %v", m)
	}
	return id
}

// TestTokenConnectAndPublish: providers declare the form → connect → publish → published.
func TestTokenConnectAndPublish(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("token@example.com")

	var caps map[string]any
	for _, p := range c.must("GET", "/api/v1/social/providers", nil, 200)["items"].([]any) {
		if m := p.(map[string]any); m["name"] == mockTokenID {
			caps = m["capabilities"].(map[string]any)
		}
	}
	fields, _ := caps["connect_fields"].([]any)
	if caps["connect_method"] != "token" || len(fields) != 2 || caps["requires_title"] != true || caps["max_image_bytes"] == nil {
		t.Fatalf("capabilities: %v", caps)
	}

	r := c.do("POST", tokenPath, tokenBody(goodKey))
	if r.status != http.StatusCreated {
		t.Fatalf("connect: %d %s", r.status, r.body)
	}
	if strings.Contains(string(r.body), goodKey) {
		t.Fatalf("the connect response contains the secret: %s", r.body)
	}
	acc := r.json(t)
	id := acc["id"].(string)
	if acc["username"] != "e2e" || acc["status"] != "active" || acc["scopes"] == nil {
		t.Fatalf("account: %v", acc)
	}

	// The network needs a title, and the account's own limit (1000 characters) applies.
	if r := c.do("POST", "/api/v1/posts", map[string]any{"content": "no title", "social_account_ids": []string{id}}); r.status != 400 {
		t.Fatalf("missing title: %d %s", r.status, r.body)
	}
	long := map[string]any{"title": "T", "content": strings.Repeat("x", 1001), "social_account_ids": []string{id}}
	if r := c.do("POST", "/api/v1/posts", long); r.status != 400 {
		t.Fatalf("over the account limit: %d %s", r.status, r.body)
	}

	p := c.must("POST", "/api/v1/posts", map[string]any{"title": "Hello", "content": "token flow", "social_account_ids": []string{id}}, 201)
	postID := p["id"].(string)
	c.must("POST", "/api/v1/posts/"+postID+"/publish", map[string]any{"confirm": true}, 202)
	st := c.waitStatus(postID, "published", 15*time.Second)
	target := st["targets"].([]any)[0].(map[string]any)
	if target["status"] != "published" || target["external_url"] == "" {
		t.Fatalf("target: %v", target)
	}

	// Nothing the API says afterwards contains the credential.
	for _, path := range []string{"/api/v1/social/accounts", "/api/v1/social/accounts/" + id, "/api/v1/audit-logs?limit=100", "/api/v1/posts/" + postID} {
		if r := c.do("GET", path, nil); strings.Contains(string(r.body), goodKey) {
			t.Errorf("GET %s leaks the credential", path)
		}
	}
	if actions := auditActions(t, c); actions["social_account.connected"] != 1 {
		t.Fatalf("audit: %v", actions)
	}
}

func TestTokenConnectRejectsBadInput(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	c.register("badinput@example.com")
	for name, body := range map[string]any{
		"rejected by the network": tokenBody("wrong-key"),
		"unknown field":           map[string]any{"provider": mockTokenID, "fields": map[string]string{"api_key": goodKey, "nope": "x"}},
		"missing required":        map[string]any{"provider": mockTokenID, "fields": map[string]string{"handle": "x"}},
		"oversize":                map[string]any{"provider": mockTokenID, "fields": map[string]string{"api_key": "mt_" + strings.Repeat("a", 3000)}},
		"non-string value":        map[string]any{"provider": mockTokenID, "fields": map[string]any{"api_key": 12345}},
	} {
		r := c.do("POST", tokenPath, body)
		if r.status != http.StatusBadRequest || (name != "non-string value" && r.errCode(t) != "VALIDATION_ERROR") {
			t.Errorf("%s: want 400, got %d %s", name, r.status, r.body)
		}
		if strings.Contains(string(r.body), "wrong-key") {
			t.Errorf("%s: the error echoes the credential", name)
		}
	}
	for _, name := range []string{"linkedin", "telegram", "mock", "x", "nope"} {
		r := c.do("POST", tokenPath, map[string]any{"provider": name, "fields": map[string]string{"api_key": goodKey}})
		if r.status != http.StatusNotImplemented || r.errCode(t) != "PROVIDER_NOT_AVAILABLE" {
			t.Errorf("%s: want 501 PROVIDER_NOT_AVAILABLE, got %d %s", name, r.status, r.body)
		}
	}
	if n := len(c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)); n != 0 {
		t.Fatalf("a failed connect created %d accounts", n)
	}
}

// social:connect is critical: never in the default scope set, required for keys.
func TestTokenConnectNeedsTheCriticalScopeForKeys(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	c.register("scope@example.com")

	def := e.apiKeyClient(c.createKey("defaults")) // no scopes requested: the default set
	noScope := e.apiKeyClient(c.createKey("publisher", "social:read", "posts:publish", "social:disconnect"))
	for name, k := range map[string]*client{"default key": def, "other critical scopes": noScope} {
		if r := k.do("POST", tokenPath, tokenBody(goodKey)); r.status != http.StatusForbidden || r.errCode(t) != "INSUFFICIENT_SCOPE" {
			t.Errorf("%s: want 403 INSUFFICIENT_SCOPE, got %d %s", name, r.status, r.body)
		}
	}
	if n := len(c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)); n != 0 {
		t.Fatalf("a forbidden connect created %d accounts", n)
	}
	k := e.apiKeyClient(c.createTrustedKey("connector", "social:read", "social:connect")) // approvals have their own tests
	acc := k.must("POST", tokenPath, tokenBody(goodKey), 201)
	if strings.Contains(string(mustMarshal(acc)), goodKey) {
		t.Fatal("response leaks the credential")
	}
	// The agent's connect is attributed in the audit log.
	var found bool
	for _, l := range c.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)["items"].([]any) {
		m := l.(map[string]any)
		found = found || (m["action"] == "social_account.connected" && m["actor_type"] == "api_key")
	}
	if !found {
		t.Fatal("the key's connect is not audited as an api_key action")
	}
}

// A token revoked at the network later: publish fails with SOCIAL_ACCOUNT_EXPIRED and reconnecting repairs it.
func TestTokenAccountExpiresAndReconnects(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("revoked@example.com")
	id := c.connectToken(revokedKey)
	p := c.must("POST", "/api/v1/posts", map[string]any{"title": "T", "content": "x", "social_account_ids": []string{id}}, 201)
	postID := p["id"].(string)
	c.must("POST", "/api/v1/posts/"+postID+"/publish", map[string]any{"confirm": true}, 202)
	st := c.waitStatus(postID, "failed", 15*time.Second)
	if code := st["targets"].([]any)[0].(map[string]any)["error_code"]; code != "SOCIAL_ACCOUNT_EXPIRED" {
		t.Fatalf("target: %v", st)
	}
	if got := c.must("GET", "/api/v1/social/accounts/"+id, nil, 200); got["status"] != "expired" {
		t.Fatalf("account: %v", got)
	}
	// Reconnecting repeats the same POST: one account, active again (the mock accepts the key at connect time).
	if again := c.connectToken(revokedKey); again != id {
		t.Fatalf("reconnect created a second account: %s vs %s", again, id)
	}
	if got := c.must("GET", "/api/v1/social/accounts/"+id, nil, 200); got["status"] != "active" {
		t.Fatalf("account after reconnect: %v", got)
	}
}
