package posts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
)

// revalidate checks every pending target is still publishable (accounts active, content valid).
func (s *Service) revalidate(ctx context.Context, p *post.Post) error {
	if len(p.Targets) == 0 {
		return errs.Validationf("post has no social accounts").WithField("social_account_ids", "required")
	}
	var mediaList []media.Media
	if len(p.MediaIDs) > 0 {
		var err error
		if mediaList, err = s.loadMedia(ctx, p.UserID, p.MediaIDs); err != nil {
			return err
		}
	}
	for _, t := range p.Targets {
		if t.Status != post.TargetPending {
			continue
		}
		acc, err := s.accounts.Get(ctx, p.UserID, t.SocialAccountID)
		if err != nil {
			return err
		}
		if err := s.checkTarget(acc, t.Content, mediaList, true); err != nil {
			return err
		}
	}
	return nil
}

// scheduleLocked moves a locked post to scheduled and creates jobs.
func (s *Service) scheduleLocked(ctx context.Context, a actor.Actor, p *post.Post, at time.Time) ([]post.Job, error) {
	if err := post.Transition(p.Status, post.StatusScheduled); err != nil {
		return nil, err
	}
	if err := s.revalidate(ctx, p); err != nil {
		return nil, err
	}
	p.Status, p.ScheduledAt = post.StatusScheduled, &at
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	jobs, err := s.createJobs(ctx, p, at)
	if err != nil {
		return nil, err
	}
	return jobs, s.audit.Record(ctx, a, audit.ActionPostScheduled, "post", p.ID.String(),
		map[string]any{"scheduled_at": at.UTC().Format(time.RFC3339)})
}

// Schedule schedules a draft for publication at `at`.
func (s *Service) Schedule(ctx context.Context, a actor.Actor, id uuid.UUID, at time.Time) (*post.Post, error) {
	if err := a.Require(apikey.PostsSchedule); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
		return nil, err
	}
	if err := s.validateScheduleTime(at); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		return s.scheduleLocked(ctx, a, p, at)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

// Unschedule returns a scheduled post to draft.
func (s *Service) Unschedule(ctx context.Context, a actor.Actor, id uuid.UUID) (*post.Post, error) {
	if err := a.Require(apikey.PostsSchedule); err != nil {
		return nil, err
	}
	return s.stopPost(ctx, a, id, post.StatusDraft, post.TargetPending, audit.ActionPostUnscheduled)
}

// Cancel cancels a draft or scheduled post.
func (s *Service) Cancel(ctx context.Context, a actor.Actor, id uuid.UUID) (*post.Post, error) {
	if err := a.Require(apikey.PostsWrite); err != nil {
		return nil, err
	}
	return s.stopPost(ctx, a, id, post.StatusCancelled, post.TargetCancelled, audit.ActionPostCancelled)
}

func (s *Service) stopPost(ctx context.Context, a actor.Actor, id uuid.UUID, to post.Status, targetTo post.TargetStatus, action string) (*post.Post, error) {
	err := s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		if err := post.Transition(p.Status, to); err != nil {
			return nil, err
		}
		jobs, err := s.cancelJobs(ctx, p)
		if err != nil {
			return nil, err
		}
		s.dequeueAfter(ctx, jobs)
		p.Status = to
		if to == post.StatusDraft {
			p.ScheduledAt = nil
		}
		if err := s.repo.Update(ctx, p); err != nil {
			return nil, err
		}
		if err := s.setTargets(ctx, p, targetTo, post.TargetPending); err != nil {
			return nil, err
		}
		return nil, s.audit.Record(ctx, a, action, "post", p.ID.String(), nil)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

func (s *Service) setTargets(ctx context.Context, p *post.Post, to post.TargetStatus, from ...post.TargetStatus) error {
	for i := range p.Targets {
		t := &p.Targets[i]
		for _, f := range from {
			if t.Status == f {
				t.Status = to
				if err := s.repo.UpdateTarget(ctx, t); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// PublishNow publishes a draft or scheduled post immediately (via the queue).
func (s *Service) PublishNow(ctx context.Context, a actor.Actor, id uuid.UUID) (*post.Post, error) {
	if err := a.Require(apikey.PostsPublish); err != nil {
		return nil, err
	}
	if err := a.RequireVerified(); err != nil {
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
	err := s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		if !p.Status.Retryable() {
			return nil, errs.Newf(errs.InvalidStateTransition, "post in status %s cannot be retried", p.Status)
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
