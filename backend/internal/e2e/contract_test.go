package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

var capabilityKeys = []string{"can_publish_text", "can_publish_image", "can_publish_video", "can_schedule", "can_delete",
	"can_analytics", "max_text_length", "max_media_count", "requires_approval", "notes"}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func hasKeys(t *testing.T, what string, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("%s: missing key %q (have %v)", what, k, keysOf(m))
		}
	}
}

// TestMeShape pins the identity payload the MCP server and the browser read.
func TestMeShape(t *testing.T) {
	e := newEnv(t, envOpts{})
	c := e.browser()
	reg := c.register("me-shape@example.com")
	for name, me := range map[string]map[string]any{"register": reg, "me": c.must("GET", "/api/v1/me", nil, 200),
		"login": e.browser().must("POST", "/api/v1/auth/login", map[string]any{"email": "me-shape@example.com", "password": "correct horse battery"}, 200)} {
		hasKeys(t, name, me, "user", "scopes", "csrf_token", "auth_type", "api_key", "id", "email", "display_name")
		user := me["user"].(map[string]any)
		hasKeys(t, name+".user", user, "id", "email", "display_name", "created_at")
		if me["auth_type"] != "session" || me["api_key"] != nil || me["csrf_token"] == "" || me["csrf_token"] == nil {
			t.Errorf("%s: session identity wrong: %v", name, me)
		}
		if me["id"] != user["id"] || me["email"] != user["email"] || me["display_name"] != "Tester" {
			t.Errorf("%s: flat user fields must mirror user: %v", name, me)
		}
		if len(me["scopes"].([]any)) != 10 {
			t.Errorf("%s: sessions hold every scope, got %v", name, me["scopes"])
		}
	}
	k := e.apiKeyClient(c.createKey("reader", "posts:read", "social:read"))
	me := k.must("GET", "/api/v1/me", nil, 200)
	if me["auth_type"] != "api_key" || me["csrf_token"] != nil {
		t.Fatalf("api key identity: %v", me)
	}
	if got := me["scopes"].([]any); len(got) != 2 || got[0] != "posts:read" || got[1] != "social:read" {
		t.Fatalf("api key scopes: %v", got)
	}
	ak := me["api_key"].(map[string]any)
	if ak["name"] != "reader" || len(ak["id"].(string)) != 36 {
		t.Fatalf("api_key block: %v", ak)
	}
}

