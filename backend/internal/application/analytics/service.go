// Package analytics serves the dashboard summary and analytics metrics.
package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
)

// Counts are dashboard counters.
type Counts struct {
	ConnectedAccounts  int64
	ScheduledPosts     int64
	Drafts             int64
	PublishedThisMonth int64
	Failed             int64
}

// MetricRow is an aggregated analytics value.
type MetricRow struct {
	Platform string
	Metric   string
	Day      time.Time
	Value    int64
}

// Repo reads aggregates (tenant-scoped) and records counters.
type Repo interface {
	Counts(ctx context.Context, userID uuid.UUID, monthStart time.Time) (Counts, error)
	Upcoming(ctx context.Context, userID uuid.UUID, now time.Time, limit int) ([]post.Post, error)
	Recent(ctx context.Context, userID uuid.UUID, limit int) ([]post.Post, error)
	Metrics(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]MetricRow, error)
	Increment(ctx context.Context, userID, accountID, targetID uuid.UUID, metric string, value int64) error
}

// Service implements dashboard and analytics reads.
type Service struct {
	repo  Repo
	clock port.Clock
}

// NewService creates the service.
func NewService(repo Repo, clock port.Clock) *Service { return &Service{repo: repo, clock: clock} }

// Summary is the dashboard payload.
type Summary struct {
	Counts
	Upcoming []post.Post
	Recent   []post.Post
}

// Dashboard returns counters plus upcoming and recent posts.
func (s *Service) Dashboard(ctx context.Context, a actor.Actor) (*Summary, error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	c, err := s.repo.Counts(ctx, a.UserID, monthStart)
	if err != nil {
		return nil, err
	}
	up, err := s.repo.Upcoming(ctx, a.UserID, now, 5)
	if err != nil {
		return nil, err
	}
	recent, err := s.repo.Recent(ctx, a.UserID, 5)
	if err != nil {
		return nil, err
	}
	return &Summary{Counts: c, Upcoming: up, Recent: recent}, nil
}

// Report is the analytics payload.
type Report struct {
	From, To time.Time
	Totals   map[string]int64
	Rows     []MetricRow
}

// MaxRange bounds analytics queries.
const MaxRange = 366 * 24 * time.Hour

// Analytics aggregates metrics in [from, to). Zero values default to the last 30 days.
func (s *Service) Analytics(ctx context.Context, a actor.Actor, from, to time.Time) (*Report, error) {
	if err := a.Require(apikey.AnalyticsRead); err != nil {
		return nil, err
	}
	if to.IsZero() {
		to = s.clock.Now()
	}
	if from.IsZero() {
		from = to.Add(-30 * 24 * time.Hour)
	}
	if !from.Before(to) || to.Sub(from) > MaxRange {
		return nil, errs.Validationf("invalid range: from must be before to and span at most 366 days").WithField("from", "invalid range")
	}
	rows, err := s.repo.Metrics(ctx, a.UserID, from, to)
	if err != nil {
		return nil, err
	}
	totals := map[string]int64{}
	for _, r := range rows {
		totals[r.Metric] += r.Value
	}
	return &Report{From: from, To: to, Totals: totals, Rows: rows}, nil
}

// Increment records an internal counter (used by the publisher).
func (s *Service) Increment(ctx context.Context, userID, accountID, targetID uuid.UUID, metric string, value int64) error {
	return s.repo.Increment(ctx, userID, accountID, targetID, metric, value)
}
