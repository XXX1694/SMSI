package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const approvalHeader = "X-Approval-Id"

// needApproval asserts a 428 APPROVAL_REQUIRED and returns the approval id.
func needApproval(t *testing.T, r resp, action string) string {
	t.Helper()
	if r.status != http.StatusPreconditionRequired || r.errCode(t) != "APPROVAL_REQUIRED" {
		t.Fatalf("want 428 APPROVAL_REQUIRED, got %d %s", r.status, r.body)
	}
	f, _ := r.errBody(t)["fields"].(map[string]any)
	id, _ := f["approval_id"].(string)
	if id == "" || f["action"] != action || f["expires_at"] == "" || !strings.HasSuffix(f["approve_url"].(string), "/approvals") {
		t.Fatalf("approval details missing or wrong (want action %s): %v", action, f)
	}
	return id
}

func withApproval(id string) map[string]string { return map[string]string{approvalHeader: id} }

// agentRig is an owner with a mock account, a draft and a key with every dangerous scope.
type agentRig struct {
	e      *env
	owner  *client
	agent  *client
	acc    string
	postID string
}

func newAgentRig(t *testing.T, email string) *agentRig {
	t.Helper()
	e := newEnv(t, envOpts{startWorker: true})
	owner := e.browser()
	owner.register(email)
	acc := owner.connectMock()
	scopes := []string{"social:read", "posts:read", "posts:write", "posts:schedule", "posts:publish", "posts:delete", "social:disconnect", "social:connect"}
	r := &agentRig{e: e, owner: owner, agent: e.apiKeyClient(owner.createKey("agent", scopes...)), acc: acc}
	r.postID = r.draft("first draft")
	return r
}

func (r *agentRig) draft(content string) string {
	return r.owner.must("POST", "/api/v1/posts", map[string]any{"content": content, "social_account_ids": []string{r.acc}}, 201)["id"].(string)
}

func (r *agentRig) approve(t *testing.T, id string) {
	t.Helper()
	r.owner.must("POST", "/api/v1/approvals/"+id+"/approve", nil, 200)
}

func TestKeyPublishNeedsApprovalThenWorksOnce(t *testing.T) {
	r := newAgentRig(t, "approve-publish@example.com")
	pub := "/api/v1/posts/" + r.postID + "/publish"

	// The agent-supplied confirm flag bypasses nothing.
	id := needApproval(t, r.agent.do("POST", pub, map[string]any{"confirm": true}), "post.publish")
	if st := r.owner.must("GET", "/api/v1/posts/"+r.postID+"/status", nil, 200); st["status"] != "draft" {
		t.Fatalf("the post moved without approval: %v", st)
	}
	pending := r.owner.must("GET", "/api/v1/approvals", nil, 200)["items"].([]any)
	if len(pending) != 1 {
		t.Fatalf("owner should see one pending approval: %v", pending)
	}
	item := pending[0].(map[string]any)
	if item["id"] != id || item["status"] != "pending" || item["actor_label"] != "agent" || item["resource_id"] != r.postID ||
		item["summary"].(map[string]any)["content"] != "first draft" || item["fingerprint"] != nil {
		t.Fatalf("approval as the owner sees it: %v", item)
	}
	// Asking again returns the same pending approval instead of piling up new ones.
	if again := needApproval(t, r.agent.do("POST", pub, nil), "post.publish"); again != id {
		t.Fatalf("repeat request created a second approval %s != %s", again, id)
	}
	// Pending is not approved.
	needApproval(t, r.agent.doWith("POST", pub, nil, withApproval(id)), "post.publish")

	r.approve(t, id)
	r.agent.doWith("POST", pub, nil, withApproval(id))
	if st := r.owner.waitStatus(r.postID, "published", 20*time.Second); st["status"] != "published" {
		t.Fatalf("approved publish did not run: %v", st)
	}
	// Single use: the same id on another publish of the same call is refused with a fresh request.
	again := needApproval(t, r.agent.doWith("POST", pub, nil, withApproval(id)), "post.publish")
	if again == id {
		t.Fatal("a consumed approval was offered again")
	}
}

