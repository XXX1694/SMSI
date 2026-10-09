package posts

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
)

// CreateInput is the POST /posts payload.
type CreateInput struct {
	Title            string
	Content          string
	SocialAccountIDs []uuid.UUID
	MediaIDs         []uuid.UUID
	Targets          []TargetInput
	ScheduledAt      *time.Time
	Schedule         bool
}

// Create stores a draft, or a scheduled post when Schedule && ScheduledAt.
func (s *Service) Create(ctx context.Context, a actor.Actor, in CreateInput) (*post.Post, error) {
	if err := a.Require(apikey.PostsWrite); err != nil {
		return nil, err
	}
	schedule := in.Schedule && in.ScheduledAt != nil
	if schedule {
		if err := a.Require(apikey.PostsSchedule); err != nil {
			return nil, err
		}
		if err := a.RequireVerified(); err != nil {
			return nil, err
		}
		if err := s.validateScheduleTime(*in.ScheduledAt); err != nil {
			return nil, err
		}
	}
	accountIDs, overrides := mergeAccounts(in.SocialAccountIDs, in.Targets)
	title := strings.TrimSpace(in.Title)
	if err := validateBasics(title, in.Content, len(accountIDs), len(in.MediaIDs)); err != nil {
		return nil, err
	}
	if schedule && len(accountIDs) == 0 {
		return nil, errs.Validationf("at least one social account is required to schedule").WithField("social_account_ids", "required")
	}
	mediaList, err := s.loadMedia(ctx, a.UserID, in.MediaIDs)
	if err != nil {
		return nil, err
	}
	targets, err := s.buildTargets(ctx, a.UserID, accountIDs, overrides, title, in.Content, mediaList, requireActive(schedule))
	if err != nil {
		return nil, err
	}
	by, ref := createdBy(a)
	p := &post.Post{
		ID: uuid.New(), UserID: a.UserID, Title: title, Content: in.Content, Status: post.StatusDraft,
		CreatedBy: by, CreatedByRef: ref, Targets: targets, MediaIDs: mediaIDs(mediaList),
	}
	for i := range p.Targets {
		p.Targets[i].PostID = p.ID
	}
	err = s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		if schedule {
			if err := s.requireSoonCreate(ctx, a, p, mediaList, *in.ScheduledAt); err != nil {
				return nil, err
			}
		}
		if err := s.repo.Create(ctx, p); err != nil {
			return nil, err
		}
		if err := s.audit.Record(ctx, a, audit.ActionPostCreated, "post", p.ID.String(),
			map[string]any{"targets": len(p.Targets), "media": len(p.MediaIDs)}); err != nil {
			return nil, err
		}
		if !schedule {
			return nil, nil
		}
		return s.scheduleLocked(ctx, a, p, *in.ScheduledAt)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, p.ID)
}

// UpdateInput is the PATCH /posts/{id} payload (nil = unchanged).
type UpdateInput struct {
	Title            *string
	Content          *string
	SocialAccountIDs *[]uuid.UUID
	MediaIDs         *[]uuid.UUID
	Targets          *[]TargetInput
	ScheduledAt      *time.Time
}

