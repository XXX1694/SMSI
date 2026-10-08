package posts

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Limits on post input.
const (
	MaxTitleLen   = 200
	MaxContentLen = 10000
	MaxTargets    = 20
	MaxMedia      = 20
	MaxScheduleIn = 366 * 24 * time.Hour
)

func errInvalidStatus(s post.Status) error {
	return errs.Validationf("invalid status %q", s).WithField("status", "invalid")
}

// TargetInput is a per-account content override.
type TargetInput struct {
	SocialAccountID uuid.UUID
	Content         *string
}

// mergeAccounts unions account ids (ordered, de-duplicated) and collects overrides.
func mergeAccounts(ids []uuid.UUID, targets []TargetInput) ([]uuid.UUID, map[uuid.UUID]string) {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	overrides := map[uuid.UUID]string{}
	add := func(id uuid.UUID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range ids {
		add(id)
	}
	for _, t := range targets {
		add(t.SocialAccountID)
		if t.Content != nil {
			overrides[t.SocialAccountID] = *t.Content
		}
	}
	return out, overrides
}

func validateBasics(title, content string, accounts, mediaIDs int) error {
	switch {
	case utf8.RuneCountInString(title) > MaxTitleLen:
		return errs.Validationf("title too long").WithField("title", fmt.Sprintf("max %d characters", MaxTitleLen))
	case utf8.RuneCountInString(content) > MaxContentLen:
		return errs.Validationf("content too long").WithField("content", fmt.Sprintf("max %d characters", MaxContentLen))
	case accounts > MaxTargets:
		return errs.Validationf("too many social accounts").WithField("social_account_ids", fmt.Sprintf("max %d", MaxTargets))
	case mediaIDs > MaxMedia:
		return errs.Validationf("too many media").WithField("media_ids", fmt.Sprintf("max %d", MaxMedia))
	}
	return nil
}

// loadMedia resolves media ids for the tenant, preserving order.
func (s *Service) loadMedia(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]media.Media, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	found, err := s.media.GetMany(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]media.Media{}
	for _, m := range found {
		byID[m.ID] = m
	}
	out := make([]media.Media, 0, len(ids))
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		m, ok := byID[id]
		if !ok {
			return nil, errs.Validationf("media %s not found", id).WithField("media_ids", "not found")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, m)
	}
	return out, nil
}

// requireActive selects how strictly account status is checked.
type requireActive bool

// buildTargets resolves accounts and validates content against provider capabilities.
func (s *Service) buildTargets(ctx context.Context, userID uuid.UUID, accountIDs []uuid.UUID, overrides map[uuid.UUID]string,
	title, content string, mediaList []media.Media, strict requireActive) ([]post.Target, error) {
	targets := make([]post.Target, 0, len(accountIDs))
	for _, id := range accountIDs {
		acc, err := s.accounts.Get(ctx, userID, id)
		if errs.Is(err, errs.NotFound) {
			return nil, errs.Validationf("social account %s not found", id).WithField("social_account_ids", "not found")
		}
		if err != nil {
			return nil, err
		}
		text := content
		if o, ok := overrides[id]; ok {
			text = o
		}
		if err := s.checkTarget(acc, title, text, mediaList, strict); err != nil {
			return nil, err
		}
		tid := uuid.New()
		targets = append(targets, post.Target{
			ID: tid, UserID: userID, SocialAccountID: acc.ID, Platform: acc.Provider, Content: text,
			Status: post.TargetPending, IdempotencyKey: tid.String(),
		})
	}
	return targets, nil
}

func (s *Service) checkTarget(acc *socialaccount.Account, title, text string, mediaList []media.Media, strict requireActive) error {
	switch {
	case acc.Status == socialaccount.StatusRevoked:
		return errs.Validationf("social account %s is disconnected", acc.ID).WithField("social_account_ids", "disconnected")
	case bool(strict) && !acc.Publishable():
		return errs.Newf(errs.SocialAccountExpired, "%s account @%s needs to be reconnected", acc.Provider, acc.Username)
	}
	p, _, err := s.registry.Publisher(acc.Provider)
	if err != nil {
		return err
	}
	return CheckContent(p.DisplayName(), p.Capabilities().WithAccountLimits(acc.Metadata), title, text, mediaList)
}

// CheckContent validates title, text and media against provider capabilities.
// Callers pass capabilities already narrowed by the account's own limits
// (provider.Capabilities.WithAccountLimits).
func CheckContent(name string, c provider.Capabilities, title, text string, mediaList []media.Media) error {
	n := utf8.RuneCountInString(text)
	if c.RequiresTitle && strings.TrimSpace(title) == "" {
		return errs.Validationf("%s posts need a title", name).WithField("title", "required")
	}
	if strings.TrimSpace(text) == "" && len(mediaList) == 0 {
		return errs.Validationf("%s post needs content or media", name).WithField("content", "required")
	}
	if len(mediaList) == 0 && !c.CanPublishText {
		return errs.Validationf("%s requires media", name).WithField("media_ids", "required")
	}
	if c.MaxTextLength > 0 && n > c.MaxTextLength {
		return errs.Validationf("%s content exceeds %d characters", name, c.MaxTextLength).WithField("content", "too long")
	}
	if len(mediaList) > 0 && c.MaxCaptionLength > 0 && n > c.MaxCaptionLength {
		return errs.Validationf("%s captions with media are limited to %d characters", name, c.MaxCaptionLength).WithField("content", "too long")
	}
	if len(mediaList) > c.MaxMediaCount {
		return errs.Validationf("%s allows at most %d media", name, c.MaxMediaCount).WithField("media_ids", "too many")
	}
	for _, m := range mediaList {
		if m.Kind == media.KindImage && !c.CanPublishImage {
			return errs.Validationf("%s does not support images", name).WithField("media_ids", "unsupported")
		}
		if m.Kind == media.KindImage && c.MaxImageBytes > 0 && m.SizeBytes > c.MaxImageBytes {
			return errs.Validationf("%s accepts images up to %d bytes", name, c.MaxImageBytes).WithField("media_ids", "too large")
		}
		if m.Kind == media.KindVideo && !c.CanPublishVideo {
			return errs.Validationf("%s does not support video in this release", name).WithField("media_ids", "unsupported")
		}
	}
	return nil
}

func (s *Service) validateScheduleTime(at time.Time) error {
	now := s.clock.Now()
	if !at.After(now) {
		return errs.Validationf("scheduled_at must be in the future").WithField("scheduled_at", "must be in the future")
	}
	if at.After(now.Add(MaxScheduleIn)) {
		return errs.Validationf("scheduled_at is too far in the future").WithField("scheduled_at", "max 1 year ahead")
	}
	return nil
}

func mediaIDs(ms []media.Media) []uuid.UUID {
	out := make([]uuid.UUID, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}
