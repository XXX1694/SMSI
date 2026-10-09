package scheduler

import (
	"context"
	"log/slog"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
)

// Internal analytics counters.
const (
	MetricPublished = "posts_published"
	MetricFailed    = "posts_failed"
)

// relock re-acquires the post then the target lock inside a new tx. The order (post → target) is the one the API
// and begin use everywhere (issue #38), so no path can deadlock with a cancel; the post lock also serialises sibling
// targets settling the same post.
func (p *Publisher) relock(ctx context.Context, r *run) error {
	ps, err := p.posts.GetForUpdate(ctx, r.target.UserID, r.target.PostID)
	if err != nil {
		return err
	}
	if err := p.targets.LockTargetWait(ctx, r.target.UserID, r.target.ID); err != nil {
		return err
	}
	r.post = ps
	return nil
}

func (p *Publisher) finishAttempt(ctx context.Context, r *run, st post.AttemptStatus, f *failure, meta map[string]any) error {
	if r.attempt == nil {
		return nil
	}
	now := p.clock.Now()
	r.attempt.Status, r.attempt.FinishedAt = st, &now
	if f != nil {
		r.attempt.ErrorCode, r.attempt.ErrorMessage = f.code, f.message
	}
	if meta != nil {
		r.attempt.ResponseMetadata = meta
	}
	return p.targets.UpdateAttempt(ctx, r.attempt)
}

// succeed persists a successful publish in one transaction.
func (p *Publisher) succeed(ctx context.Context, r *run, res provider.PublishResult, meta map[string]any) error {
	err := p.tx.InTx(context.WithoutCancel(ctx), func(ctx context.Context) error {
		if err := p.relock(ctx, r); err != nil {
			return err
		}
		return p.markPublishedLocked(ctx, r, res, meta)
	})
	if err != nil {
		// Provider succeeded but we could not record it: the attempt stays
		// `started` and the next run resolves it as unknown (never re-posts blindly).
		p.log.ErrorContext(ctx, "failed to record successful publish", slog.String("target_id", r.target.ID.String()), slog.Any("error", err))
		return &RetryableError{Err: err}
	}
	p.outcome(r, "published")
	return nil
}

func (p *Publisher) markPublishedLocked(ctx context.Context, r *run, res provider.PublishResult, meta map[string]any) error {
	now := p.clock.Now()
	t := r.target
	t.Status, t.ExternalPostID, t.ExternalURL, t.PublishedAt = post.TargetPublished, res.ExternalID, res.URL, &now
	t.ErrorCode, t.ErrorMessage = "", ""
	if err := p.targets.UpdateTarget(ctx, t); err != nil {
		return err
	}
	if err := p.finishAttempt(ctx, r, post.AttemptSucceeded, nil, meta); err != nil {
		return err
	}
	if err := p.jobs.MarkDoneForTarget(ctx, t.ID); err != nil {
		return err
	}
	if err := p.audit.Record(ctx, r.actor, audit.ActionTargetPublished, "post_target", t.ID.String(),
		map[string]any{"post_id": t.PostID.String(), "platform": t.Platform, "external_post_id": res.ExternalID, "url": res.URL}); err != nil {
		return err
	}
	if p.metrics != nil {
		if err := p.metrics.Increment(ctx, t.UserID, t.SocialAccountID, t.ID, MetricPublished, 1); err != nil {
			return err
		}
	}
	return p.settle(ctx, r)
}

// fail marks the target permanently failed.
func (p *Publisher) fail(ctx context.Context, r *run, f failure, cause error) error {
	err := p.tx.InTx(context.WithoutCancel(ctx), func(ctx context.Context) error {
		if err := p.relock(ctx, r); err != nil {
			return err
		}
		if f.accountAuth {
			if err := p.accounts.MarkExpired(ctx, r.actor, r.account, f.message); err != nil {
				return err
			}
		}
		t := r.target
		t.Status, t.ErrorCode, t.ErrorMessage = post.TargetFailed, f.code, f.message
		if err := p.targets.UpdateTarget(ctx, t); err != nil {
			return err
		}
		if err := p.finishAttempt(ctx, r, post.AttemptFailed, &f, nil); err != nil {
			return err
		}
		if err := p.jobs.MarkDoneForTarget(ctx, t.ID); err != nil {
			return err
		}
		if err := p.audit.Record(ctx, r.actor, audit.ActionTargetFailed, "post_target", t.ID.String(),
			map[string]any{"post_id": t.PostID.String(), "platform": t.Platform, "error_code": f.code}); err != nil {
			return err
		}
		if p.metrics != nil {
			if err := p.metrics.Increment(ctx, t.UserID, t.SocialAccountID, t.ID, MetricFailed, 1); err != nil {
				return err
			}
		}
		return p.settle(ctx, r)
	})
	if err != nil {
		return &RetryableError{Err: err}
	}
	p.outcome(r, "failed")
	return nil
}