// Update edits a draft or scheduled post. Scheduled posts are re-queued.
func (s *Service) Update(ctx context.Context, a actor.Actor, id uuid.UUID, in UpdateInput) (*post.Post, error) {
	if err := a.Require(apikey.PostsWrite); err != nil {
		return nil, err
	}
	if in.ScheduledAt != nil {
		if err := a.Require(apikey.PostsSchedule); err != nil {
			return nil, err
		}
	}
	err := s.inTx(ctx, func(ctx context.Context) ([]post.Job, error) {
		p, err := s.repo.GetForUpdate(ctx, a.UserID, id)
		if err != nil {
			return nil, err
		}
		if !p.Status.Editable() {
			return nil, errs.Newf(errs.InvalidStateTransition, "post in status %s cannot be edited", p.Status)
		}
		if p.Status == post.StatusScheduled {
			if err := a.RequireVerified(); err != nil {
				return nil, err
			}
		}
		if in.ScheduledAt != nil && p.Status != post.StatusScheduled {
			return nil, errs.Validationf("use POST /posts/{id}/schedule to schedule a draft").WithField("scheduled_at", "post is not scheduled")
		}
		if in.ScheduledAt != nil {
			if err := s.validateScheduleTime(*in.ScheduledAt); err != nil {
				return nil, err
			}
		}
		if err := s.applyUpdate(ctx, p, in); err != nil {
			return nil, err
		}
		if err := s.requireEditApproval(ctx, a, p, in); err != nil {
			return nil, err
		}
		if err := s.audit.Record(ctx, a, audit.ActionPostUpdated, "post", p.ID.String(), nil); err != nil {
			return nil, err
		}
		if p.Status != post.StatusScheduled {
			return nil, nil
		}
		runAt := *p.ScheduledAt
		if in.ScheduledAt != nil {
			runAt = *in.ScheduledAt
		}
		return s.reschedule(ctx, p, runAt)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

// requireEditApproval asks a key for approval when the edit leaves a scheduled post running within the minimum lead,
// whether or not it moved the time. It runs after the edit so that the approval covers the post as it will be; a 428
// rolls the edit back.
func (s *Service) requireEditApproval(ctx context.Context, a actor.Actor, p *post.Post, in UpdateInput) error {
	if p.Status != post.StatusScheduled || p.ScheduledAt == nil {
		return nil
	}
	runAt := *p.ScheduledAt
	if in.ScheduledAt != nil {
		runAt = *in.ScheduledAt
	}
	return s.requireSoonAfterEdit(ctx, a, p, runAt)
}

func (s *Service) applyUpdate(ctx context.Context, p *post.Post, in UpdateInput) error {
	oldContent := p.Content
	if in.Title != nil {
		p.Title = strings.TrimSpace(*in.Title)
	}
	if in.Content != nil {
		p.Content = *in.Content
	}
	mids := p.MediaIDs
	if in.MediaIDs != nil {
		mids = *in.MediaIDs
	}
	accountIDs, overrides := currentAccounts(p, oldContent)
	if in.SocialAccountIDs != nil || in.Targets != nil {
		var ids []uuid.UUID
		var tin []TargetInput
		if in.SocialAccountIDs != nil {
			ids = *in.SocialAccountIDs
		}
		if in.Targets != nil {
			tin = *in.Targets
		}
		if in.SocialAccountIDs == nil {
			ids = accountIDs
		}
		var newOverrides map[uuid.UUID]string
		accountIDs, newOverrides = mergeAccounts(ids, tin)
		for k, v := range newOverrides {
			overrides[k] = v
		}
	}
	if err := validateBasics(p.Title, p.Content, len(accountIDs), len(mids)); err != nil {
		return err
	}
	mediaList, err := s.loadMedia(ctx, p.UserID, mids)
	if err != nil {
		return err
	}
	built, err := s.buildTargets(ctx, p.UserID, accountIDs, overrides, p.Title, p.Content, mediaList, requireActive(p.Status == post.StatusScheduled))
	if err != nil {
		return err
	}
	if p.Status == post.StatusScheduled && len(built) == 0 {
		return errs.Validationf("a scheduled post needs at least one social account").WithField("social_account_ids", "required")
	}
	p.Targets = keepTargetIDs(p.Targets, built)
	for i := range p.Targets {
		p.Targets[i].PostID = p.ID
	}
	p.MediaIDs = mediaIDs(mediaList)
	if err := s.repo.Update(ctx, p); err != nil {
		return err
	}
	if err := s.repo.ReplaceTargets(ctx, p); err != nil {
		return err
	}
	return s.repo.ReplaceMedia(ctx, p.UserID, p.ID, p.MediaIDs)
}

// currentAccounts returns existing account ids and the targets whose content
// differs from the old base content (i.e. explicit overrides).
func currentAccounts(p *post.Post, oldContent string) ([]uuid.UUID, map[uuid.UUID]string) {
	ids := make([]uuid.UUID, 0, len(p.Targets))
	overrides := map[uuid.UUID]string{}
	for _, t := range p.Targets {
		ids = append(ids, t.SocialAccountID)
		if t.Content != oldContent {
			overrides[t.SocialAccountID] = t.Content
		}
	}
	return ids, overrides
}

// keepTargetIDs reuses existing target ids (and idempotency keys) per account.
func keepTargetIDs(existing, built []post.Target) []post.Target {
	byAccount := map[uuid.UUID]post.Target{}
	for _, t := range existing {
		byAccount[t.SocialAccountID] = t
	}
	for i, t := range built {
		if old, ok := byAccount[t.SocialAccountID]; ok {
			built[i].ID, built[i].IdempotencyKey = old.ID, old.IdempotencyKey
		}
	}
	return built
}
