package scheduler

import (
	"context"
	"time"

	"github.com/socialos/backend/internal/domain/errs"
)

// EarlyWaitMax is the longest the worker sleeps for a task that fired before
// its job's run_at; a larger gap is re-enqueued instead of holding a worker.
const EarlyWaitMax = 2 * time.Second

// holdUntilDue enforces "never publish before scheduled_at". The job row (not
// the payload) is the authority for run_at. It returns proceed=true once
// now >= run_at (or when the job is gone/finished, which begin treats as a
// no-op), and proceed=false after handing a far-too-early task back to the queue.
func (p *Publisher) holdUntilDue(ctx context.Context, pl Payload) (bool, error) {
	for {
		job, err := p.jobs.Get(ctx, pl.JobID)
		if errs.Is(err, errs.NotFound) || (err == nil && !job.Status.Active()) {
			return true, nil
		}
		if err != nil {
			return false, &RetryableError{Err: err}
		}
		wait := job.RunAt.Sub(p.clock.Now())
		if wait <= 0 {
			return true, nil
		}
		if wait <= EarlyWaitMax {
			if err := p.sleep(ctx, wait); err != nil {
				return false, &RetryableError{Err: err}
			}
			continue // re-read: the job may have been rescheduled meanwhile
		}
		if p.queue == nil {
			return false, &RetryableError{Err: errTooEarly}
		}
		id, err := p.queue.Reenqueue(ctx, *job)
		if err != nil {
			return false, &RetryableError{Err: err}
		}
		if err := p.jobs.MarkEnqueued(ctx, job.ID, id); err != nil {
			return false, &RetryableError{Err: err}
		}
		return false, nil
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
