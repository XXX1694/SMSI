package posts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/quota"
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
		if err := s.checkTarget(acc, p.Title, t.Content, mediaList, true); err != nil {
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
	if err := s.countQuota(ctx, p); err != nil {
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

// needsQuota: a post is counted in the month it is first scheduled or published, and again when it is scheduled or
// published in a later month, so a post cannot bank an allowance by being scheduled at the end of one month.
func (s *Service) needsQuota(p *post.Post) bool {
	return p.QuotaCountedAt == nil || p.QuotaCountedAt.Before(quota.MonthStart(s.clock.Now()))
}

// requireQuota refuses early, before the owner is asked to approve something that would fail anyway.
func (s *Service) requireQuota(ctx context.Context, p *post.Post) error {
	if !s.needsQuota(p) {
		return nil
	}
	return s.quota.EnforcePost(ctx, p.UserID)
}

// countQuota counts the post against the monthly quota; unscheduling and scheduling again within the month, or
// retrying, never counts it twice. The caller's transaction holds the user lock, so the count and
// the update that records it cannot interleave with another request of the same user.
func (s *Service) countQuota(ctx context.Context, p *post.Post) error {
	if !s.needsQuota(p) {
		return nil
	}
	if err := s.quota.EnforcePost(ctx, p.UserID); err != nil {
		return err
	}
	now := s.clock.Now()
	p.QuotaCountedAt = &now
	return nil
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
		if err := s.requireQuota(ctx, p); err != nil {
			return nil, err
		}
		if err := s.requireSoon(ctx, a, p, at); err != nil {
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

// UnscheduleAll takes every scheduled post of the user back to draft and returns how many it stopped. Account deletion
// uses it so nothing is published during the grace period. Posts already being published are left to finish.
func (s *Service) UnscheduleAll(ctx context.Context, userID uuid.UUID) (int, error) {
	a := actor.System(userID, "account_deletion")
	stopped := 0
	for {
		batch, err := s.repo.List(ctx, userID, ListFilter{Status: post.StatusScheduled}, port.Page{Limit: port.MaxLimit})
		if err != nil {
			return stopped, err
		}
		if len(batch) == 0 {
			return stopped, nil
		}
		for _, p := range batch {
			if _, err := s.stopPost(ctx, a, p.ID, post.StatusDraft, post.TargetPending, audit.ActionPostUnscheduled); err != nil {
				return stopped, err
			}
			stopped++
		}
	}
}