// retryLater records a failed/unknown attempt and asks the queue to retry. It relocks (post → target) and re-reads the
// target first: a cancel or unschedule that committed during the provider call must win, so the stale in-memory
// target is never written back over it (issue #38). In that case the attempt is still recorded and no retry is asked.
func (p *Publisher) retryLater(ctx context.Context, r *run, f failure, unknown bool, cause error) error {
	st := post.AttemptFailed
	if unknown {
		st = post.AttemptUnknown
	}
	stopped := false
	err := p.tx.InTx(context.WithoutCancel(ctx), func(ctx context.Context) error {
		if err := p.relock(ctx, r); err != nil {
			return err
		}
		current, err := p.currentTargetStatus(ctx, r)
		if err != nil {
			return err
		}
		if stopped = current != post.TargetPublishing; !stopped {
			t := r.target
			t.Status, t.ErrorCode, t.ErrorMessage = post.TargetPending, f.code, f.message
			if err := p.targets.UpdateTarget(ctx, t); err != nil {
				return err
			}
		}
		return p.finishAttempt(ctx, r, st, &f, nil)
	})
	if err != nil {
		return &RetryableError{Err: err}
	}
	if stopped {
		p.outcome(r, "cancelled")
		return nil
	}
	p.outcome(r, "retry")
	return &RetryableError{Err: cause}
}

// currentTargetStatus reads the target's status inside the transaction that holds its locks.
func (p *Publisher) currentTargetStatus(ctx context.Context, r *run) (post.TargetStatus, error) {
	ts, err := p.targets.ListTargets(ctx, r.target.UserID, r.target.PostID)
	if err != nil {
		return "", err
	}
	for _, t := range ts {
		if t.ID == r.target.ID {
			return t.Status, nil
		}
	}
	return "", errs.NotFoundf("post target")
}

// needsReview stops automatic publishing to avoid a possible duplicate.
func (p *Publisher) needsReview(ctx context.Context, r *run, f *failure) error {
	err := p.tx.InTx(context.WithoutCancel(ctx), func(ctx context.Context) error {
		if err := p.relock(ctx, r); err != nil {
			return err
		}
		return p.needsReviewLocked(ctx, r, f)
	})
	if err != nil {
		return &RetryableError{Err: err}
	}
	p.outcome(r, "needs_review")
	return nil
}

func (p *Publisher) needsReviewLocked(ctx context.Context, r *run, f *failure) error {
	if f == nil {
		f = &failure{code: "OUTCOME_UNKNOWN", message: "a previous attempt may have published this post; verify on the network, then retry or dismiss"}
	}
	t := r.target
	t.Status, t.ErrorCode, t.ErrorMessage = post.TargetNeedsReview, f.code, f.message
	if err := p.targets.UpdateTarget(ctx, t); err != nil {
		return err
	}
	if err := p.finishAttempt(ctx, r, post.AttemptUnknown, f, nil); err != nil {
		return err
	}
	if err := p.jobs.MarkDoneForTarget(ctx, t.ID); err != nil {
		return err
	}
	if err := p.audit.Record(ctx, r.actor, audit.ActionTargetReview, "post_target", t.ID.String(),
		map[string]any{"post_id": t.PostID.String(), "platform": t.Platform}); err != nil {
		return err
	}
	return p.settle(ctx, r)
}

// settle derives the post status once every target is terminal.
func (p *Publisher) settle(ctx context.Context, r *run) error {
	ps := r.post
	targets, err := p.targets.ListTargets(ctx, ps.UserID, ps.ID)
	if err != nil {
		return err
	}
	statuses := make([]post.TargetStatus, len(targets))
	var anyPublished bool
	for i, t := range targets {
		statuses[i] = t.Status
		anyPublished = anyPublished || t.Status == post.TargetPublished
	}
	derived, ok := post.DeriveStatus(statuses)
	if !ok {
		return nil
	}
	if ps.Status == post.StatusScheduled {
		ps.Status = post.StatusPublishing
	}
	if ps.Status != post.StatusPublishing || post.Transition(ps.Status, derived) != nil {
		return nil
	}
	ps.Status = derived
	if anyPublished && ps.PublishedAt == nil {
		now := p.clock.Now()
		ps.PublishedAt = &now
	}
	if err := p.posts.Update(ctx, ps); err != nil {
		return err
	}
	return p.audit.Record(ctx, r.actor, audit.ActionPostCompleted, "post", ps.ID.String(), map[string]any{"status": string(derived)})
}
