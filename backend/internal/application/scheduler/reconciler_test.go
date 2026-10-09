package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/post"
)

// overdue makes the seeded job overdue for the reconciler.
func overdue(w *schedulertest.World) {
	w.Clock.Advance(scheduler.OverdueGrace + time.Minute)
}

func runOnce(t *testing.T, w *schedulertest.World) scheduler.Report {
	t.Helper()
	rep, err := w.Reconciler().RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestReconcileJobs(t *testing.T) {
	tests := []struct {
		name      string
		taskID    string
		state     scheduler.TaskState
		status    post.JobStatus
		want      scheduler.Report
		enqueued  int
		reenqueue int
	}{
		{name: "never enqueued job is enqueued", taskID: "", status: post.JobPending,
			want: scheduler.Report{Enqueued: 1}, enqueued: 1},
		{name: "task lost from queue is re-enqueued", taskID: "gone", state: scheduler.TaskMissing, status: post.JobEnqueued,
			want: scheduler.Report{Reenqueued: 1}, enqueued: 1},
		{name: "completed task without settling runs again", taskID: "done", state: scheduler.TaskCompleted, status: post.JobEnqueued,
			want: scheduler.Report{Completed: 1, Reenqueued: 1}, reenqueue: 1},
		{name: "live task is left alone", taskID: "live", state: scheduler.TaskLive, status: post.JobEnqueued},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := schedulertest.NewWorld(nil)
			w.Queue.States[tc.taskID] = tc.state
			w.SetJob(func(j *post.Job) { j.AsynqTaskID, j.Status = tc.taskID, tc.status })
			overdue(w)
			if rep := runOnce(t, w); rep != tc.want {
				t.Fatalf("report = %+v, want %+v", rep, tc.want)
			}
			if len(w.Queue.Enqueued) != tc.enqueued || len(w.Queue.Reenqueued) != tc.reenqueue {
				t.Fatalf("enqueued=%d reenqueued=%d", len(w.Queue.Enqueued), len(w.Queue.Reenqueued))
			}
			if tc.enqueued+tc.reenqueue > 0 && w.Store.Jobs[w.JobID].AsynqTaskID == tc.taskID {
				t.Fatal("new task id not stored on the job")
			}
		})
	}
}

func TestReconcileIgnoresJobsNotYetDue(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.SetJob(func(j *post.Job) { j.AsynqTaskID = "lost"; j.RunAt = schedulertest.Epoch.Add(time.Hour) })
	if rep := runOnce(t, w); rep != (scheduler.Report{}) {
		t.Fatalf("report = %+v", rep)
	}
}

func TestReconcileArchivedTaskSettlesTarget(t *testing.T) {
	archive := func(w *schedulertest.World) {
		w.Queue.States["dead"] = scheduler.TaskArchived
		w.SetJob(func(j *post.Job) { j.AsynqTaskID = "dead" })
		overdue(w)
	}
	t.Run("no attempt in doubt fails with RETRIES_EXHAUSTED", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		archive(w)
		w.AddAttempt(1, post.AttemptFailed, schedulertest.Epoch)
		if rep := runOnce(t, w); rep.Exhausted != 1 {
			t.Fatalf("report = %+v", rep)
		}
		wantTarget(t, w, post.TargetFailed, "RETRIES_EXHAUSTED")
		wantPost(t, w, post.StatusFailed)
		wantJob(t, w, post.JobDone)
	})
	for _, st := range []post.AttemptStatus{post.AttemptStarted, post.AttemptUnknown} {
		t.Run("last attempt "+string(st)+" goes to needs_review", func(t *testing.T) {
			w := schedulertest.NewWorld(nil)
			archive(w)
			w.AddAttempt(1, st, schedulertest.Epoch)
			runOnce(t, w)
			wantTarget(t, w, post.TargetNeedsReview, "OUTCOME_UNKNOWN")
			wantLastAttempt(t, w, post.AttemptUnknown)
			wantJob(t, w, post.JobDone)
		})
	}
	t.Run("already terminal target only closes the job", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		archive(w)
		w.SetTarget(func(tg *post.Target) { tg.Status = post.TargetPublished })
		runOnce(t, w)
		wantTarget(t, w, post.TargetPublished, "")
		wantJob(t, w, post.JobDone)
	})
	t.Run("locked target is skipped", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		archive(w)
		w.Store.Locked[w.TargetID] = true
		runOnce(t, w)
		wantTarget(t, w, post.TargetPending, "")
		wantJob(t, w, post.JobEnqueued)
	})
}

