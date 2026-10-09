package posts

import (
	"context"

	"github.com/socialos/backend/internal/application/port"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/post"
)

// createJobs inserts one pending scheduled_jobs row per pending target (in tx).
func (s *Service) createJobs(ctx context.Context, p *post.Post, runAt time.Time) ([]post.Job, error) {
	var jobs []post.Job
	for _, t := range p.Targets {
		if t.Status != post.TargetPending {
			continue
		}
		j := post.Job{ID: uuid.New(), PostTargetID: t.ID, RunAt: runAt, Status: post.JobPending}
		if err := s.jobs.Create(ctx, &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// cancelJobs cancels active jobs in the DB (in tx); queue tasks are deleted after commit.
func (s *Service) cancelJobs(ctx context.Context, p *post.Post) ([]post.Job, error) {
	return s.jobs.CancelActiveForPost(ctx, p.UserID, p.ID)
}

// enqueue pushes committed jobs to the queue. Failures are logged and left
// pending: the reconciler re-enqueues them (DB is the source of truth).
func (s *Service) enqueue(ctx context.Context, jobs []post.Job) {
	ctx = context.WithoutCancel(ctx)
	for _, j := range jobs {
		taskID, err := s.queue.Enqueue(ctx, j)
		if err != nil {
			s.log.WarnContext(ctx, "enqueue failed; reconciler will retry", slog.String("job_id", j.ID.String()), slog.Any("error", err))
			continue
		}
		if err := s.jobs.MarkEnqueued(ctx, j.ID, taskID); err != nil {
			s.log.WarnContext(ctx, "mark enqueued failed", slog.String("job_id", j.ID.String()), slog.Any("error", err))
		}
	}
}

// dequeue removes queue tasks of cancelled jobs (best effort; the worker
// re-checks job status so a surviving task is a no-op).
func (s *Service) dequeue(ctx context.Context, jobs []post.Job) {
	ctx = context.WithoutCancel(ctx)
	for _, j := range jobs {
		if j.AsynqTaskID == "" {
			continue
		}
		if err := s.queue.Delete(ctx, j.AsynqTaskID); err != nil {
			s.log.DebugContext(ctx, "queue task delete failed", slog.String("task_id", j.AsynqTaskID), slog.Any("error", err))
		}
	}
}

// reschedule replaces active jobs of a scheduled post with new ones at runAt.
func (s *Service) reschedule(ctx context.Context, p *post.Post, runAt time.Time) ([]post.Job, error) {
	old, err := s.cancelJobs(ctx, p)
	if err != nil {
		return nil, err
	}
	s.dequeueAfter(ctx, old)
	p.ScheduledAt = &runAt
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	fresh, err := s.repo.Get(ctx, p.UserID, p.ID)
	if err != nil {
		return nil, err
	}
	return s.createJobs(ctx, fresh, runAt)
}

type pendingDequeueKey struct{}

// dequeueAfter records jobs whose tasks must be deleted once the tx commits.
func (s *Service) dequeueAfter(ctx context.Context, jobs []post.Job) {
	if bucket, ok := ctx.Value(pendingDequeueKey{}).(*[]post.Job); ok {
		*bucket = append(*bucket, jobs...)
	}
}

// inTx runs fn in a transaction, then enqueues/dequeues queue tasks after commit.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context) ([]post.Job, error)) error {
	var toDequeue []post.Job
	var toEnqueue []post.Job
	err := s.tx.InTx(context.WithValue(ctx, pendingDequeueKey{}, &toDequeue), func(ctx context.Context) error {
		jobs, err := fn(ctx)
		toEnqueue = jobs
		return err
	})
	if err != nil {
		// The transaction is over: a pending approval can be recorded now (and survives).
		return port.OpenIfNeeded(ctx, s.gate, err)
	}
	s.dequeue(ctx, toDequeue)
	s.enqueue(ctx, toEnqueue)
	return nil
}
