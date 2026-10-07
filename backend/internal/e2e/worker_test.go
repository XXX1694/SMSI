package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/adapters/mock"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
)

// workerEnv runs the publisher directly (no Asynq server) for deterministic tests.
type workerEnv struct {
	*env
	c *client
}

func newWorkerEnv(t *testing.T, ps ...provider.Provider) *workerEnv {
	e := newEnv(t, envOpts{providers: ps})
	c := e.browser()
	c.register(uuid.NewString()[:8] + "@worker.test")
	return &workerEnv{env: e, c: c}
}

func (w *workerEnv) connect(providerName string) string {
	w.t.Helper()
	r := w.c.do("GET", "/api/v1/social/"+providerName+"/connect", nil)
	if r.status != 302 {
		w.t.Fatalf("connect %s: %d %s", providerName, r.status, r.body)
	}
	for _, a := range w.c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any) {
		if m := a.(map[string]any); m["provider"] == providerName {
			return m["id"].(string)
		}
	}
	w.t.Fatalf("%s not connected", providerName)
	return ""
}

// publishJob creates a post, calls publish (enqueue only; no worker running) and returns payload.
func (w *workerEnv) publishJob(accountID, content string) (string, scheduler.Payload) {
	w.t.Helper()
	p := w.c.must("POST", "/api/v1/posts", map[string]any{"content": content, "social_account_ids": []string{accountID}}, 201)
	id := p["id"].(string)
	w.c.must("POST", "/api/v1/posts/"+id+"/publish", nil, 202)
	return id, w.activePayload(id)
}

func (w *workerEnv) activePayload(postID string) scheduler.Payload {
	w.t.Helper()
	var pl scheduler.Payload
	err := w.app.DB.Pool.QueryRow(context.Background(), `SELECT j.post_target_id, j.id FROM scheduled_jobs j
		JOIN post_targets t ON t.id = j.post_target_id WHERE t.post_id = $1 AND j.status IN ('pending','enqueued')`, postID).Scan(&pl.TargetID, &pl.JobID)
	if err != nil {
		w.t.Fatalf("active job: %v", err)
	}
	return pl
}

func (w *workerEnv) run(pl scheduler.Payload, retried int) error {
	return w.app.Publisher.Run(context.Background(), pl, scheduler.RetryInfo{Retried: retried, MaxRetry: scheduler.MaxRetry})
}

func (w *workerEnv) status(postID string) (string, map[string]any) {
	st := w.c.must("GET", "/api/v1/posts/"+postID+"/status", nil, 200)
	return st["status"].(string), st["targets"].([]any)[0].(map[string]any)
}

func (w *workerEnv) exec(sql string, args ...any) {
	w.t.Helper()
	if _, err := w.app.DB.Pool.Exec(context.Background(), sql, args...); err != nil {
		w.t.Fatalf("exec %q: %v", sql, err)
	}
}

func mockOf(t *testing.T, w *workerEnv) *mock.Provider {
	p, err := w.app.Registry.Get("mock")
	if err != nil {
		t.Fatal(err)
	}
	return p.(*mock.Provider)
}

