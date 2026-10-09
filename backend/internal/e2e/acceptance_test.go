package e2e

import (
	"net/http"
	"testing"
	"time"
)

// TestAcceptanceFlow: register → connect mock account → draft → schedule →
// worker publishes → status published → audit entries.
func TestAcceptanceFlow(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("alice@example.com")

	me := c.must("GET", "/api/v1/me", nil, 200)
	if me["auth_type"] != "session" || len(me["scopes"].([]any)) != 10 || me["csrf_token"] != c.csrf {
		t.Fatalf("unexpected /me: %v", me)
	}
	providers := c.must("GET", "/api/v1/social/providers", nil, 200)["items"].([]any)
	if len(providers) != 17 {
		t.Fatalf("expected 17 providers (linkedin, telegram, discord, mastodon, bluesky, mock, mocktoken + 10 stubs), got %d", len(providers))
	}

	accountID := c.connectMock()

	draft := c.must("POST", "/api/v1/posts", map[string]any{
		"title": "Launch", "content": "Hello from SocialOS", "social_account_ids": []string{accountID},
	}, 201)
	postID := draft["id"].(string)
	if draft["status"] != "draft" || len(draft["targets"].([]any)) != 1 {
		t.Fatalf("bad draft: %v", draft)
	}

	at := time.Now().Add(2 * time.Second)
	sched := c.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(at.Add(time.Second))}, 200)
	if sched["status"] != "scheduled" {
		t.Fatalf("not scheduled: %v", sched)
	}

	st := c.waitStatus(postID, "published", 20*time.Second)
	target := st["targets"].([]any)[0].(map[string]any)
	if target["status"] != "published" || target["external_post_id"] == "" || target["external_url"] == "" {
		t.Fatalf("target not published: %v", target)
	}

	detail := c.must("GET", "/api/v1/posts/"+postID, nil, 200)
	attempts := detail["attempts"].([]any)
	if len(attempts) != 1 || attempts[0].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("expected one succeeded attempt: %v", attempts)
	}

	actions := auditActions(t, c)
	for _, want := range []string{"user.registered", "social_account.connected", "post.created", "post.scheduled",
		"post_target.published", "post.completed"} {
		if actions[want] == 0 {
			t.Errorf("missing audit action %s (have %v)", want, actions)
		}
	}

	dash := c.must("GET", "/api/v1/dashboard/summary", nil, 200)
	if dash["connected_accounts"].(float64) != 1 || dash["published_this_month"].(float64) != 1 {
		t.Fatalf("dashboard: %v", dash)
	}
	an := c.must("GET", "/api/v1/analytics", nil, 200)
	if an["totals"].(map[string]any)["posts_published"].(float64) != 1 {
		t.Fatalf("analytics: %v", an)
	}

	// Published posts are terminal: cannot be scheduled again.
	r := c.do("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))})
	if r.status != http.StatusConflict || r.errCode(t) != "INVALID_STATE_TRANSITION" {
		t.Fatalf("expected 409 INVALID_STATE_TRANSITION, got %d %s", r.status, r.body)
	}
}

func TestPublishNowAndCancel(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("bob@example.com")
	acc := c.connectMock()

	p := c.must("POST", "/api/v1/posts", map[string]any{"content": "now", "social_account_ids": []string{acc}}, 201)
	id := p["id"].(string)
	c.must("POST", "/api/v1/posts/"+id+"/publish", map[string]any{"confirm": true}, 202)
	c.waitStatus(id, "published", 15*time.Second)

	// Scheduled then cancelled: worker must never publish it.
	p2 := c.must("POST", "/api/v1/posts", map[string]any{"content": "later", "social_account_ids": []string{acc},
		"scheduled_at": fmtTime(time.Now().Add(2 * time.Second)), "schedule": true}, 201)
	if p2["status"] != "scheduled" {
		t.Fatalf("expected scheduled: %v", p2)
	}
	id2 := p2["id"].(string)
	c.must("POST", "/api/v1/posts/"+id2+"/cancel", nil, 200)
	time.Sleep(3 * time.Second)
	st := c.must("GET", "/api/v1/posts/"+id2+"/status", nil, 200)
	if st["status"] != "cancelled" || st["targets"].([]any)[0].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("cancelled post changed: %v", st)
	}

	// Permanent provider failure → failed, then retry is allowed.
	p3 := c.must("POST", "/api/v1/posts", map[string]any{"content": "boom #mock-fail", "social_account_ids": []string{acc}}, 201)
	id3 := p3["id"].(string)
	c.must("POST", "/api/v1/posts/"+id3+"/publish", nil, 202)
	st = c.waitStatus(id3, "failed", 15*time.Second)
	if st["targets"].([]any)[0].(map[string]any)["error_code"] != "MOCK_REJECTED" {
		t.Fatalf("expected MOCK_REJECTED: %v", st)
	}
	c.must("POST", "/api/v1/posts/"+id3+"/retry", nil, 202)
	c.waitStatus(id3, "failed", 15*time.Second)
}
