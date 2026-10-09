package e2e

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/domain/dataexport"
)

func deletionEnv(t *testing.T, cm *captureMailer) (*env, *steppingClock) {
	t.Helper()
	clk := &steppingClock{}
	clk.set(time.Now().UTC())
	return newEnv(t, envOpts{startWorker: true, clock: clk, mailer: cm, mutate: func(c *config.Config) {
		c.SessionTTL = 90 * 24 * time.Hour
	}}), clk
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.app.DB.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (c *client) deleteAccount(password, confirm string) resp {
	return c.do("POST", "/api/v1/account/delete", map[string]any{"password": password, "confirm": confirm})
}

// cookiesOf copies the browser's cookies into a new client, to prove an old session is dead.
func (e *env) replay(c *client) *client {
	u, _ := url.Parse(e.srv.URL)
	old := e.browser()
	old.http.Jar.SetCookies(u, c.http.Jar.Cookies(u))
	old.csrf = c.csrf
	return old
}

func (e *env) waitPurged(userID string) {
	e.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if e.count(`SELECT count(*) FROM users WHERE id = $1`, userID) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	e.t.Fatal("the account was never purged")
}

// The full flow: ask (re-auth and typed email), everything is revoked at once, the owner can come back and cancel, ask
// again, and after the grace period the worker deletes every row and object, while the same address can register again.
func TestAccountDeletionFlow(t *testing.T) {
	cm := &captureMailer{}
	e, clk := deletionEnv(t, cm)
	alice, bob := e.browser(), e.browser()
	me := alice.register("alice@delete.test")
	uid := me["user"].(map[string]any)["id"].(string)
	bob.register("bob@delete.test")
	bobAcc := bob.connectMock()
	bobMedia := bob.upload("b.png", pngBytes(t)).json(t)["id"].(string)
	bob.must("POST", "/api/v1/posts", map[string]any{"content": "bob keeps this", "social_account_ids": []string{bobAcc}, "media_ids": []string{bobMedia}}, 201)

	acc := alice.connectMock()
	mediaID := alice.upload("a.png", pngBytes(t)).json(t)["id"].(string)
	post := alice.must("POST", "/api/v1/posts", map[string]any{"content": "alice scheduled", "social_account_ids": []string{acc}, "media_ids": []string{mediaID}}, 201)
	postID := post["id"].(string)
	alice.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(clk.Now().Add(48 * time.Hour))}, 200)
	rawKey := alice.createKey("agent", "posts:read")
	exportID := alice.must("POST", "/api/v1/account/exports", nil, 202)["id"].(string)
	alice.waitExport(exportID, "ready")
	aliceKey := dataexport.ObjectKey(uuid.MustParse(uid), uuid.MustParse(exportID))

	// Refused attempts change nothing.
	if r := alice.deleteAccount("wrong password", "alice@delete.test"); r.status != 400 || r.errBody(t)["fields"].(map[string]any)["password"] == nil {
		t.Fatalf("wrong password: %d %s", r.status, r.body)
	}
	if r := alice.deleteAccount("correct horse battery", "alice@wrong.test"); r.status != 400 || r.errBody(t)["fields"].(map[string]any)["confirm"] == nil {
		t.Fatalf("wrong confirmation: %d %s", r.status, r.body)
	}
	if r := e.apiKeyClient(rawKey).do("POST", "/api/v1/account/delete", map[string]any{"password": "correct horse battery", "confirm": "alice@delete.test"}); r.status != http.StatusForbidden {
		t.Fatalf("an API key asked for deletion: %d %s", r.status, r.body)
	}
	alice.must("GET", "/api/v1/me", nil, 200)
	stale := e.replay(alice)

	// The request: 202, cookies cleared, sessions and keys dead, scheduled post back to a draft.
	r := alice.deleteAccount("correct horse battery", "ALICE@delete.test")
	if r.status != http.StatusAccepted || r.json(t)["status"] != "scheduled" {
		t.Fatalf("request: %d %s", r.status, r.body)
	}
	want := clk.Now().Add(7 * 24 * time.Hour)
	if at, err := time.Parse(time.RFC3339, r.json(t)["scheduled_for"].(string)); err != nil || at.Sub(want).Abs() > 2*time.Second {
		t.Fatalf("scheduled_for %v, want about %v", r.json(t)["scheduled_for"], want)
	}
	if got := strings.Join(r.header.Values("Set-Cookie"), ";"); !strings.Contains(got, "Max-Age=0") && !strings.Contains(got, "Max-Age=-1") {
		t.Fatalf("session cookies not cleared: %q", got)
	}
	if s := stale.do("GET", "/api/v1/me", nil); s.status != http.StatusUnauthorized {
		t.Fatalf("old session still works: %d", s.status)
	}
	if s := e.apiKeyClient(rawKey).do("GET", "/api/v1/posts", nil); s.status != http.StatusUnauthorized {
		t.Fatalf("old API key still works: %d", s.status)
	}

	// Signing in during the grace period works, shows the schedule, and the owner can cancel.
	back := e.browser()
	if l := back.login("alice@delete.test", "correct horse battery"); l.status != 200 {
		t.Fatalf("login in grace period: %d %s", l.status, l.body)
	} else {
		back.csrf, _ = l.json(t)["csrf_token"].(string)
		if l.json(t)["user"].(map[string]any)["deletion_scheduled_at"] == nil {
			t.Fatalf("login does not report the schedule: %s", l.body)
		}
	}
	if st := back.must("GET", "/api/v1/posts/"+postID, nil, 200)["status"]; st != "draft" {
		t.Fatalf("scheduled post is %v during the grace period", st)
	}
	// While the deletion is pending the account cannot schedule or publish anything.
	for _, tc := range []struct {
		path string
		body any
	}{
		{"/api/v1/posts/" + postID + "/schedule", map[string]any{"scheduled_at": fmtTime(clk.Now().Add(48 * time.Hour))}},
		{"/api/v1/posts/" + postID + "/publish", nil},
	} {
		if r := back.do("POST", tc.path, tc.body); r.status != http.StatusConflict || !strings.Contains(string(r.body), "scheduled for deletion") {
			t.Fatalf("POST %s during the grace period: %d %s", tc.path, r.status, r.body)
		}
	}
	if st := back.must("GET", "/api/v1/posts/"+postID, nil, 200)["status"]; st != "draft" {
		t.Fatalf("post is %v after refused attempts", st)
	}
	back.must("POST", "/api/v1/account/delete/cancel", nil, 204)
	// Cancelling gives the account back its rights.
	back.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(clk.Now().Add(48 * time.Hour))}, 200)
	back.must("POST", "/api/v1/posts/"+postID+"/unschedule", nil, 200)
	if back.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["deletion_scheduled_at"] != nil {
		t.Fatal("cancel did not clear the schedule")
	}
	if r := back.do("POST", "/api/v1/account/delete/cancel", nil); r.status != http.StatusConflict {
		t.Fatalf("second cancel: %d", r.status)
	}
	if n := e.count(`SELECT count(*) FROM account_deletions WHERE user_id = $1`, uid); n != 0 {
		t.Fatalf("cancelled request left %d records", n)
	}
	// A sweep after the grace period finds nothing to purge for a cancelled deletion.
	clk.set(clk.Now().Add(9 * 24 * time.Hour))
	if n, err := e.app.Services.Deletion.Sweep(context.Background()); err != nil || n != 0 {
		t.Fatalf("sweep of cancelled deletion: %d %v", n, err)
	}
	back.must("GET", "/api/v1/me", nil, 200)

	// Ask again; the grace period has to pass again.
	if r := back.deleteAccount("correct horse battery", "alice@delete.test"); r.status != 202 {
		t.Fatalf("second request: %d %s", r.status, r.body)
	}
	if n, _ := e.app.Services.Deletion.Sweep(context.Background()); n != 0 {
		t.Fatalf("sweep inside the grace period queued %d", n)
	}
	// An object no row knows about (a failed export's leftover) must go with the account, another user's must stay.
	leftover := "users/" + uid + "/exports/leftover.zip"
	_ = e.storage.Put(context.Background(), leftover, strings.NewReader("x"), 1, "application/zip")
	clk.set(clk.Now().Add(8 * 24 * time.Hour))
	if n, err := e.app.Services.Deletion.Sweep(context.Background()); err != nil || n != 1 {
		t.Fatalf("sweep after the grace period: %d %v", n, err)
	}
	e.waitPurged(uid)

	// Everything of Alice is gone, in the database and in storage; Bob has all of his.
	for _, table := range []string{"posts", "post_targets", "media", "social_accounts", "api_keys", "sessions", "audit_logs", "data_exports", "scheduled_jobs"} {
		col := "user_id"
		query := `SELECT count(*) FROM ` + table + ` WHERE ` + col + ` = $1`
		if table == "scheduled_jobs" {
			query = `SELECT count(*) FROM scheduled_jobs j JOIN post_targets t ON t.id = j.post_target_id WHERE t.user_id = $1`
		}
		if n := e.count(query, uid); n != 0 {
			t.Errorf("%s: %d rows left", table, n)
		}
	}
	if e.storage.Has("users/"+uid+"/media/"+mediaID+".png") || e.storage.Has(aliceKey) || e.storage.Has(leftover) {
		t.Error("objects left in storage")
	}
	for _, k := range []string{"posts", "media", "social_accounts", "audit_logs"} {
		if n := e.count(`SELECT count(*) FROM `+k+` WHERE user_id <> $1`, uid); n == 0 {
			t.Errorf("bob lost his %s", k)
		}
	}
	if st := bob.must("GET", "/api/v1/media/"+bobMedia, nil, 200); st["id"] != bobMedia {
		t.Fatalf("bob's media: %v", st)
	}
	var purged *time.Time
	var posts int
	row := e.app.DB.Pool.QueryRow(context.Background(), `SELECT purged_at, (counts->>'posts')::int FROM account_deletions WHERE user_id = $1`, uid)
	if err := row.Scan(&purged, &posts); err != nil || purged == nil || posts != 1 {
		t.Fatalf("deletion record: %v %v %d", err, purged, posts)
	}
	if l := e.browser().login("alice@delete.test", "correct horse battery"); l.status != http.StatusUnauthorized {
		t.Fatalf("a deleted account can sign in: %d", l.status)
	}
	// The address is free again.
	e.browser().register("alice@delete.test")
	for _, tmpl := range []string{"account_deletion_scheduled", "account_deleted"} {
		deadline := time.Now().Add(10 * time.Second)
		for !cm.has(tmpl) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if !cm.has(tmpl) {
			t.Errorf("no %s mail was sent", tmpl)
		}
	}
}

func (c *captureMailer) has(template string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.got {
		if m.Template == template {
			return true
		}
	}
	return false
}