func TestSessionCookies(t *testing.T) {
	e := newEnv(t, envOpts{})
	body := `{"email":"cookie@example.com","password":"correct horse battery","display_name":"C","accept_terms":true}`
	res, err := http.Post(e.srv.URL+"/api/v1/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var sess, csrf *http.Cookie
	for _, ck := range res.Cookies() {
		switch ck.Name {
		case "socialos_session":
			sess = ck
		case "socialos_csrf":
			csrf = ck
		}
	}
	if sess == nil || csrf == nil {
		t.Fatalf("cookies missing: %v", res.Cookies())
	}
	if !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode || sess.Path != "/" || sess.MaxAge <= 0 {
		t.Errorf("session cookie must be HttpOnly, SameSite=Lax, path=/ with expiry: %+v", sess)
	}
	if csrf.HttpOnly || csrf.SameSite != http.SameSiteLaxMode {
		t.Errorf("csrf cookie must be script-readable (double submit) and SameSite=Lax: %+v", csrf)
	}
	if len(sess.Value) < 32 || sess.Value == csrf.Value {
		t.Errorf("tokens must be long and distinct")
	}
	var tokens map[string]any
	c := e.browser()
	c.register("cookie2@example.com")
	tokens = c.must("GET", "/api/v1/me", nil, 200)
	if tokens["csrf_token"] == "" {
		t.Fatal("csrf token not returned")
	}
}

// TestEndpointContract walks every REST endpoint of ARCHITECTURE.md §4.
func TestEndpointContract(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("contract@example.com")

	t.Run("ops", func(t *testing.T) {
		for _, p := range []string{"/health", "/api/v1/health"} {
			if r := c.do("GET", p, nil); r.status != 200 || r.json(t)["status"] != "ok" {
				t.Errorf("%s: %d %s", p, r.status, r.body)
			}
		}
		for _, p := range []string{"/ready", "/api/v1/ready"} {
			r := c.do("GET", p, nil)
			checks, _ := r.json(t)["checks"].(map[string]any)
			if r.status != 200 || checks["postgres"] != "ok" || checks["redis"] != "ok" || checks["storage"] != "ok" {
				t.Errorf("%s: %d %s", p, r.status, r.body)
			}
		}
		if r := c.do("GET", "/api/v1/version", nil); r.status != 200 || r.json(t)["version"] == nil || r.json(t)["commit"] == nil {
			t.Errorf("/api/v1/version: %d %s", r.status, r.body)
		}
		r := c.do("GET", "/metrics", nil)
		if r.status != 200 || !strings.Contains(string(r.body), "socialos_http_requests_total") || !strings.Contains(string(r.body), `status="200"`) {
			t.Errorf("metrics: %d %.200s", r.status, r.body)
		}
	})

	t.Run("providers", func(t *testing.T) {
		items := c.must("GET", "/api/v1/social/providers", nil, 200)["items"].([]any)
		byID := map[string]map[string]any{}
		for _, it := range items {
			p := it.(map[string]any)
			hasKeys(t, "provider", p, "id", "name", "display_name", "status", "supported", "configured", "capabilities")
			caps := p["capabilities"].(map[string]any)
			hasKeys(t, "capabilities of "+p["id"].(string), caps, capabilityKeys...)
			for k := range caps {
				if strings.ToLower(k) != k {
					t.Errorf("capability key %q is not snake_case", k)
				}
			}
			byID[p["id"].(string)] = p
		}
		for _, id := range []string{"linkedin", "telegram", "mock", "instagram", "facebook", "tiktok", "youtube", "x", "threads", "pinterest"} {
			if byID[id] == nil {
				t.Errorf("provider %s not listed", id)
			}
		}
		li := byID["linkedin"]["capabilities"].(map[string]any)
		if li["can_publish_text"] != true || li["can_publish_image"] != true || li["can_publish_video"] != false ||
			li["requires_approval"] != true || li["can_analytics"] != false || li["max_text_length"].(float64) != 3000 {
			t.Errorf("linkedin capabilities: %v", li)
		}
		if byID["linkedin"]["configured"] != false {
			t.Errorf("linkedin has no credentials in this env, configured must be false")
		}
		tg := byID["telegram"]["capabilities"].(map[string]any)
		if tg["can_publish_video"] != true || tg["can_delete"] != true || tg["max_text_length"].(float64) != 4096 {
			t.Errorf("telegram capabilities: %v", tg)
		}
		for _, id := range []string{"instagram", "facebook", "tiktok", "youtube", "x", "threads", "pinterest"} {
			p := byID[id]
			caps := p["capabilities"].(map[string]any)
			if p["status"] != "unsupported" || p["supported"] != false || p["configured"] != false || caps["requires_approval"] != true ||
				caps["can_publish_text"] != false || !strings.HasPrefix(caps["notes"].(string), "Not available yet: ") {
				t.Errorf("%s must be a clearly-labelled unsupported stub: %v", id, p)
			}
		}
		if byID["mock"]["status"] != "supported" || !strings.Contains(byID["mock"]["capabilities"].(map[string]any)["notes"].(string), "Test network") {
			t.Errorf("mock must be labelled: %v", byID["mock"])
		}
	})

	acc := c.connectMock()
	t.Run("accounts", func(t *testing.T) {
		a := c.must("GET", "/api/v1/social/accounts/"+acc, nil, 200)
		hasKeys(t, "account", a, "id", "provider", "username", "display_name", "avatar_url", "scopes", "status", "connected_at")
		if a["provider"] != "mock" || a["status"] != "active" {
			t.Errorf("account: %v", a)
		}
		for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
			if r := c.do("GET", "/api/v1/social/accounts/"+id, nil); r.status != 404 || r.errCode(t) != "NOT_FOUND" {
				t.Errorf("account %s: %d %s", id, r.status, r.body)
			}
		}
		if r := c.do("POST", "/api/v1/social/telegram/connect", nil); r.status != 501 || r.errCode(t) != "PROVIDER_NOT_AVAILABLE" {
			t.Errorf("unconfigured telegram: %d %s", r.status, r.body)
		}
		// The old direct connect no longer exists: a chat in the body changes nothing.
		if r := c.do("POST", "/api/v1/social/telegram/connect", map[string]any{"chat": "@x"}); r.status != 501 {
			t.Errorf("telegram connect with a chat: %d %s", r.status, r.body)
		}
	})

	var postID string
	t.Run("posts: create validation", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"no content":        {"social_account_ids": []string{acc}},
			"blank content":     {"content": "   ", "social_account_ids": []string{acc}},
			"too long":          {"content": strings.Repeat("x", 5001), "social_account_ids": []string{acc}},
			"bad account id":    {"content": "x", "social_account_ids": []string{"nope"}},
			"unknown account":   {"content": "x", "social_account_ids": []string{"00000000-0000-0000-0000-000000000001"}},
			"unknown media":     {"content": "x", "social_account_ids": []string{acc}, "media_ids": []string{"00000000-0000-0000-0000-000000000001"}},
			"title too long":    {"content": "x", "title": strings.Repeat("t", 201)},
			"schedule in past":  {"content": "x", "social_account_ids": []string{acc}, "schedule": true, "scheduled_at": "2001-01-01T00:00:00Z"},
			"schedule no accts": {"content": "x", "schedule": true, "scheduled_at": fmtTime(time.Now().Add(time.Hour))},
		} {
			if r := c.do("POST", "/api/v1/posts", body); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
				t.Errorf("%s: want 400 VALIDATION_ERROR, got %d %s", name, r.status, r.body)
			}
		}
		// An empty body is a legal blank draft (editor autosave) but can neither be scheduled nor published.
		blank := c.must("POST", "/api/v1/posts", nil, 201)
		if blank["status"] != "draft" || len(blank["targets"].([]any)) != 0 {
			t.Errorf("blank draft: %v", blank)
		}
		for _, path := range []string{"/publish", "/schedule"} {
			if r := c.do("POST", "/api/v1/posts/"+blank["id"].(string)+path, map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))}); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
				t.Errorf("blank draft %s: want 400 VALIDATION_ERROR, got %d %s", path, r.status, r.body)
			}
		}
		c.must("DELETE", "/api/v1/posts/"+blank["id"].(string), nil, 204)
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/posts", strings.NewReader("{not json"))
		req.Header.Set("Content-Type", "application/json")
		if r := c.send(req); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
			t.Errorf("malformed json: %d %s", r.status, r.body)
		}
		req, _ = http.NewRequest("POST", e.srv.URL+"/api/v1/posts", strings.NewReader("content=x"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if r := c.send(req); r.status != 400 {
			t.Errorf("wrong content type: %d %s", r.status, r.body)
		}
	})

	t.Run("posts: lifecycle", func(t *testing.T) {
		p := c.must("POST", "/api/v1/posts", map[string]any{"title": "T", "content": "base", "social_account_ids": []string{acc},
			"targets": []map[string]any{{"social_account_id": acc, "content": "override for mock"}}}, 201)
		hasKeys(t, "post", p, "id", "title", "content", "status", "scheduled_at", "published_at", "created_by", "created_at", "updated_at", "targets", "media_ids")
		postID = p["id"].(string)
		tg := p["targets"].([]any)[0].(map[string]any)
		hasKeys(t, "target", tg, "id", "social_account_id", "platform", "content", "status", "attempt_count",
			"external_post_id", "external_url", "published_at", "error_code", "error_message")
		for _, k := range []string{"external_post_id", "external_url", "published_at", "error_code", "error_message"} {
			if tg[k] != nil {
				t.Errorf("a pending target has %s=%v, want null", k, tg[k])
			}
		}
		if p["status"] != "draft" || p["created_by"] != "user" || tg["content"] != "override for mock" || tg["platform"] != "mock" || tg["status"] != "pending" {
			t.Fatalf("draft: %v", p)
		}

		detail := c.must("GET", "/api/v1/posts/"+postID, nil, 200)
		hasKeys(t, "post detail", detail, "targets", "media", "media_ids", "attempts")
		if len(detail["media"].([]any)) != 0 || len(detail["attempts"].([]any)) != 0 {
			t.Errorf("fresh draft has media/attempts: %v", detail)
		}

		upd := c.must("PATCH", "/api/v1/posts/"+postID, map[string]any{"title": "T2", "content": "changed"}, 200)
		if upd["title"] != "T2" || upd["content"] != "changed" || upd["targets"].([]any)[0].(map[string]any)["content"] != "override for mock" {
			t.Errorf("patch must keep explicit per-target overrides: %v", upd)
		}

		at := time.Now().Add(time.Hour)
		sched := c.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": at.In(time.FixedZone("x", 5*3600)).Format(time.RFC3339)}, 200)
		if sched["status"] != "scheduled" || sched["scheduled_at"] == nil || !strings.HasSuffix(sched["scheduled_at"].(string), "Z") {
			t.Errorf("schedule: %v", sched)
		}
		if r := c.do("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{}); r.status != 400 {
			t.Errorf("schedule without time: %d", r.status)
		}
		st := c.must("GET", "/api/v1/posts/"+postID+"/status", nil, 200)
		hasKeys(t, "status", st, "id", "status", "scheduled_at", "published_at", "targets")
		if st["status"] != "scheduled" {
			t.Errorf("status: %v", st)
		}
		un := c.must("POST", "/api/v1/posts/"+postID+"/unschedule", nil, 200)
		if un["status"] != "draft" || un["scheduled_at"] != nil {
			t.Errorf("unschedule: %v", un)
		}
		c.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(at)}, 200)
		cancelled := c.must("POST", "/api/v1/posts/"+postID+"/cancel", nil, 200)
		if cancelled["status"] != "cancelled" || cancelled["targets"].([]any)[0].(map[string]any)["status"] != "cancelled" {
			t.Errorf("cancel: %v", cancelled)
		}
		for _, path := range []string{"/publish", "/schedule", "/retry", "/unschedule", "/cancel"} {
			if r := c.do("POST", "/api/v1/posts/"+postID+path, map[string]any{"scheduled_at": fmtTime(at)}); r.status != 409 || r.errCode(t) != "INVALID_STATE_TRANSITION" {
				t.Errorf("cancelled post %s: want 409 INVALID_STATE_TRANSITION, got %d %s", path, r.status, r.body)
			}
		}
		c.must("DELETE", "/api/v1/posts/"+postID, nil, 204)
		for _, tc := range []struct{ m, p string }{{"GET", ""}, {"GET", "/status"}, {"PATCH", ""}, {"DELETE", ""}, {"POST", "/publish"}} {
			if r := c.do(tc.m, "/api/v1/posts/"+postID+tc.p, map[string]any{"content": "x"}); r.status != 404 || r.errCode(t) != "NOT_FOUND" {
				t.Errorf("deleted post %s %s: %d %s", tc.m, tc.p, r.status, r.body)
			}
		}
	})

	t.Run("posts: publish retry failed", func(t *testing.T) {
		p := c.must("POST", "/api/v1/posts", map[string]any{"content": "#mock-fail contract", "social_account_ids": []string{acc}}, 201)
		id := p["id"].(string)
		pub := c.must("POST", "/api/v1/posts/"+id+"/publish", map[string]any{"confirm": true}, 202)
		if pub["status"] != "publishing" {
			t.Errorf("publish: %v", pub)
		}
		if r := c.do("POST", "/api/v1/posts/"+id+"/publish", nil); r.status != 409 {
			t.Errorf("publish while publishing: %d %s", r.status, r.body)
		}
		st := c.waitStatus(id, "failed", 15*time.Second)
		tg := st["targets"].([]any)[0].(map[string]any)
		if tg["error_code"] != "MOCK_REJECTED" || tg["error_message"] == nil || tg["error_message"] == "" || tg["external_post_id"] != nil {
			t.Errorf("failure details: %v", tg)
		}
		if r := c.do("POST", "/api/v1/posts/"+id+"/publish", nil); r.status != 409 || r.errCode(t) != "INVALID_STATE_TRANSITION" {
			t.Errorf("publish on failed must point to retry: %d %s", r.status, r.body)
		}
		if r := c.do("PATCH", "/api/v1/posts/"+id, map[string]any{"content": "edit"}); r.status != 409 {
			t.Errorf("edit failed post: %d", r.status)
		}
		// Retry with a future time re-schedules instead of publishing now.
		re := c.must("POST", "/api/v1/posts/"+id+"/retry", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))}, 202)
		if re["status"] != "scheduled" {
			t.Errorf("retry with scheduled_at: %v", re)
		}
		c.must("POST", "/api/v1/posts/"+id+"/cancel", nil, 200)
	})

	t.Run("posts: list pagination and filters", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			c.must("POST", "/api/v1/posts", map[string]any{"content": "page " + string(rune('a'+i)), "social_account_ids": []string{acc}}, 201)
		}
		seen := map[string]bool{}
		cursor := ""
		pages := 0
		for {
			q := "/api/v1/posts?limit=2"
			if cursor != "" {
				q += "&cursor=" + url.QueryEscape(cursor)
			}
			page := c.must("GET", q, nil, 200)
			hasKeys(t, "page", page, "items", "next_cursor")
			items := page["items"].([]any)
			if len(items) > 2 {
				t.Fatalf("limit ignored: %d items", len(items))
			}
			for _, it := range items {
				id := it.(map[string]any)["id"].(string)
				if seen[id] {
					t.Fatalf("duplicate %s across pages", id)
				}
				seen[id] = true
			}
			pages++
			if page["next_cursor"] == nil {
				break
			}
			cursor = page["next_cursor"].(string)
			if pages > 10 {
				t.Fatal("pagination does not terminate")
			}
		}
		if len(seen) < 4 || pages < 2 {
			t.Errorf("expected several pages, got %d posts over %d pages", len(seen), pages)
		}
		drafts := c.must("GET", "/api/v1/posts?status=draft&limit=100", nil, 200)["items"].([]any)
		for _, it := range drafts {
			if it.(map[string]any)["status"] != "draft" {
				t.Errorf("status filter leaked: %v", it)
			}
		}
		future := fmtTime(time.Now().Add(48 * time.Hour))
		if n := len(c.must("GET", "/api/v1/posts?from="+url.QueryEscape(future), nil, 200)["items"].([]any)); n != 0 {
			t.Errorf("from filter in the future matched %d posts", n)
		}
		for _, q := range []string{"limit=0", "limit=abc", "cursor=not-a-cursor", "cursor=bm9wZQ", "status=bogus", "from=yesterday", "to=2026-13-45"} {
			if r := c.do("GET", "/api/v1/posts?"+q, nil); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
				t.Errorf("?%s: want 400 VALIDATION_ERROR, got %d %s", q, r.status, r.body)
			}
		}
		if got := len(c.must("GET", "/api/v1/posts?limit=100000", nil, 200)["items"].([]any)); got > 100 {
			t.Errorf("limit not capped: %d", got)
		}
	})

	t.Run("media", func(t *testing.T) {
		up := c.upload("x.png", pngBytes(t))
		if up.status != 201 {
			t.Fatalf("upload: %d %s", up.status, up.body)
		}
		m := up.json(t)
		hasKeys(t, "media", m, "id", "kind", "mime_type", "size_bytes", "original_name", "width", "height", "sha256", "status", "created_at", "url")
		list := c.must("GET", "/api/v1/media", nil, 200)
		items := list["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["url"] == "" || list["next_cursor"] != nil {
			t.Errorf("media list: %v", list)
		}
		p := c.must("POST", "/api/v1/posts", map[string]any{"content": "with media", "social_account_ids": []string{acc}, "media_ids": []string{m["id"].(string)}}, 201)
		d := c.must("GET", "/api/v1/posts/"+p["id"].(string), nil, 200)
		media := d["media"].([]any)
		if len(media) != 1 || media[0].(map[string]any)["id"] != m["id"] || media[0].(map[string]any)["url"] == "" {
			t.Errorf("post detail must embed media objects: %v", d["media"])
		}
		if ids := d["media_ids"].([]any); len(ids) != 1 || ids[0] != m["id"] {
			t.Errorf("media_ids: %v", ids)
		}
		c.must("DELETE", "/api/v1/posts/"+p["id"].(string), nil, 204)
		// A soft-deleted post no longer pins its media.
		c.must("DELETE", "/api/v1/media/"+m["id"].(string), nil, 204)
		if r := c.do("GET", "/api/v1/media/"+m["id"].(string), nil); r.status != 404 {
			t.Errorf("deleted media: %d", r.status)
		}
	})

	t.Run("dashboard analytics audit", func(t *testing.T) {
		d := c.must("GET", "/api/v1/dashboard/summary", nil, 200)
		hasKeys(t, "dashboard", d, "connected_accounts", "scheduled_posts", "drafts", "published_this_month", "failed", "upcoming", "recent")
		if d["connected_accounts"].(float64) != 1 || d["drafts"].(float64) < 3 || d["failed"].(float64) != 0 {
			// the failed post was re-scheduled then cancelled in the lifecycle test
			t.Errorf("dashboard counters: %v", d)
		}
		if _, ok := d["upcoming"].([]any); !ok {
			t.Errorf("upcoming must be a list: %v", d["upcoming"])
		}
		recent, ok := d["recent"].([]any)
		if !ok {
			t.Fatalf("recent must be a list: %v", d["recent"])
		}
		for _, it := range recent {
			switch st := it.(map[string]any)["status"]; st {
			case "published", "partially_published", "failed":
			default:
				t.Errorf("recent publications must not include %v posts", st)
			}
		}
		an := c.must("GET", "/api/v1/analytics", nil, 200)
		hasKeys(t, "analytics", an, "from", "to", "totals", "items")
		if r := c.do("GET", "/api/v1/analytics?from=2026-01-01T00:00:00Z&to=2025-01-01T00:00:00Z", nil); r.status != 400 {
			t.Errorf("inverted range: %d", r.status)
		}
		if r := c.do("GET", "/api/v1/analytics?from=2020-01-01T00:00:00Z&to=2026-01-01T00:00:00Z", nil); r.status != 400 {
			t.Errorf("range over 366 days: %d", r.status)
		}

		first := c.must("GET", "/api/v1/audit-logs?limit=3", nil, 200)
		items := first["items"].([]any)
		if len(items) != 3 || first["next_cursor"] == nil {
			t.Fatalf("audit page 1: %v", first)
		}
		hasKeys(t, "audit entry", items[0].(map[string]any), "id", "actor_type", "actor_id", "actor_label", "action", "resource_type", "resource_id", "metadata", "request_id", "ip", "created_at")
		second := c.must("GET", "/api/v1/audit-logs?limit=3&cursor="+url.QueryEscape(first["next_cursor"].(string)), nil, 200)
		ids := map[any]bool{}
		for _, it := range append(items, second["items"].([]any)...) {
			id := it.(map[string]any)["id"]
			if ids[id] {
				t.Fatalf("audit pagination repeats %v", id)
			}
			ids[id] = true
		}
		times := []string{}
		for _, it := range items {
			times = append(times, it.(map[string]any)["created_at"].(string))
		}
		for i := 1; i < len(times); i++ {
			if times[i] > times[i-1] {
				t.Errorf("audit log must be newest first: %v", times)
			}
		}
	})

	t.Run("developer", func(t *testing.T) {
		// API keys
		for name, body := range map[string]map[string]any{
			"no name":       {"scopes": []string{"posts:read"}},
			"unknown scope": {"name": "k", "scopes": []string{"posts:everything"}},
			"expired":       {"name": "k", "scopes": []string{"posts:read"}, "expires_at": "2001-01-01T00:00:00Z"},
		} {
			if r := c.do("POST", "/api/v1/developer/api-keys", body); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
				t.Errorf("%s: %d %s", name, r.status, r.body)
			}
		}
		created := c.must("POST", "/api/v1/developer/api-keys", map[string]any{"name": "ci", "scopes": []string{"posts:read"}, "expires_at": fmtTime(time.Now().Add(24 * time.Hour))}, 201)
		raw := created["key"].(string)
		meta := created["api_key"].(map[string]any)
		if !regexp.MustCompile(`^sk_live_[A-Za-z0-9]{48}$`).MatchString(raw) || !strings.HasPrefix(raw, meta["prefix"].(string)) || created["raw_key"] != raw {
			t.Errorf("raw key / prefix: %q %v", raw, meta)
		}
		hasKeys(t, "api key", meta, "id", "name", "prefix", "scopes", "expires_at", "revoked_at", "last_used_at", "created_at")
		list := c.must("GET", "/api/v1/developer/api-keys", nil, 200)
		if strings.Contains(string(c.do("GET", "/api/v1/developer/api-keys", nil).body), raw) {
			t.Fatal("raw key listed after creation")
		}
		if scopes := list["available_scopes"].([]any); len(scopes) != 10 {
			t.Errorf("available_scopes: %v", scopes)
		}
		dangerous := 0
		for _, s := range list["available_scopes"].([]any) {
			if s.(map[string]any)["dangerous"] == true {
				dangerous++
			}
		}
		if dangerous != 4 {
			t.Errorf("expected 4 dangerous scopes, got %d", dangerous)
		}
		// Default scopes never include dangerous ones.
		def := c.must("POST", "/api/v1/developer/api-keys", map[string]any{"name": "defaults"}, 201)["api_key"].(map[string]any)
		for _, s := range def["scopes"].([]any) {
			if strings.Contains("posts:publish posts:delete social:disconnect social:connect", s.(string)) {
				t.Errorf("default scopes include %v", s)
			}
		}
		// Expiry is enforced.
		if _, err := e.app.DB.Pool.Exec(t.Context(), `UPDATE api_keys SET expires_at = now() - interval '1 minute' WHERE id = $1`, meta["id"]); err != nil {
			t.Fatal(err)
		}
		if r := e.apiKeyClient(raw).do("GET", "/api/v1/me", nil); r.status != 401 {
			t.Errorf("expired key accepted: %d", r.status)
		}
		c.must("DELETE", "/api/v1/developer/api-keys/"+meta["id"].(string), nil, 204)
		c.must("DELETE", "/api/v1/developer/api-keys/"+meta["id"].(string), nil, 204) // idempotent
		if r := c.do("DELETE", "/api/v1/developer/api-keys/00000000-0000-0000-0000-000000000009", nil); r.status != 404 {
			t.Errorf("revoke unknown: %d", r.status)
		}

		// MCP connections
		if r := c.do("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": ""}); r.status != 400 {
			t.Errorf("mcp without name: %d", r.status)
		}
		m := c.must("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": "Cursor", "client_name": "cursor", "scopes": []string{"posts:read", "social:read"}}, 201)
		rawMCP := m["key"].(string)
		hasKeys(t, "mcp response", m, "connection", "key", "config", "warning")
		conn := m["connection"].(map[string]any)
		hasKeys(t, "mcp connection", conn, "id", "name", "client_name", "api_key_id", "key_prefix", "scopes", "last_seen_at", "revoked_at", "created_at")
		cfg := m["config"].(map[string]any)
		hasKeys(t, "mcp config", cfg, "http", "stdio", "mcpServers", "claude_code")
		httpCfg := cfg["http"].(map[string]any)["mcpServers"].(map[string]any)["steerpost"].(map[string]any)
		stdioCfg := cfg["stdio"].(map[string]any)["mcpServers"].(map[string]any)["steerpost"].(map[string]any)
		if httpCfg["type"] != "http" || httpCfg["url"] != "http://mcp.test/mcp" || httpCfg["headers"].(map[string]any)["Authorization"] != "Bearer "+rawMCP {
			t.Errorf("http config not ready to paste: %v", httpCfg)
		}
		env := stdioCfg["env"].(map[string]any)
		if stdioCfg["command"] != "npx" || env["SOCIALOS_AUTH_HEADER"] != "Bearer "+rawMCP || strings.Contains(fmt.Sprint(stdioCfg["args"]), "socialos-mcp") {
			t.Errorf("stdio config not ready to paste: %v", stdioCfg)
		}
		if !strings.Contains(cfg["claude_code"].(string), rawMCP) || !strings.Contains(cfg["claude_code"].(string), "http://mcp.test/mcp") {
			t.Errorf("claude_code command: %v", cfg["claude_code"])
		}
		if strings.Contains(string(c.do("GET", "/api/v1/developer/mcp-connections", nil).body), rawMCP) {
			t.Fatal("MCP raw key listed after creation")
		}
		e.apiKeyClient(rawMCP).must("GET", "/api/v1/posts", nil, 200)
		usage := c.must("GET", "/api/v1/developer/usage", nil, 200)
		hasKeys(t, "usage", usage, "total_requests", "by_key", "by_day", "window_days")
		if usage["total_requests"].(float64) < 1 {
			t.Errorf("usage did not count the key request: %v", usage)
		}
		days := usage["by_day"].([]any)
		if len(days) != 30 {
			t.Errorf("by_day should cover the 30-day window, got %d days", len(days))
		}
		last := days[len(days)-1].(map[string]any)
		if last["day"] != time.Now().UTC().Format("2006-01-02") || last["requests"].(float64) < 1 {
			t.Errorf("today's bucket: %v", last)
		}
		for _, k := range usage["by_key"].([]any) {
			hasKeys(t, "by_key", k.(map[string]any), "api_key_id", "name", "prefix", "requests", "last_used_at")
		}
		c.must("DELETE", "/api/v1/developer/mcp-connections/"+conn["id"].(string), nil, 204)
		if r := e.apiKeyClient(rawMCP).do("GET", "/api/v1/me", nil); r.status != 401 {
			t.Errorf("revoked MCP key: %d", r.status)
		}
		if r := c.do("DELETE", "/api/v1/developer/mcp-connections/"+conn["id"].(string), nil); r.status != 204 {
			t.Errorf("revoke twice: %d", r.status)
		}
	})

	t.Run("account disconnect", func(t *testing.T) {
		c.must("DELETE", "/api/v1/social/accounts/"+acc, nil, 204)
		if n := len(c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any)); n != 0 {
			t.Errorf("disconnected account still listed: %d", n)
		}
		var creds int
		_ = e.app.DB.Pool.QueryRow(t.Context(), `SELECT count(*) FROM oauth_credentials WHERE social_account_id = $1`, acc).Scan(&creds)
		if creds != 0 {
			t.Errorf("tokens must be destroyed on disconnect, %d rows remain", creds)
		}
		if r := c.do("POST", "/api/v1/posts", map[string]any{"content": "x", "social_account_ids": []string{acc}}); r.status != 400 {
			t.Errorf("posting to a disconnected account: %d %s", r.status, r.body)
		}
	})

	t.Run("unknown routes use the error envelope", func(t *testing.T) {
		for _, p := range []string{"/api/v1/nope", "/nope", "/api/v1/posts/x/y/z"} {
			if r := c.do("GET", p, nil); r.status != 404 || r.errCode(t) != "NOT_FOUND" {
				t.Errorf("%s: %d %s", p, r.status, r.body)
			}
		}
	})
}