func TestApprovedPublishIsBoundToTheExactPostVersion(t *testing.T) {
	r := newAgentRig(t, "bound-publish@example.com")
	pub := "/api/v1/posts/" + r.postID + "/publish"
	id := needApproval(t, r.agent.do("POST", pub, nil), "post.publish")
	r.approve(t, id)

	r.owner.must("PATCH", "/api/v1/posts/"+r.postID, map[string]any{"content": "edited after approval"}, 200)
	needApproval(t, r.agent.doWith("POST", pub, nil, withApproval(id)), "post.publish")

	other := r.draft("another post")
	needApproval(t, r.agent.doWith("POST", "/api/v1/posts/"+other+"/publish", nil, withApproval(id)), "post.publish")

	sibling := r.e.apiKeyClient(r.owner.createKey("sibling", "posts:publish", "posts:read"))
	fresh := needApproval(t, r.agent.do("POST", "/api/v1/posts/"+other+"/publish", nil), "post.publish")
	r.approve(t, fresh)
	needApproval(t, sibling.doWith("POST", "/api/v1/posts/"+other+"/publish", nil, withApproval(fresh)), "post.publish")
	if st := r.owner.must("GET", "/api/v1/posts/"+other+"/status", nil, 200); st["status"] != "draft" {
		t.Fatalf("a mismatched approval published the post: %v", st)
	}
}

func TestDeniedApprovalStopsTheAgent(t *testing.T) {
	r := newAgentRig(t, "deny@example.com")
	pub := "/api/v1/posts/" + r.postID + "/publish"
	id := needApproval(t, r.agent.do("POST", pub, nil), "post.publish")
	r.owner.must("POST", "/api/v1/approvals/"+id+"/deny", nil, 200)
	if res := r.agent.doWith("POST", pub, nil, withApproval(id)); res.status != http.StatusForbidden || res.errCode(t) != "FORBIDDEN" {
		t.Fatalf("denied approval: %d %s", res.status, res.body)
	}
	if res := r.owner.do("POST", "/api/v1/approvals/"+id+"/approve", nil); res.status != http.StatusConflict {
		t.Fatalf("a decided approval can be decided again: %d %s", res.status, res.body)
	}
}

func TestDeleteAndDisconnectNeedApproval(t *testing.T) {
	r := newAgentRig(t, "delete-disconnect@example.com")
	del := "/api/v1/posts/" + r.postID
	id := needApproval(t, r.agent.do("DELETE", del, nil), "post.delete")
	r.owner.must("GET", del, nil, 200) // still there
	r.approve(t, id)
	if res := r.agent.doWith("DELETE", del, nil, withApproval(id)); res.status != http.StatusNoContent {
		t.Fatalf("approved delete: %d %s", res.status, res.body)
	}
	r.owner.do("GET", del, nil)

	acc := "/api/v1/social/accounts/" + r.acc
	id = needApproval(t, r.agent.do("DELETE", acc, nil), "social_account.disconnect")
	r.approve(t, id)
	if res := r.agent.doWith("DELETE", acc, nil, withApproval(id)); res.status != http.StatusNoContent {
		t.Fatalf("approved disconnect: %d %s", res.status, res.body)
	}
	if res := r.owner.do("GET", acc, nil); res.status == http.StatusOK && res.json(t)["status"] == "active" {
		t.Fatal("the account is still active")
	}
}

