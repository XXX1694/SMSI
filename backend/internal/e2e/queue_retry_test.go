package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
)

// delayRecorder is a RetryDelayFunc that records the retry counter Asynq passes
// in and returns a short delay so the whole retry ladder runs in a second.
type delayRecorder struct {
	mu sync.Mutex
	ns []int
}

func (d *delayRecorder) fn(n int, _ error) time.Duration {
	d.mu.Lock()
	d.ns = append(d.ns, n)
	d.mu.Unlock()
	return 30 * time.Millisecond
}

func (d *delayRecorder) seen() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]int(nil), d.ns...)
}

// TestAsynqRetriesUpToMaxThenFails runs the real worker: a provider that always
// answers 503 is called once plus MaxRetry retries, then the target is marked
// failed (not left pending, not archived by the queue).
func TestAsynqRetriesUpToMaxThenFails(t *testing.T) {
	flaky := newNoLookup("flaky")
	flaky.fail = provider.FromHTTPStatus("flaky", 503, "upstream down", 0)
	rec := &delayRecorder{}
	e := newEnv(t, envOpts{startWorker: true, providers: []provider.Provider{flaky}, retryDelay: rec.fn})
	c := e.browser()
	c.register("retry-max@example.com")
	acc := (&workerEnv{env: e, c: c}).connect("flaky")

	post := c.must("POST", "/api/v1/posts", map[string]any{"content": "never works", "social_account_ids": []string{acc}}, 201)
	id := post["id"].(string)
	c.must("POST", "/api/v1/posts/"+id+"/publish", nil, 202)

	st := c.waitStatus(id, "failed", 30*time.Second)
	tg := st["targets"].([]any)[0].(map[string]any)
	if tg["status"] != "failed" || tg["error_code"] != "HTTP_503" || tg["attempt_count"].(float64) != float64(scheduler.MaxRetry+1) {
		t.Fatalf("unexpected final target: %v", tg)
	}
	if got := flaky.calls.Load(); got != int32(scheduler.MaxRetry+1) {
		t.Fatalf("provider called %d times, want %d (1 + %d retries)", got, scheduler.MaxRetry+1, scheduler.MaxRetry)
	}
	// Asynq asks for a delay before each retry with a growing retry counter.
	want := []int{0, 1, 2, 3, 4}
	got := rec.seen()
	if len(got) != len(want) {
		t.Fatalf("retry delays requested for %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("retry counters %v, want %v", got, want)
		}
	}
	detail := c.must("GET", "/api/v1/posts/"+id, nil, 200)
	attempts := detail["attempts"].([]any)
	if len(attempts) != scheduler.MaxRetry+1 {
		t.Fatalf("expected %d recorded attempts, got %d", scheduler.MaxRetry+1, len(attempts))
	}
	for _, a := range attempts {
		if a.(map[string]any)["status"] != "failed" {
			t.Fatalf("every attempt should be failed: %v", attempts)
		}
	}
	// No job is left behind for the reconciler to resurrect.
	var active int
	_ = e.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_jobs j JOIN post_targets t ON t.id = j.post_target_id
		WHERE t.post_id = $1 AND j.status IN ('pending','enqueued')`, id).Scan(&active)
	if active != 0 {
		t.Fatalf("%d active jobs left after failure", active)
	}
	// The audit log records the failure exactly once.
	if n := auditActions(t, c)["post_target.failed"]; n != 1 {
		t.Fatalf("post_target.failed audited %d times", n)
	}
}

// TestAsynqTransientFailureThenPublished: a 503 on the first attempt is retried
// by the queue and then succeeds exactly once.
func TestAsynqTransientFailureThenPublished(t *testing.T) {
	rec := &delayRecorder{}
	e := newEnv(t, envOpts{startWorker: true, retryDelay: rec.fn})
	c := e.browser()
	c.register("retry-ok@example.com")
	acc := c.connectMock()
	post := c.must("POST", "/api/v1/posts", map[string]any{"content": "flaky #mock-retry", "social_account_ids": []string{acc}}, 201)
	id := post["id"].(string)
	c.must("POST", "/api/v1/posts/"+id+"/publish", nil, 202)

	st := c.waitStatus(id, "published", 20*time.Second)
	tg := st["targets"].([]any)[0].(map[string]any)
	if tg["attempt_count"].(float64) != 2 || tg["error_code"] != nil {
		t.Fatalf("expected 2 attempts and a cleared error: %v", tg)
	}
	attempts := c.must("GET", "/api/v1/posts/"+id, nil, 200)["attempts"].([]any)
	if len(attempts) != 2 || attempts[0].(map[string]any)["status"] != "failed" || attempts[1].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("attempt history: %v", attempts)
	}
	if got := rec.seen(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("retry delay requests: %v", got)
	}
}

// TestAsynqUnknownOutcomeIsLookedUpNotReposted: the mock records the post and
// then "times out". The queue retries; the worker resolves the unknown attempt
// by lookup instead of calling Publish again.
func TestAsynqUnknownOutcomeIsLookedUpNotReposted(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true, retryDelay: (&delayRecorder{}).fn})
	c := e.browser()
	c.register("unknown@example.com")
	acc := c.connectMock()
	post := c.must("POST", "/api/v1/posts", map[string]any{"content": "timeout #mock-unknown", "social_account_ids": []string{acc}}, 201)
	id := post["id"].(string)
	c.must("POST", "/api/v1/posts/"+id+"/publish", nil, 202)
	st := c.waitStatus(id, "published", 20*time.Second)
	tg := st["targets"].([]any)[0].(map[string]any)

	w := &workerEnv{env: e, c: c}
	key := ""
	_ = e.app.DB.Pool.QueryRow(context.Background(), `SELECT idempotency_key FROM post_targets WHERE id = $1`, tg["id"]).Scan(&key)
	if n := mockOf(t, w).Calls(key); n != 1 {
		t.Fatalf("provider Publish called %d times for one post, want 1", n)
	}
}

// TestScheduledJobsSurviveRedisFlush: Postgres is the source of truth. After the
// queue loses every task the reconciler re-enqueues the overdue job and the
// worker publishes it.
func TestScheduledJobsSurviveRedisFlush(t *testing.T) {
	e := newEnv(t, envOpts{startWorker: true})
	c := e.browser()
	c.register("flush@example.com")
	acc := c.connectMock()
	ctx := context.Background()

	post := c.must("POST", "/api/v1/posts", map[string]any{"content": "survive redis", "social_account_ids": []string{acc},
		"scheduled_at": fmtTime(time.Now().Add(1500 * time.Millisecond)), "schedule": true}, 201)
	id := post["id"].(string)

	if err := e.app.Redis.Client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond)
	if st := c.must("GET", "/api/v1/posts/"+id+"/status", nil, 200); st["status"] != "scheduled" {
		t.Fatalf("task should have been lost with Redis, status=%v", st["status"])
	}
	if _, err := e.app.DB.Pool.Exec(ctx, `UPDATE scheduled_jobs SET run_at = now() - interval '5 minutes'
		WHERE post_target_id IN (SELECT id FROM post_targets WHERE post_id = $1) AND status IN ('pending','enqueued')`, id); err != nil {
		t.Fatal(err)
	}
	rep, err := e.app.Reconciler.RunOnce(ctx)
	if err != nil || rep.Enqueued+rep.Reenqueued < 1 {
		t.Fatalf("reconciler did not re-enqueue: %+v %v", rep, err)
	}
	c.waitStatus(id, "published", 20*time.Second)
}