// TestMeRequiresAuthentication ensures nothing but the documented public routes answers anonymously.
func TestMeRequiresAuthentication(t *testing.T) {
	e := newEnv(t, envOpts{})
	anon := e.browser()
	for _, tc := range []struct{ m, p string }{
		{"GET", "/api/v1/me"}, {"POST", "/api/v1/auth/logout"}, {"GET", "/api/v1/social/providers"}, {"GET", "/api/v1/social/accounts"},
		{"GET", "/api/v1/posts"}, {"POST", "/api/v1/posts"}, {"GET", "/api/v1/media"}, {"POST", "/api/v1/media"}, {"GET", "/api/v1/analytics"},
		{"GET", "/api/v1/dashboard/summary"}, {"GET", "/api/v1/audit-logs"}, {"GET", "/api/v1/developer/api-keys"},
		{"POST", "/api/v1/developer/api-keys"}, {"GET", "/api/v1/developer/mcp-connections"}, {"GET", "/api/v1/developer/usage"},
		{"POST", "/api/v1/social/telegram/connect"}, {"GET", "/api/v1/social/telegram/connect/00000000-0000-0000-0000-000000000000"},
		{"GET", "/api/v1/social/mock/connect"},
	} {
		if r := anon.do(tc.m, tc.p, nil); r.status != 401 || r.errCode(t) != "UNAUTHENTICATED" {
			t.Errorf("%s %s anonymous: want 401 UNAUTHENTICATED, got %d %s", tc.m, tc.p, r.status, r.body)
		}
	}
	for _, auth := range []string{"Bearer", "Basic abc", "Bearer sk_test_nothing", "Bearer " + strings.Repeat("a", 5000)} {
		req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/me", nil)
		req.Header.Set("Authorization", auth)
		if r := anon.send(req); r.status != 401 || r.errCode(t) != "UNAUTHENTICATED" {
			t.Errorf("Authorization %.20q: %d %s", auth, r.status, r.body)
		}
	}
}
