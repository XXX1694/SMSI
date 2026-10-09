package posts

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
)

// PublishNow publishes a draft or scheduled post immediately (via the queue).
func (s *Service) PublishNow(ctx context.Context, a actor.Actor, id uuid.UUID) (*post.Post, error) {
	if err := a.Require(apikey.PostsPublish); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
		return nil, err
	}
	if err := a.RequireNotDeleting(); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		if p.Status.Retryable() {
			return nil, errs.Newf(errs.InvalidStateTransition, "post is %s; use retry", p.Status)
		}
		if err := s.requireQuota(ctx, p); err != nil {
			return nil, err
		}
		if err := s.requirePublish(ctx, a, p); err != nil {
			return nil, err
		}
		if err := post.Transition(p.Status, post.StatusPublishing); err != nil {
			return nil, err
		}
		return s.startPublishing(ctx, a, p, audit.ActionPostPublishReq)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

func (s *Service) startPublishing(ctx context.Context, a actor.Actor, p *post.Post, action string) ([]post.Job, error) {
	if err := s.revalidate(ctx, p); err != nil {
		return nil, err
	}
	if err := s.countQuota(ctx, p); err != nil {
		return nil, err
	}
	old, err := s.cancelJobs(ctx, p)
	if err != nil {
		return nil, err
	}
	s.dequeueAfter(ctx, old)
	p.Status = post.StatusPublishing
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	jobs, err := s.createJobs(ctx, p, s.clock.Now())
	if err != nil {
		return nil, err
	}
	return jobs, s.audit.Record(ctx, a, action, "post", p.ID.String(), map[string]any{"targets": len(jobs)})
}

// RetryInput selects how failed targets are retried.
type RetryInput struct {
	ScheduledAt        *time.Time
	IncludeNeedsReview bool
}

// Retry re-queues failed (and optionally needs_review) targets now or at ScheduledAt.
func (s *Service) Retry(ctx context.Context, a actor.Actor, id uuid.UUID, in RetryInput) (*post.Post, error) {
	scope := apikey.PostsPublish
	if in.ScheduledAt != nil {
		scope = apikey.PostsSchedule
		if err := s.validateScheduleTime(*in.ScheduledAt); err != nil {
			return nil, err
		}
	}
	if err := a.Require(scope); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
		return nil, err
	}
	if err := a.RequireNotDeleting(); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		if !p.Status.Retryable() {
			return nil, errs.Newf(errs.InvalidStateTransition, "post in status %s cannot be retried", p.Status)
		}
		if err := s.requireQuota(ctx, p); err != nil {
			return nil, err
		}
		if err := s.requireRetry(ctx, a, p, in); err != nil {
			return nil, err
		}
		if n, err := s.resetForRetry(ctx, p, in.IncludeNeedsReview); err != nil || n == 0 {
			if err == nil {
				err = errs.Validationf("no failed targets to retry")
			}
			return nil, err
		}
		if in.ScheduledAt != nil {
			jobs, err := s.scheduleLocked(ctx, a, p, *in.ScheduledAt)
			if err == nil {
				err = s.audit.Record(ctx, a, audit.ActionPostRetried, "post", p.ID.String(), nil)
			}
			return jobs, err
		}
		return s.startPublishing(ctx, a, p, audit.ActionPostRetried)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

// requireRetry: retrying now is publishing now; retrying for a time too close is the same as scheduling it.
func (s *Service) requireRetry(ctx context.Context, a actor.Actor, p *post.Post, in RetryInput) error {
	if in.ScheduledAt != nil {
		return s.requireSoon(ctx, a, p, *in.ScheduledAt, strconv.FormatBool(in.IncludeNeedsReview))
	}
	return s.requireRetryNow(ctx, a, p, in)
}

func (s *Service) resetForRetry(ctx context.Context, p *post.Post, includeReview bool) (int, error) {
	n := 0
	for i := range p.Targets {
		t := &p.Targets[i]
		if t.Status == post.TargetFailed || (includeReview && t.Status == post.TargetNeedsReview) {
			t.Status, t.ErrorCode, t.ErrorMessage = post.TargetPending, "", ""
			if err := s.repo.UpdateTarget(ctx, t); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, nil
}

// Delete soft-deletes a post (local only; published content stays on the network).
func (s *Service) Delete(ctx context.Context, a actor.Actor, id uuid.UUID) error {
	if err := a.Require(apikey.PostsDelete); err != nil {
		return err
	}
	return s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		if !p.Status.Deletable() {
			return nil, errs.Newf(errs.InvalidStateTransition, "post in status %s cannot be deleted", p.Status)
		}
		if err := s.requireDelete(ctx, a, p); err != nil {
			return nil, err
		}
		jobs, err := s.cancelJobs(ctx, p)
		if err != nil {
			return nil, err
		}
		s.dequeueAfter(ctx, jobs)
		if err := s.setTargets(ctx, p, post.TargetCancelled, post.TargetPending); err != nil {
			return nil, err
		}
		if err := s.repo.SoftDelete(ctx, a.UserID, id, s.clock.Now()); err != nil {
			return nil, err
		}
		return nil, s.audit.Record(ctx, a, audit.ActionPostDeleted, "post", id.String(), map[string]any{"status": string(p.Status)})
	})
}
