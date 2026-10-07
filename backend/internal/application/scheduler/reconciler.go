package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/post"
)

// Reconciler tunables.
const (
	ReconcileInterval = time.Minute
	OverdueGrace      = time.Minute
	StuckAfter        = 15 * time.Minute
	reconcileBatch    = 200
)

// Reconciler keeps the queue consistent with scheduled_jobs (DB is truth).
type Reconciler struct {
	pub   *Publisher
	queue Queue
	log   *slog.Logger
}

// NewReconciler creates a reconciler.
func NewReconciler(pub *Publisher, q Queue) *Reconciler {
	return &Reconciler{pub: pub, queue: q, log: pub.log}
}

// Report summarises one reconciliation pass.
type Report struct {
	Enqueued, Reenqueued, Exhausted, Completed, Recovered int
}

// Loop runs RunOnce every interval until ctx is cancelled.
func (r *Reconciler) Loop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if rep, err := r.RunOnce(ctx); err != nil {
			r.log.ErrorContext(ctx, "reconcile failed", slog.Any("error", err))
		} else if rep != (Report{}) {
			r.log.InfoContext(ctx, "reconciled", slog.Any("report", rep))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce performs one pass: re-enqueue missing tasks, settle exhausted ones,
// recover targets stuck in publishing.
func (r *Reconciler) RunOnce(ctx context.Context) (Report, error) {
	var rep Report
	now := r.pub.clock.Now()
	jobs, err := r.pub.jobs.NeedingReconcile(ctx, now.Add(-OverdueGrace), reconcileBatch)
	if err != nil {
		return rep, err
	}
	for _, j := range jobs {
		if err := r.reconcileJob(ctx, j, &rep); err != nil {
			r.log.WarnContext(ctx, "reconcile job failed", slog.String("job_id", j.ID.String()), slog.Any("error", err))
		}
	}
	stuck, err := r.pub.targets.StuckPublishing(ctx, now.Add(-StuckAfter), reconcileBatch)
	if err != nil {
		return rep, err
	}
	for _, t := range stuck {
		if err := r.recoverStuck(ctx, t.ID); err != nil {
			r.log.WarnContext(ctx, "recover stuck target failed", slog.String("target_id", t.ID.String()), slog.Any("error", err))
			continue
		}
		rep.Recovered++
	}
	return rep, nil
}

func (r *Reconciler) reconcileJob(ctx context.Context, j post.Job, rep *Report) error {
	if j.AsynqTaskID == "" {
		return r.enqueue(ctx, j, false, &rep.Enqueued)
	}
	st, err := r.queue.TaskState(ctx, j.AsynqTaskID)
	if err != nil {
		return err
	}
	switch st {
	case TaskMissing:
		return r.enqueue(ctx, j, false, &rep.Reenqueued) // e.g. Redis was flushed
	case TaskCompleted:
		rep.Completed++
		return r.enqueue(ctx, j, true, &rep.Reenqueued) // handler ended without settling: run again
	case TaskArchived:
		rep.Exhausted++
		return r.exhaust(ctx, j)
	}
	return nil
}

func (r *Reconciler) enqueue(ctx context.Context, j post.Job, fresh bool, counter *int) error {
	var id string
	var err error
	if fresh {
		id, err = r.queue.Reenqueue(ctx, j)
	} else {
		id, err = r.queue.Enqueue(ctx, j)
	}
	if err != nil {
		return err
	}
	*counter++
	return r.pub.jobs.MarkEnqueued(ctx, j.ID, id)
}

// exhaust settles a target whose queue task ran out of retries without
// finishing. A target whose last attempt may have reached the provider
// (started/unknown) goes to needs_review rather than failed, so it is never
// silently re-posted or hidden; otherwise it fails with RETRIES_EXHAUSTED.
func (r *Reconciler) exhaust(ctx context.Context, j post.Job) error {
	return r.pub.tx.InTx(ctx, func(ctx context.Context) error {
		run, err := r.lockedRun(ctx, j.PostTargetID)
		if err != nil || run == nil {
			return err
		}
		if run.target.Status.Terminal() {
			return r.pub.jobs.MarkDoneForTarget(ctx, run.target.ID)
		}
		latest, err := r.pub.targets.LatestAttempt(ctx, run.target.ID)
		if err != nil {
			return err
		}
		if latest != nil && (latest.Status == post.AttemptStarted || latest.Status == post.AttemptUnknown) {
			run.attempt = latest
			f := &failure{code: "OUTCOME_UNKNOWN", message: "publishing stopped after retries; a previous attempt may have published this post"}
			return r.pub.needsReviewLocked(ctx, run, f)
		}
		f := failure{code: "RETRIES_EXHAUSTED", message: "publishing failed after all retries"}
		t := run.target
		t.Status, t.ErrorCode, t.ErrorMessage = post.TargetFailed, f.code, f.message
		if err := r.pub.targets.UpdateTarget(ctx, t); err != nil {
			return err
		}
		if err := r.pub.jobs.MarkDoneForTarget(ctx, t.ID); err != nil {
			return err
		}
		return r.pub.settle(ctx, run)
	})
}

// recoverStuck handles a target left in `publishing` by a crashed worker.
func (r *Reconciler) recoverStuck(ctx context.Context, targetID uuid.UUID) error {
	var toEnqueue *post.Job
	err := r.pub.tx.InTx(ctx, func(ctx context.Context) error {
		run, err := r.lockedRun(ctx, targetID)
		if err != nil || run == nil || run.target.Status != post.TargetPublishing {
			return err
		}
		latest, err := r.pub.targets.LatestAttempt(ctx, targetID)
		if err != nil {
			return err
		}
		if latest != nil && latest.Status == post.AttemptStarted {
			run.attempt = latest
			if err := r.pub.finishAttempt(ctx, run, post.AttemptUnknown, &failure{code: "OUTCOME_UNKNOWN", message: "worker stopped mid-publish"}, nil); err != nil {
				return err
			}
		}
		acc, err := r.pub.accounts.Get(ctx, run.target.UserID, run.target.SocialAccountID)
		if err != nil {
			return err
		}
		run.account = acc
		proceed, err := r.pub.decideUnknown(ctx, run)
		if err != nil || !proceed {
			return err
		}
		run.target.Status = post.TargetPending
		if err := r.pub.targets.UpdateTarget(ctx, run.target); err != nil {
			return err
		}
		toEnqueue, err = r.ensureJob(ctx, targetID)
		return err
	})
	if err != nil || toEnqueue == nil {
		return err
	}
	id, err := r.queue.Reenqueue(ctx, *toEnqueue)
	if err != nil {
		return err
	}
	return r.pub.jobs.MarkEnqueued(ctx, toEnqueue.ID, id)
}

func (r *Reconciler) ensureJob(ctx context.Context, targetID uuid.UUID) (*post.Job, error) {
	j, err := r.pub.jobs.ActiveForTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if j != nil {
		return j, nil
	}
	j = &post.Job{ID: uuid.New(), PostTargetID: targetID, RunAt: r.pub.clock.Now(), Status: post.JobPending}
	return j, r.pub.jobs.Create(ctx, j)
}

// lockedRun locks the target and its post; nil when locked elsewhere or gone.
func (r *Reconciler) lockedRun(ctx context.Context, targetID uuid.UUID) (*run, error) {
	t, ok, err := r.pub.targets.LockTarget(ctx, targetID)
	if err != nil || !ok {
		return nil, err
	}
	ps, err := r.pub.posts.GetForUpdate(ctx, t.UserID, t.PostID)
	if err != nil {
		return nil, err
	}
	return &run{target: t, post: ps, actor: actor.Scheduler(t.UserID)}, nil
}
