package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAgentAcceptanceFlow is the acceptance scenario driven the way an MCP
// agent drives it: a human registers and connects an account in the browser,
// then an API key (no publish scope) creates a draft and schedules it. The
// worker (real Asynq against Redis) publishes it, the status turns published
// and the audit log attributes every step.
func TestAgentAcceptanceFlow(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	human := e.browser()
	human.register("agent-owner@example.com")
	acc := human.connectMock()

	raw := human.createKey("mcp agent", "social:read", "posts:read", "posts:write", "posts:schedule")
	agent := e.apiKeyClient(raw)

	// The MCP server derives its tool list from /me.
	me := agent.must("GET", "/api/v1/me", nil, 200)
	scopes := me["scopes"].([]any)
	if me["auth_type"] != "api_key" || len(scopes) != 4 || me["csrf_token"] != nil {
		t.Fatalf("unexpected /me for key: %v", me)
	}
	if k, _ := me["api_key"].(map[string]any); k == nil || k["name"] != "mcp agent" || k["id"] == "" {
		t.Fatalf("api_key block missing: %v", me)
	}
	if me["user"].(map[string]any)["email"] != "agent-owner@example.com" {
		t.Fatalf("user block missing: %v", me)
	}

	accounts := agent.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)
	if len(accounts) != 1 || accounts[0].(map[string]any)["id"] != acc {
		t.Fatalf("agent cannot see the connected account: %v", accounts)
	}

	draft := agent.must("POST", "/api/v1/posts", map[string]any{
		"title": "Agent draft", "content": "Written by an agent", "social_account_ids": []string{acc},
	}, 201)
	postID := draft["id"].(string)
	if draft["status"] != "draft" || draft["created_by"] != "api_key" {
		t.Fatalf("draft not created by api key: %v", draft)
	}

	// Without posts:publish the agent can neither publish nor retry.
	for _, path := range []string{"/publish", "/retry"} {
		r := agent.do("POST", "/api/v1/posts/"+postID+path, map[string]any{"confirm": true})
		if r.status != http.StatusForbidden || r.errCode(t) != "INSUFFICIENT_SCOPE" {
			t.Fatalf("%s without scope: want 403 INSUFFICIENT_SCOPE, got %d %s", path, r.status, r.body)
		}
		if body := r.errBody(t); body["request_id"] == "" || !strings.Contains(body["message"].(string), "posts:publish") {
			t.Fatalf("scope error should name the missing scope and carry a request id: %v", body)
		}
	}
	if st := agent.must("GET", "/api/v1/posts/"+postID+"/status", nil, 200); st["status"] != "draft" {
		t.Fatalf("forbidden publish changed the post: %v", st)
	}

	at := time.Now().Add(2 * time.Second)
	sched := agent.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(at.Add(time.Second))}, 200)
	if sched["status"] != "scheduled" {
		t.Fatalf("not scheduled: %v", sched)
	}
	st := agent.waitStatus(postID, "published", 20*time.Second)
	target := st["targets"].([]any)[0].(map[string]any)
	if target["status"] != "published" || target["external_post_id"] == "" || target["published_at"] == nil {
		t.Fatalf("target not published: %v", target)
	}
	if st["published_at"] == nil {
		t.Fatalf("post.published_at not set: %v", st)
	}

	// Audit trail: user actions by the user, key requests and creation by the key, publishing by the scheduler.
	logs := human.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)["items"].([]any)
	type actorAction struct{ actor, action string }
	seen := map[actorAction]int{}
	for _, l := range logs {
		m := l.(map[string]any)
		seen[actorAction{m["actor_type"].(string), m["action"].(string)}]++
	}
	for _, want := range []actorAction{
		{"user", "user.registered"}, {"user", "social_account.connected"}, {"user", "api_key.created"},
		{"api_key", "post.created"}, {"api_key", "post.scheduled"}, {"api_key", "api_key.request"},
		{"scheduler", "post_target.published"}, {"scheduler", "post.completed"},
	} {
		if seen[want] == 0 {
			t.Errorf("missing audit entry %v (have %v)", want, seen)
		}
	}
	for _, l := range logs {
		m := l.(map[string]any)
		if m["actor_type"] == "api_key" && m["action"] == "post.created" && m["actor_label"] != "mcp agent" {
			t.Errorf("api key actions must carry the key name: %v", m)
		}
	}
}

// TestPublishScopeEnforcedAcrossDangerousEndpoints makes sure the sensitive
// scopes are required individually (a key with all others still gets 403).
func TestPublishScopeEnforcedAcrossDangerousEndpoints(t *testing.T) {
	e := newEnv(t, envOpts{})
	human := e.browser()
	human.register("scopes@example.com")
	acc := human.connectMock()
	post := human.must("POST", "/api/v1/posts", map[string]any{"content": "x", "social_account_ids": []string{acc}}, 201)
	id := post["id"].(string)

	k := e.apiKeyClient(human.createKey("almost root", "social:read", "posts:read", "posts:write", "posts:schedule", "media:write", "analytics:read"))
	for _, tc := range []struct{ method, path, scope string }{
		{"POST", "/api/v1/posts/" + id + "/publish", "posts:publish"},
		{"DELETE", "/api/v1/posts/" + id, "posts:delete"},
		{"DELETE", "/api/v1/social/accounts/" + acc, "social:disconnect"},
	} {
		r := k.do(tc.method, tc.path, nil)
		if r.status != 403 || r.errCode(t) != "INSUFFICIENT_SCOPE" || !strings.Contains(r.errBody(t)["message"].(string), tc.scope) {
			t.Errorf("%s %s: want 403 INSUFFICIENT_SCOPE naming %s, got %d %s", tc.method, tc.path, tc.scope, r.status, r.body)
		}
	}
	// The post and the account survived.
	human.must("GET", "/api/v1/posts/"+id, nil, 200)
	if a := human.must("GET", "/api/v1/social/accounts/"+acc, nil, 200); a["status"] != "active" {
		t.Fatalf("account changed: %v", a)
	}
}