func TestReconcileRecoversStuckTargets(t *testing.T) {
	stuck := func(w *schedulertest.World, attempt post.AttemptStatus) {
		w.SetTarget(func(tg *post.Target) {
			tg.Status, tg.AttemptCount = post.TargetPublishing, 1
			tg.UpdatedAt = schedulertest.Epoch.Add(-scheduler.StuckAfter - time.Minute)
		})
		w.AddAttempt(1, attempt, schedulertest.Epoch.Add(-time.Hour))
	}
	safe := func() provider.Provider {
		p := schedulertest.NewProvider("fake")
		p.Caps.SafeToRetryAfterUnknown = true
		return p
	}
	t.Run("idempotent provider goes back to pending and is re-enqueued", func(t *testing.T) {
		w := schedulertest.NewWorld(safe())
		stuck(w, post.AttemptStarted)
		if rep := runOnce(t, w); rep.Recovered != 1 {
			t.Fatalf("report = %+v", rep)
		}
		wantTarget(t, w, post.TargetPending, "")
		wantLastAttempt(t, w, post.AttemptUnknown)
		if len(w.Queue.Reenqueued) != 1 || w.Store.Jobs[w.JobID].AsynqTaskID == "task-0" {
			t.Fatalf("reenqueued=%d job=%+v", len(w.Queue.Reenqueued), w.Store.Jobs[w.JobID])
		}
	})
	t.Run("lookup provider is recoverable", func(t *testing.T) {
		w := schedulertest.NewWorld(schedulertest.NewLookupProvider("fake"))
		stuck(w, post.AttemptUnknown)
		runOnce(t, w)
		wantTarget(t, w, post.TargetPending, "")
	})
	t.Run("plain provider goes to needs_review", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		stuck(w, post.AttemptStarted)
		runOnce(t, w)
		wantTarget(t, w, post.TargetNeedsReview, "OUTCOME_UNKNOWN")
		if len(w.Queue.Reenqueued) != 0 {
			t.Fatal("must not re-enqueue a target needing review")
		}
	})
	t.Run("job missing is created before enqueue", func(t *testing.T) {
		w := schedulertest.NewWorld(safe())
		stuck(w, post.AttemptStarted)
		w.SetJob(func(j *post.Job) { j.Status = post.JobDone })
		runOnce(t, w)
		if len(w.Queue.Reenqueued) != 1 || len(w.Store.Jobs) != 2 {
			t.Fatalf("reenqueued=%d jobs=%d", len(w.Queue.Reenqueued), len(w.Store.Jobs))
		}
	})
	t.Run("recently updated target is not stuck", func(t *testing.T) {
		w := schedulertest.NewWorld(safe())
		stuck(w, post.AttemptStarted)
		w.SetTarget(func(tg *post.Target) { tg.UpdatedAt = schedulertest.Epoch })
		if rep := runOnce(t, w); rep.Recovered != 0 {
			t.Fatalf("report = %+v", rep)
		}
	})
}

func TestReconcileErrors(t *testing.T) {
	t.Run("listing jobs fails", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.Store.Errors["Jobs.NeedingReconcile"] = errInfra
		if _, err := w.Reconciler().RunOnce(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("listing stuck targets fails", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.Store.Errors["Targets.StuckPublishing"] = errInfra
		if _, err := w.Reconciler().RunOnce(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("one failing job does not stop the pass", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.AsynqTaskID = "x" })
		w.Store.Errors["Queue.TaskState"] = errInfra
		overdue(w)
		if rep := runOnce(t, w); rep != (scheduler.Report{}) {
			t.Fatalf("report = %+v", rep)
		}
	})
	t.Run("enqueue failure leaves the job for the next pass", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetJob(func(j *post.Job) { j.AsynqTaskID, j.Status = "", post.JobPending })
		w.Store.Errors["Queue.Enqueue"] = errInfra
		overdue(w)
		if rep := runOnce(t, w); rep.Enqueued != 0 {
			t.Fatalf("report = %+v", rep)
		}
	})
	t.Run("failing recovery is skipped", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetTarget(func(tg *post.Target) {
			tg.Status, tg.UpdatedAt = post.TargetPublishing, schedulertest.Epoch.Add(-time.Hour)
		})
		w.Store.Errors["Accounts.Get"] = errInfra
		if rep := runOnce(t, w); rep.Recovered != 0 {
			t.Fatalf("report = %+v", rep)
		}
	})
}

func TestReconcilerLoopStopsOnCancel(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.Store.Errors["Jobs.NeedingReconcile"] = errInfra // exercise the error log branch too
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { w.Reconciler().Loop(ctx, time.Hour); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Loop did not stop after cancel")
	}
}
