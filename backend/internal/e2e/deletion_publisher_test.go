package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/config"
)

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.app.DB.Pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *env) activeJobs(postID string) []scheduler.Payload {
	e.t.Helper()
	rows, err := e.app.DB.Pool.Query(context.Background(), `SELECT j.post_target_id, j.id FROM scheduled_jobs j
		JOIN post_targets t ON t.id = j.post_target_id WHERE t.post_id = $1 AND j.status IN ('pending','enqueued') ORDER BY j.id`, postID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []scheduler.Payload
	for rows.Next() {
		var pl scheduler.Payload
		if err := rows.Scan(&pl.TargetID, &pl.JobID); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, pl)
	}
	return out
}

func (e *env) publish(pl scheduler.Payload) error {
	return e.app.Publisher.Run(context.Background(), pl, scheduler.RetryInfo{MaxRetry: scheduler.MaxRetry})
}

// twoMockAccounts connects the mock network and adds a second account row, so one post can have two targets.
func (c *client) twoMockAccounts() (string, string) {
	c.e.t.Helper()
	first := c.connectMock()
	var second string
	err := c.e.app.DB.Pool.QueryRow(context.Background(), `INSERT INTO social_accounts (user_id, provider, provider_account_id, username, status)
		SELECT user_id, provider, provider_account_id || '-2', username, status FROM social_accounts WHERE id = $1 RETURNING id::text`, first).Scan(&second)
	if err != nil {
		c.e.t.Fatal(err)
	}
	return first, second
}

// The race the publisher guards: a job comes due after the owner asked for deletion but before the posts were stopped.
// The post becomes a draft, EVERY job of the post is cancelled (not just the one that ran), and once the deletion is
// cancelled the post can be scheduled again without tripping the one-active-job-per-target index.
func TestPublisherSkipsScheduledPostOfAccountBeingDeletedAndItCanBeRescheduled(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Now().UTC())
	e := newEnv(t, envOpts{clock: clk, mutate: func(c *config.Config) { c.SessionTTL = 90 * 24 * time.Hour }})
	c := e.browser()
	uid := c.register("skip@delete.test")["user"].(map[string]any)["id"].(string)
	a1, a2 := c.twoMockAccounts()
	postID := c.must("POST", "/api/v1/posts", map[string]any{"content": "two targets", "social_account_ids": []string{a1, a2}}, 201)["id"].(string)
	at := fmtTime(clk.Now().Add(time.Hour))
	c.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": at}, 200)
	jobs := e.activeJobs(postID)
	if len(jobs) != 2 {
		t.Fatalf("active jobs = %d, want 2", len(jobs))
	}

	e.exec(`UPDATE users SET deletion_scheduled_at = $2 WHERE id = $1`, uid, clk.Now().Add(7*24*time.Hour))
	clk.set(clk.Now().Add(2 * time.Hour))
	for _, pl := range jobs {
		if err := e.publish(pl); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	if got := c.must("GET", "/api/v1/posts/"+postID, nil, 200)["status"]; got != "draft" {
		t.Fatalf("post status = %v, want draft", got)
	}
	if n := len(e.activeJobs(postID)); n != 0 {
		t.Fatalf("%d jobs still active; the skip must cancel every job of the post", n)
	}
	if e.count(`SELECT count(*) FROM post_attempts a JOIN post_targets t ON t.id = a.post_target_id WHERE t.post_id = $1`, postID) != 0 {
		t.Fatal("an attempt was made for an account being deleted")
	}

	e.exec(`UPDATE users SET deletion_scheduled_at = NULL WHERE id = $1`, uid)
	c.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(clk.Now().Add(time.Hour))}, 200)
	if n := len(e.activeJobs(postID)); n != 2 {
		t.Fatalf("after re-scheduling: %d active jobs, want 2", n)
	}
}

// A post that is already publishing cannot become a draft: the target fails with ACCOUNT_DELETION_SCHEDULED, the post
// settles to failed, and the job is finished.
func TestPublisherFailsTargetOfPublishingPostOfAccountBeingDeleted(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Now().UTC())
	e := newEnv(t, envOpts{clock: clk, mutate: func(c *config.Config) { c.SessionTTL = 90 * 24 * time.Hour }})
	c := e.browser()
	uid := c.register("skip2@delete.test")["user"].(map[string]any)["id"].(string)
	acc := c.connectMock()
	postID := c.must("POST", "/api/v1/posts", map[string]any{"content": "in flight", "social_account_ids": []string{acc}}, 201)["id"].(string)
	c.must("POST", "/api/v1/posts/"+postID+"/schedule", map[string]any{"scheduled_at": fmtTime(clk.Now().Add(time.Hour))}, 200)
	jobs := e.activeJobs(postID)
	e.exec(`UPDATE posts SET status = 'publishing' WHERE id = $1`, postID)
	e.exec(`UPDATE users SET deletion_scheduled_at = $2 WHERE id = $1`, uid, clk.Now().Add(7*24*time.Hour))
	clk.set(clk.Now().Add(2 * time.Hour))

	if err := e.publish(jobs[0]); err != nil {
		t.Fatalf("publish: %v", err)
	}
	st := c.must("GET", "/api/v1/posts/"+postID+"/status", nil, 200)
	if st["status"] != "failed" {
		t.Fatalf("post status = %v, want failed", st["status"])
	}
	if tg := st["targets"].([]any)[0].(map[string]any); tg["status"] != "failed" || tg["error_code"] != "ACCOUNT_DELETION_SCHEDULED" {
		t.Fatalf("target = %v", tg)
	}
	if n := len(e.activeJobs(postID)); n != 0 {
		t.Fatalf("%d jobs still active", n)
	}
}
