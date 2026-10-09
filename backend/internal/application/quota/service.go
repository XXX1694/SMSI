// Package quota enforces the plan limits and reports usage (D-014). Counting happens here, in the application layer,
// under a per-user lock taken in the caller's transaction, so there is no check-then-insert race.
package quota

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/quota"
)

// Usage reads what a user has used. Every method is tenant-scoped by userID.
type Usage interface {
	// LockUser serializes quota changes of one user until the surrounding transaction ends.
	LockUser(ctx context.Context, userID uuid.UUID) error
	// CountAccounts counts non-revoked social accounts, leaving out the given identity.
	CountAccounts(ctx context.Context, userID uuid.UUID, exceptProvider, exceptProviderAccountID string) (int64, error)
	// CountPostsSince counts posts that were counted against the monthly quota at or after t.
	CountPostsSince(ctx context.Context, userID uuid.UUID, t time.Time) (int64, error)
	// SumMediaBytes adds up the size of the user's media.
	SumMediaBytes(ctx context.Context, userID uuid.UUID) (int64, error)
	// Plan returns the user's plan name.
	Plan(ctx context.Context, userID uuid.UUID) (string, error)
}

// Service implements port.QuotaGate and the usage report.
type Service struct {
	usage  Usage
	limits quota.Limits
	clock  port.Clock
}

var _ port.QuotaGate = (*Service)(nil)

// NewService creates the service; limits apply to every plan (there is one: "free").
func NewService(u Usage, l quota.Limits, c port.Clock) *Service {
	return &Service{usage: u, limits: l, clock: c}
}

// Limits returns the configured caps.
func (s *Service) Limits() quota.Limits { return s.limits }

// MonthStart is the first instant of t's UTC month.
func MonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// EnforceAccount implements port.QuotaGate.
func (s *Service) EnforceAccount(ctx context.Context, userID uuid.UUID, provider, providerAccountID string) error {
	if s.limits.Accounts < 0 {
		return nil
	}
	if err := s.usage.LockUser(ctx, userID); err != nil {
		return err
	}
	n, err := s.usage.CountAccounts(ctx, userID, provider, providerAccountID)
	if err != nil {
		return err
	}
	return quota.Check(quota.ConnectedAccounts, n, 1, int64(s.limits.Accounts))
}

// EnforcePost implements port.QuotaGate.
func (s *Service) EnforcePost(ctx context.Context, userID uuid.UUID) error {
	if s.limits.PostsPerMonth < 0 {
		return nil
	}
	if err := s.usage.LockUser(ctx, userID); err != nil {
		return err
	}
	n, err := s.usage.CountPostsSince(ctx, userID, MonthStart(s.clock.Now()))
	if err != nil {
		return err
	}
	return quota.Check(quota.ScheduledPostsMonth, n, 1, int64(s.limits.PostsPerMonth))
}

// EnforceMedia implements port.QuotaGate.
func (s *Service) EnforceMedia(ctx context.Context, userID uuid.UUID, size int64) error {
	if s.limits.MediaBytes < 0 {
		return nil
	}
	if err := s.usage.LockUser(ctx, userID); err != nil {
		return err
	}
	n, err := s.usage.SumMediaBytes(ctx, userID)
	if err != nil {
		return err
	}
	return quota.Check(quota.MediaBytes, n, size, s.limits.MediaBytes)
}

// Item is one line of the usage report. Used is nil for limits that are not counted.
type Item struct {
	Used  *int64
	Limit int64
}

// Report is GET /account/usage.
type Report struct {
	Plan        string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Items       map[quota.Metric]Item
}

// Report returns the caller's usage next to the limits.
func (s *Service) Report(ctx context.Context, a actor.Actor) (*Report, error) {
	if err := a.Require(apikey.AnalyticsRead); err != nil {
		return nil, err
	}
	start := MonthStart(s.clock.Now())
	plan, err := s.usage.Plan(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	accounts, err := s.usage.CountAccounts(ctx, a.UserID, "", "")
	if err != nil {
		return nil, err
	}
	posts, err := s.usage.CountPostsSince(ctx, a.UserID, start)
	if err != nil {
		return nil, err
	}
	bytes, err := s.usage.SumMediaBytes(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return &Report{Plan: plan, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), Items: map[quota.Metric]Item{
		quota.ConnectedAccounts:   {Used: &accounts, Limit: s.limits.For(quota.ConnectedAccounts)},
		quota.ScheduledPostsMonth: {Used: &posts, Limit: s.limits.For(quota.ScheduledPostsMonth)},
		quota.MediaBytes:          {Used: &bytes, Limit: s.limits.For(quota.MediaBytes)},
	}}, nil
}
