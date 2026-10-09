package queue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/socialos/backend/internal/testutil"
)

type stuckExports struct {
	started chan struct{}
	release chan struct{}
}

func (s stuckExports) Build(context.Context, uuid.UUID) error {
	s.started <- struct{}{}
	<-s.release // ignores ctx, like a build that is deep in an upload
	return nil
}

// Both servers have in-flight tasks that never finish. Shutdown must wait for them side by side (the larger timeout),
// not one after the other, or the worker overruns the compose stop_grace_period.
func TestShutdownWaitsForBothServersConcurrently(t *testing.T) {
	opt, err := asynq.ParseRedisURI(testutil.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	const timeout = 1500 * time.Millisecond
	q := "shutdown-test-" + uuid.NewString()
	ex := stuckExports{started: make(chan struct{}, 1), release: make(chan struct{})}
	srv := NewServer(opt, ServerConfig{Queue: q, ShutdownTimeout: timeout, ExportShutdownTimeout: timeout, Exports: exportBuilder(ex.Build)}, nil, testutil.Logger())
	pubStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	defer close(ex.release)
	srv.mux = asynq.NewServeMux()
	srv.mux.HandleFunc(TypePublishTarget, func(context.Context, *asynq.Task) error {
		pubStarted <- struct{}{}
		<-release
		return nil
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	c := asynq.NewClient(opt)
	defer func() { _ = c.Close() }()
	if _, err := c.Enqueue(asynq.NewTask(TypePublishTarget, []byte("{}")), asynq.Queue(q), asynq.MaxRetry(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Enqueue(asynq.NewTask(TypeAccountExport, []byte(`{"export_id":"`+uuid.NewString()+`"}`)), asynq.Queue(ExportQueueName(q)), asynq.MaxRetry(0)); err != nil {
		t.Fatal(err)
	}
	for name, ch := range map[string]chan struct{}{"publish": pubStarted, "export": ex.started} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s handler never started", name)
		}
	}
	begin := time.Now()
	srv.Shutdown()
	got := time.Since(begin)
	if got < timeout-200*time.Millisecond {
		t.Fatalf("Shutdown returned after %v, before the in-flight tasks' %v budget", got, timeout)
	}
	if got > timeout+800*time.Millisecond { // sequential would be 2*timeout = 3s
		t.Fatalf("Shutdown took %v, want about %v: the two servers must drain together", got, timeout)
	}
}

type exportBuilder func(context.Context, uuid.UUID) error

func (f exportBuilder) Build(ctx context.Context, id uuid.UUID) error { return f(ctx, id) }
