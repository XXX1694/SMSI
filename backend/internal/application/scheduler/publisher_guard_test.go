package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/post"
)

func TestEarlyTaskGuard(t *testing.T) {
	t.Run("slightly early sleeps on the injected clock then publishes at run_at", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.RunAt = schedulertest.Epoch.Add(1500 * time.Millisecond) })
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		if len(w.Clock.Sleeps) != 1 || w.Clock.Sleeps[0] != 1500*time.Millisecond {
			t.Fatalf("sleeps = %v", w.Clock.Sleeps)
		}
		if tg := w.Target(); tg.Status != post.TargetPublished || tg.PublishedAt.Before(w.Store.Jobs[w.JobID].RunAt) {
			t.Fatalf("published at %v before run_at", tg.PublishedAt)
		}
	})
	t.Run("far too early is handed back to the queue", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.RunAt = schedulertest.Epoch.Add(time.Hour) })
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		if len(w.Queue.Reenqueued) != 1 || providerCalls(w) != 0 || len(w.Clock.Sleeps) != 0 {
			t.Fatalf("reenqueued=%d calls=%d sleeps=%v", len(w.Queue.Reenqueued), providerCalls(w), w.Clock.Sleeps)
		}
		if j := w.Store.Jobs[w.JobID]; j.AsynqTaskID != "retask-1" || j.Status != post.JobEnqueued {
			t.Fatalf("job = %+v", j)
		}
		if w.Target().Status != post.TargetPending {
			t.Fatal("target must stay pending")
		}
	})
	t.Run("reenqueue failure is retryable", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.RunAt = schedulertest.Epoch.Add(time.Hour) })
		w.Store.Errors["Queue.Reenqueue"] = errInfra
		wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	})
	t.Run("MarkEnqueued failure is retryable", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.RunAt = schedulertest.Epoch.Add(time.Hour) })
		w.Store.Errors["Jobs.MarkEnqueued"] = errInfra
		wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	})
	t.Run("no queue configured is retryable", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.RunAt = schedulertest.Epoch.Add(time.Hour) })
		d := w.Deps()
		d.Queue = nil
		wantRetry(t, scheduler.NewPublisher(d).Run(context.Background(), w.Payload(), scheduler.RetryInfo{}))
	})
	t.Run("job lookup failure is retryable", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.Store.Errors["Jobs.Get"] = errInfra
		wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	})
	t.Run("cancelled job during guard publishes nothing", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.Status = post.JobCancelled; j.RunAt = schedulertest.Epoch.Add(time.Hour) })
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		if providerCalls(w) != 0 || len(w.Queue.Reenqueued) != 0 {
			t.Fatal("cancelled job must be a no-op")
		}
	})
}

func TestDefaultSleepAndLoggerAreUsedWhenNotInjected(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	d := w.Deps()
	d.Sleep, d.Log = nil, nil
	w.SetJob(func(j *post.Job) { j.RunAt = schedulertest.Epoch.Add(5 * time.Millisecond) })
	// the fake clock does not advance on a real sleep, so cancel to prove sleepCtx honours ctx.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wantRetry(t, scheduler.NewPublisher(d).Run(ctx, w.Payload(), scheduler.RetryInfo{}))
}
