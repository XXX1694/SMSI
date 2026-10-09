package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/infrastructure/queue"
	"github.com/socialos/backend/internal/testutil"
)

// blockingProvider holds every Publish until release is closed, so a test can stop the worker mid-publish.
type blockingProvider struct {
	*noLookup
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingProvider) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return provider.PublishResult{}, ctx.Err()
	}
	return p.noLookup.Publish(ctx, req)
}

// TestWorkerShutdownFinishesInFlightPublishAndStartsNothingNew is the regression test for issue #39: a SIGTERM during
// a publish must let that publish finish (no needs_review), must not start the next queued task, and the queued task
// must still be there for the next worker.
func TestWorkerShutdownFinishesInFlightPublishAndStartsNothingNew(t *testing.T) {
	slow := &blockingProvider{noLookup: newNoLookup("slow"), started: make(chan struct{}), release: make(chan struct{})}
	e := newEnv(t, envOpts{providers: []provider.Provider{slow}})
	w := &workerEnv{env: e, c: e.browser()}
	w.c.register("shutdown@example.com")
	acc := w.connect("slow")

	newServer := func() *queue.Server {
		// One slot: the second task waits behind the first, so "not started" is observable.
		return queue.NewServer(e.app.Redis.Asynq, queue.ServerConfig{Queue: e.app.Cfg.QueueName, Concurrency: 1,
			ShutdownTimeout: 20 * time.Second, DelayedCheck: 100 * time.Millisecond}, e.app.Publisher, testutil.Logger())
	}
	a, _ := w.publishJob(acc, "in flight when SIGTERM arrives")
	b, _ := w.publishJob(acc, "queued behind it")

	srv := newServer()
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-slow.started:
	case <-time.After(10 * time.Second):
		t.Fatal("worker never started the first publish")
	}

	// Asynq does not promise FIFO: whichever post the worker picked is the in-flight one.
	first, second := a, b
	var inFlight string
	if err := e.app.DB.Pool.QueryRow(context.Background(), `SELECT post_id::text FROM post_targets WHERE status = 'publishing'`).Scan(&inFlight); err != nil {
		t.Fatal(err)
	}
	if inFlight == b {
		first, second = b, a
	}

	stopped := make(chan struct{})
	go func() { srv.Shutdown(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Shutdown returned while a publish was still in flight")
	case <-time.After(500 * time.Millisecond):
	}
	close(slow.release) // the provider answers while the worker is shutting down
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not return after the in-flight publish finished")
	}

	if st, _ := w.status(first); st != "published" {
		t.Fatalf("in-flight post is %q after graceful shutdown, want published", st)
	}
	if st, tg := w.status(second); st != "publishing" || tg["status"] != "pending" || slow.calls.Load() != 1 {
		t.Fatalf("queued post %q/%v, provider calls %d: nothing new may start after shutdown begins", st, tg["status"], slow.calls.Load())
	}
	var review int
	if err := e.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM post_targets WHERE status = 'needs_review'`).Scan(&review); err != nil || review != 0 {
		t.Fatalf("needs_review targets after shutdown: %d (%v)", review, err)
	}

	// The queued task survives for the next worker.
	next := newServer()
	if err := next.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(next.Shutdown)
	w.c.waitStatus(second, "published", 15*time.Second)
}