func TestRetryNowNeedsApproval(t *testing.T) {
	r := newAgentRig(t, "retry@example.com")
	failing := r.draft("this will fail #mock-fail")
	r.owner.must("POST", "/api/v1/posts/"+failing+"/publish", nil, 202)
	r.owner.waitStatus(failing, "failed", 20*time.Second)

	retry := "/api/v1/posts/" + failing + "/retry"
	id := needApproval(t, r.agent.do("POST", retry, nil), "post.retry_now")
	r.approve(t, id)
	if res := r.agent.doWith("POST", retry, nil, withApproval(id)); res.status != http.StatusAccepted {
		t.Fatalf("approved retry: %d %s", res.status, res.body)
	}
	// The same applies to retrying for a time that is too close.
	r.owner.waitStatus(failing, "failed", 20*time.Second)
	soon := map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Minute))}
	needApproval(t, r.agent.do("POST", retry, soon), "post.schedule_soon")
}

func TestSessionsAndTrustedKeysNeverGet428(t *testing.T) {
	r := newAgentRig(t, "never428@example.com")
	trusted := r.e.apiKeyClient(r.owner.createTrustedKey("trusted", "posts:publish", "posts:read"))
	trustedPost := r.draft("by a trusted key")
	trusted.must("POST", "/api/v1/posts/"+trustedPost+"/publish", nil, 202)
	r.owner.must("POST", "/api/v1/posts/"+r.postID+"/publish", nil, 202)
	if n := len(r.owner.must("GET", "/api/v1/approvals?status=all", nil, 200)["items"].([]any)); n != 0 {
		t.Fatalf("%d approvals were created for sessions or trusted keys", n)
	}
}

func TestOnlyTheOwnersBrowserSessionDecides(t *testing.T) {
	r := newAgentRig(t, "decide@example.com")
	id := needApproval(t, r.agent.do("POST", "/api/v1/posts/"+r.postID+"/publish", nil), "post.publish")
	for _, path := range []string{"/approve", "/deny", ""} {
		method := "POST"
		if path == "" {
			method = "GET"
		}
		if res := r.agent.do(method, "/api/v1/approvals/"+id+path, nil); res.status != http.StatusForbidden {
			t.Errorf("a key reached %s approvals/{id}%s: %d %s", method, path, res.status, res.body)
		}
	}
	if res := r.agent.do("GET", "/api/v1/approvals", nil); res.status != http.StatusForbidden {
		t.Errorf("a key listed approvals: %d", res.status)
	}
	noCSRF := &client{e: r.e, http: r.owner.http} // cookies, but no X-CSRF-Token
	if res := noCSRF.do("POST", "/api/v1/approvals/"+id+"/approve", nil); res.status != http.StatusForbidden {
		t.Fatalf("approved without CSRF: %d %s", res.status, res.body)
	}
	if st := r.owner.must("GET", "/api/v1/approvals/"+id, nil, 200); st["status"] != "pending" {
		t.Fatalf("approval changed state: %v", st)
	}
}

func TestKeyPolicyDefaultsToApprove(t *testing.T) {
	r := newAgentRig(t, "policy@example.com")
	keys := r.owner.must("GET", "/api/v1/developer/api-keys", nil, 200)["items"].([]any)
	if keys[0].(map[string]any)["dangerous_policy"] != "approve" {
		t.Fatalf("new keys must default to approve: %v", keys[0])
	}
	bad := r.owner.do("POST", "/api/v1/developer/api-keys", map[string]any{"name": "x", "dangerous_policy": "yolo"})
	if bad.status != http.StatusBadRequest {
		t.Fatalf("unknown policy: %d %s", bad.status, bad.body)
	}
	mcp := r.owner.must("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": "Claude", "scopes": []string{"posts:read", "posts:delete"}}, 201)
	if mcpKey := r.e.apiKeyClient(mcp["key"].(string)); needApproval(t, mcpKey.do("DELETE", "/api/v1/posts/"+r.postID, nil), "post.delete") == "" {
		t.Fatal("MCP connections must default to approve")
	}
}