func TestIdempotentDuplicateAndConcurrentRuns(t *testing.T) {
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	m := mockOf(t, w)

	postID, pl := w.publishJob(acc, "dup")
	if err := w.run(pl, 0); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := w.run(pl, 0); err != nil { // late duplicate task
		t.Fatalf("duplicate run: %v", err)
	}
	if st, _ := w.status(postID); st != "published" || m.Calls(pl.TargetID.String()) != 1 {
		t.Fatalf("status %s calls %d", st, m.Calls(pl.TargetID.String()))
	}

	postID2, pl2 := w.publishJob(acc, "concurrent")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = w.run(pl2, 0) }()
	}
	wg.Wait()
	for i := 0; i < 3; i++ { // retries of in-flight losers are no-ops now
		_ = w.run(pl2, 1)
	}
	if st, _ := w.status(postID2); st != "published" {
		t.Fatalf("concurrent post status %s", st)
	}
	if n := m.Calls(pl2.TargetID.String()); n != 1 {
		t.Fatalf("provider called %d times, want exactly 1", n)
	}
	var attempts int
	_ = w.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM publication_attempts WHERE post_target_id = $1`, pl2.TargetID).Scan(&attempts)
	if attempts != 1 {
		t.Fatalf("attempts %d want 1", attempts)
	}
}

func TestRetryableThenSuccess(t *testing.T) {
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	postID, pl := w.publishJob(acc, "flaky #mock-retry")
	err := w.run(pl, 0)
	if !scheduler.IsRetryable(err) {
		t.Fatalf("expected retryable error, got %v", err)
	}
	if st, tg := w.status(postID); st != "publishing" || tg["status"] != "pending" || tg["error_code"] != "HTTP_503" {
		t.Fatalf("after retryable: %s %v", st, tg)
	}
	if err := w.run(pl, 1); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if st, tg := w.status(postID); st != "published" || tg["attempt_count"].(float64) != 2 {
		t.Fatalf("after retry: %s %v", st, tg)
	}
}

func TestRetriesExhaustedFails(t *testing.T) {
	flaky := newNoLookup("flaky")
	flaky.fail = provider.FromHTTPStatus("flaky", 503, "down", 0)
	w := newWorkerEnv(t, flaky)
	acc := w.connect("flaky")
	postID, pl := w.publishJob(acc, "never")
	for i := 0; i < scheduler.MaxRetry; i++ {
		if err := w.run(pl, i); !scheduler.IsRetryable(err) {
			t.Fatalf("run %d: expected retryable, got %v", i, err)
		}
	}
	if err := w.run(pl, scheduler.MaxRetry); err != nil {
		t.Fatalf("final run should settle: %v", err)
	}
	if st, tg := w.status(postID); st != "failed" || tg["status"] != "failed" {
		t.Fatalf("expected failed: %s %v", st, tg)
	}
	if flaky.calls.Load() != int32(scheduler.MaxRetry+1) {
		t.Fatalf("calls %d", flaky.calls.Load())
	}
}

func TestUnknownOutcomeResolvedByLookup(t *testing.T) {
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	m := mockOf(t, w)
	postID, pl := w.publishJob(acc, "timeout #mock-unknown")
	if err := w.run(pl, 0); !scheduler.IsRetryable(err) {
		t.Fatalf("expected retry after unknown outcome, got %v", err)
	}
	if err := w.run(pl, 1); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.status(postID); st != "published" {
		t.Fatalf("status %s", st)
	}
	if n := m.Calls(pl.TargetID.String()); n != 1 {
		t.Fatalf("re-posted after unknown outcome: %d calls", n)
	}
}

func TestCrashRecoveryGoesToNeedsReview(t *testing.T) {
	nl := newNoLookup("nolookup")
	w := newWorkerEnv(t, nl)
	acc := w.connect("nolookup")
	postID, pl := w.publishJob(acc, "crash")
	// Simulate a worker that crashed after calling the provider: stale `started` attempt.
	w.exec(`UPDATE post_targets SET status = 'publishing', attempt_count = 1 WHERE id = $1`, pl.TargetID)
	w.exec(`UPDATE posts SET status = 'publishing' WHERE id = $1`, postID)
	w.exec(`INSERT INTO publication_attempts (post_target_id, attempt_no, started_at, status) VALUES ($1, 1, now() - interval '10 minutes', 'started')`, pl.TargetID)
	if err := w.run(pl, 1); err != nil {
		t.Fatal(err)
	}
	st, tg := w.status(postID)
	if tg["status"] != "needs_review" || st != "failed" || nl.calls.Load() != 0 {
		t.Fatalf("expected needs_review without re-posting: %s %v calls=%d", st, tg, nl.calls.Load())
	}
	// A fresh `started` attempt (another live worker) must not be touched.
	postID2, pl2 := w.publishJob(acc, "inflight")
	w.exec(`INSERT INTO publication_attempts (post_target_id, attempt_no, started_at, status) VALUES ($1, 1, now(), 'started')`, pl2.TargetID)
	w.exec(`UPDATE post_targets SET attempt_count = 1 WHERE id = $1`, pl2.TargetID)
	if err := w.run(pl2, 0); !scheduler.IsRetryable(err) {
		t.Fatalf("in-flight attempt should yield retry, got %v", err)
	}
	if st, _ := w.status(postID2); st != "publishing" || nl.calls.Load() != 0 {
		t.Fatalf("in-flight post touched: %s", st)
	}
	// Explicit retry of needs_review targets is allowed.
	w.c.must("POST", "/api/v1/posts/"+postID+"/retry", map[string]any{"include_needs_review": true}, 202)
	if err := w.run(w.activePayload(postID), 0); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.status(postID); st != "published" {
		t.Fatalf("after explicit retry: %s", st)
	}
}

func TestAuthFailureExpiresAccount(t *testing.T) {
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	postID, pl := w.publishJob(acc, "revoked #mock-auth")
	if err := w.run(pl, 0); err != nil {
		t.Fatal(err)
	}
	if st, tg := w.status(postID); st != "failed" || tg["error_code"] != "SOCIAL_ACCOUNT_EXPIRED" {
		t.Fatalf("got %s %v", st, tg)
	}
	a := w.c.must("GET", "/api/v1/social/accounts/"+acc, nil, 200)
	if a["status"] != "expired" {
		t.Fatalf("account status %v", a["status"])
	}
	r := w.c.do("POST", "/api/v1/posts", map[string]any{"content": "x", "social_account_ids": []string{acc},
		"scheduled_at": fmtTime(time.Now().Add(time.Hour)), "schedule": true})
	if r.status != 422 || r.errCode(t) != "SOCIAL_ACCOUNT_EXPIRED" {
		t.Fatalf("scheduling on expired account: %d %s", r.status, r.body)
	}
}

func TestTokenRefreshBeforePublish(t *testing.T) {
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	accID := uuid.MustParse(acc)
	w.exec(`UPDATE oauth_credentials SET expires_at = now() + interval '1 minute' WHERE social_account_id = $1`, accID)
	postID, pl := w.publishJob(acc, "refresh me")
	if err := w.run(pl, 0); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.status(postID); st != "published" {
		t.Fatalf("status %s", st)
	}
	creds, err := w.app.Services.Accounts.Vault().Load(context.Background(), accID)
	if err != nil || creds.AccessToken != "mock-access-refreshed" || !creds.ExpiresAt.After(time.Now().Add(30*time.Minute)) {
		t.Fatalf("token not refreshed: %+v %v", creds.ExpiresAt, err)
	}
	var enc string
	_ = w.app.DB.Pool.QueryRow(context.Background(), `SELECT access_token_enc FROM oauth_credentials WHERE social_account_id = $1`, accID).Scan(&enc)
	if enc == "" || enc == creds.AccessToken || enc[:3] != "v1:" {
		t.Fatalf("token stored in plaintext: %q", enc)
	}

	// Refresh failure (revoked refresh token) → account expired, target failed.
	w.exec(`UPDATE oauth_credentials SET expires_at = now() - interval '1 minute', refresh_token_enc = '' WHERE social_account_id = $1`, accID)
	postID2, pl2 := w.publishJob(acc, "cannot refresh")
	if err := w.run(pl2, 0); err != nil {
		t.Fatal(err)
	}
	if st, tg := w.status(postID2); st != "failed" || tg["error_code"] != "SOCIAL_ACCOUNT_EXPIRED" {
		t.Fatalf("got %s %v", st, tg)
	}
}

func TestReconcilerRecoversLostAndStuckJobs(t *testing.T) {
	nl := newNoLookup("nolookup")
	w := newWorkerEnv(t, nl)
	acc := w.connect("mock")
	ctx := context.Background()

	// Lost task (e.g. Redis flushed): job enqueued in DB but missing in Redis.
	_, pl := w.publishJob(acc, "lost")
	w.exec(`UPDATE scheduled_jobs SET asynq_task_id = 'gone:1', run_at = now() - interval '5 minutes' WHERE id = $1`, pl.JobID)
	rep, err := w.app.Reconciler.RunOnce(ctx)
	if err != nil || rep.Reenqueued < 1 {
		t.Fatalf("reconcile lost: %+v %v", rep, err)
	}
	var taskID string
	_ = w.app.DB.Pool.QueryRow(ctx, `SELECT asynq_task_id FROM scheduled_jobs WHERE id = $1`, pl.JobID).Scan(&taskID)
	if st, err := w.app.Queue.TaskState(ctx, taskID); err != nil || st != scheduler.TaskLive {
		t.Fatalf("task not live: %v %v", st, err)
	}

	// Never-enqueued job (Redis down at schedule time).
	_, pl2 := w.publishJob(acc, "never enqueued")
	w.exec(`ALTER TABLE scheduled_jobs DISABLE TRIGGER scheduled_jobs_updated_at`)
	w.exec(`UPDATE scheduled_jobs SET asynq_task_id = '', status = 'pending', updated_at = now() - interval '1 minute' WHERE id = $1`, pl2.JobID)
	w.exec(`ALTER TABLE scheduled_jobs ENABLE TRIGGER scheduled_jobs_updated_at`)
	if rep, err := w.app.Reconciler.RunOnce(ctx); err != nil || rep.Enqueued < 1 {
		t.Fatalf("reconcile pending: %+v %v", rep, err)
	}

	// Stuck publishing target of a non-idempotent provider → needs_review.
	nacc := w.connect("nolookup")
	postID3, pl3 := w.publishJob(nacc, "stuck")
	w.exec(`INSERT INTO publication_attempts (post_target_id, attempt_no, started_at, status) VALUES ($1, 1, now() - interval '20 minutes', 'started')`, pl3.TargetID)
	w.exec(`UPDATE posts SET status = 'publishing' WHERE id = $1`, postID3)
	w.exec(`ALTER TABLE post_targets DISABLE TRIGGER post_targets_updated_at`)
	w.exec(`UPDATE post_targets SET status = 'publishing', attempt_count = 1, updated_at = now() - interval '20 minutes' WHERE id = $1`, pl3.TargetID)
	w.exec(`ALTER TABLE post_targets ENABLE TRIGGER post_targets_updated_at`)
	if rep, err := w.app.Reconciler.RunOnce(ctx); err != nil || rep.Recovered != 1 {
		t.Fatalf("reconcile stuck: %+v %v", rep, err)
	}
	if _, tg := w.status(postID3); tg["status"] != "needs_review" {
		t.Fatalf("stuck target: %v", tg)
	}
}

func TestInvalidStateTransitionsHTTP(t *testing.T) {
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	p := w.c.must("POST", "/api/v1/posts", map[string]any{"content": "sm", "social_account_ids": []string{acc}}, 201)
	id := p["id"].(string)
	for _, path := range []string{"/retry", "/unschedule"} {
		if r := w.c.do("POST", "/api/v1/posts/"+id+path, nil); r.status != 409 || r.errCode(t) != "INVALID_STATE_TRANSITION" {
			t.Errorf("draft %s: %d %s", path, r.status, r.body)
		}
	}
	if r := w.c.do("POST", "/api/v1/posts/"+id+"/schedule", map[string]any{"scheduled_at": "2001-01-01T00:00:00Z"}); r.status != 400 {
		t.Fatalf("past schedule accepted: %d", r.status)
	}
	w.c.must("POST", "/api/v1/posts/"+id+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))}, 200)
	upd := w.c.must("PATCH", "/api/v1/posts/"+id, map[string]any{"content": "edited", "scheduled_at": fmtTime(time.Now().Add(2 * time.Hour))}, 200)
	if upd["content"] != "edited" || upd["targets"].([]any)[0].(map[string]any)["content"] != "edited" {
		t.Fatalf("patch: %v", upd)
	}
	var active int
	_ = w.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_jobs j JOIN post_targets t ON t.id = j.post_target_id
		WHERE t.post_id = $1 AND j.status IN ('pending','enqueued')`, id).Scan(&active)
	if active != 1 {
		t.Fatalf("expected exactly one active job after reschedule, got %d", active)
	}
	w.c.must("POST", "/api/v1/posts/"+id+"/unschedule", nil, 200)
	w.c.must("POST", "/api/v1/posts/"+id+"/cancel", nil, 200)
	if r := w.c.do("PATCH", "/api/v1/posts/"+id, map[string]any{"content": "x"}); r.status != 409 {
		t.Fatalf("edit cancelled: %d", r.status)
	}
	w.c.must("DELETE", "/api/v1/posts/"+id, nil, 204)
	if r := w.c.do("GET", "/api/v1/posts/"+id, nil); r.status != 404 {
		t.Fatalf("deleted post visible: %d", r.status)
	}
}