// The owner sees everything the approval is bound to: the whole text, per-network text and media.
func TestApprovalSummaryShowsFullTextOverridesAndMedia(t *testing.T) {
	r := newAgentRig(t, "summary@example.com")
	long := strings.Repeat("long text ", 60) // 600 characters, over any preview length
	p := r.owner.must("POST", "/api/v1/posts", map[string]any{"content": long, "social_account_ids": []string{r.acc}}, 201)["id"].(string)
	r.owner.must("PATCH", "/api/v1/posts/"+p, map[string]any{"targets": []map[string]any{{"social_account_id": r.acc, "content": "mock network text"}}}, 200)
	id := needApproval(t, r.agent.do("POST", "/api/v1/posts/"+p+"/publish", nil), "post.publish")
	sum := r.owner.must("GET", "/api/v1/approvals/"+id, nil, 200)["summary"].(map[string]any)
	if sum["content"] != long {
		t.Fatalf("summary text is cut: %d of %d characters", len(sum["content"].(string)), len(long))
	}
	targets, _ := sum["targets"].([]any)
	if len(targets) != 1 || targets[0].(map[string]any)["content"] != "mock network text" || targets[0].(map[string]any)["platform"] != "mock" {
		t.Fatalf("per-network text missing: %v", sum["targets"])
	}
	if m, _ := sum["media"].(map[string]any); m == nil || m["count"] != float64(0) {
		t.Fatalf("media summary missing: %v", sum["media"])
	}
}

// Every gated route answers 428 APPROVAL_REQUIRED for a key with policy approve, never a 500 or a silent success.
func TestEveryGatedRouteAsksTheOwner(t *testing.T) {
	r := newAgentRig(t, "all-routes@example.com")
	post := func(content string) string { return r.draft(content) }
	soon := fmtTime(time.Now().Add(time.Minute))
	scheduledIn := func(d time.Duration) string {
		id := post("scheduled " + d.String())
		r.owner.must("POST", "/api/v1/posts/"+id+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(d))}, 200)
		return id
	}
	failing := post("will fail #mock-fail")
	r.owner.must("POST", "/api/v1/posts/"+failing+"/publish", nil, 202)
	r.owner.waitStatus(failing, "failed", 20*time.Second)
	farScheduled, nearScheduled := scheduledIn(time.Hour), scheduledIn(90*time.Second)

	for _, tc := range []struct {
		name, method, path string
		body               any
		action             string
	}{
		{"publish now", "POST", "/api/v1/posts/" + post("p") + "/publish", nil, "post.publish"},
		{"retry now", "POST", "/api/v1/posts/" + failing + "/retry", nil, "post.retry_now"},
		{"retry under the lead", "POST", "/api/v1/posts/" + failing + "/retry", map[string]any{"scheduled_at": soon}, "post.schedule_soon"},
		{"delete", "DELETE", "/api/v1/posts/" + post("d"), nil, "post.delete"},
		{"disconnect", "DELETE", "/api/v1/social/accounts/" + r.acc, nil, "social_account.disconnect"},
		{"token connect", "POST", tokenPath, tokenBody(goodKey), "social_account.connect_token"},
		{"create under the lead", "POST", "/api/v1/posts", map[string]any{"content": "c", "social_account_ids": []string{r.acc}, "schedule": true, "scheduled_at": soon}, "post.schedule_soon"},
		{"schedule under the lead", "POST", "/api/v1/posts/" + post("s") + "/schedule", map[string]any{"scheduled_at": soon}, "post.schedule_soon"},
		{"move a schedule under the lead", "PATCH", "/api/v1/posts/" + farScheduled, map[string]any{"scheduled_at": soon}, "post.schedule_soon"},
		{"edit a post running within the lead", "PATCH", "/api/v1/posts/" + nearScheduled, map[string]any{"content": "edited"}, "post.schedule_soon"},
	} {
		res := r.agent.do(tc.method, tc.path, tc.body)
		if res.status >= 500 {
			t.Errorf("%s: server error %d %s", tc.name, res.status, res.body)
			continue
		}
		needApproval(t, res, tc.action)
	}
}
