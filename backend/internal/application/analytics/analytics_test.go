package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeRepo struct {
	rows       []MetricRow
	gotFrom    time.Time
	gotTo      time.Time
	gotMonth   time.Time
	gotLimit   int
	metricCall int
}

func (f *fakeRepo) Counts(_ context.Context, _ uuid.UUID, month time.Time) (Counts, error) {
	f.gotMonth = month
	return Counts{ConnectedAccounts: 2, Drafts: 3}, nil
}
func (f *fakeRepo) Upcoming(_ context.Context, _ uuid.UUID, _ time.Time, limit int) ([]post.Post, error) {
	f.gotLimit = limit
	return nil, nil
}
func (f *fakeRepo) Recent(context.Context, uuid.UUID, int) ([]post.Post, error) { return nil, nil }
func (f *fakeRepo) Metrics(_ context.Context, _ uuid.UUID, from, to time.Time) ([]MetricRow, error) {
	f.metricCall++
	f.gotFrom, f.gotTo = from, to
	return f.rows, nil
}
func (f *fakeRepo) Increment(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, int64) error {
	return nil
}

var now = time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)

func user() actor.Actor {
	return actor.Actor{UserID: uuid.New(), Type: actor.TypeUser, SessionID: uuid.New()}
}

func keyWith(scopes ...apikey.Scope) actor.Actor {
	return actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, Scopes: scopes}
}

func TestAnalyticsDefaultsToLast30Days(t *testing.T) {
	repo := &fakeRepo{rows: []MetricRow{{Metric: "published", Value: 2}, {Metric: "published", Value: 3}, {Metric: "failed", Value: 1}}}
	s := NewService(repo, fakeClock{now})
	rep, err := s.Analytics(context.Background(), user(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.gotTo.Equal(now) || !repo.gotFrom.Equal(now.Add(-30*24*time.Hour)) || !rep.To.Equal(now) {
		t.Fatalf("defaults: %v .. %v", repo.gotFrom, repo.gotTo)
	}
	if rep.Totals["published"] != 5 || rep.Totals["failed"] != 1 || len(rep.Rows) != 3 {
		t.Fatalf("totals: %v", rep.Totals)
	}
	// Only `from` given: `to` is now.
	from := now.Add(-48 * time.Hour)
	if _, err := s.Analytics(context.Background(), user(), from, time.Time{}); err != nil || !repo.gotFrom.Equal(from) || !repo.gotTo.Equal(now) {
		t.Fatalf("from only: %v %v %v", err, repo.gotFrom, repo.gotTo)
	}
}

func TestAnalyticsRangeValidation(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, fakeClock{now})
	for name, tc := range map[string]struct{ from, to time.Time }{
		"from after to":       {now, now.Add(-time.Hour)},
		"empty range":         {now, now},
		"over 366 days":       {now.Add(-367 * 24 * time.Hour), now},
		"just over the limit": {now.Add(-MaxRange - time.Second), now},
	} {
		err := func() error { _, err := s.Analytics(context.Background(), user(), tc.from, tc.to); return err }()
		if !errs.Is(err, errs.Validation) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if repo.metricCall != 0 {
		t.Fatal("invalid ranges must not hit the database")
	}
	if _, err := s.Analytics(context.Background(), user(), now.Add(-MaxRange), now); err != nil {
		t.Fatalf("exactly 366 days is allowed: %v", err)
	}
}

func TestScopes(t *testing.T) {
	s := NewService(&fakeRepo{}, fakeClock{now})
	ctx := context.Background()
	if _, err := s.Analytics(ctx, keyWith(apikey.PostsRead), time.Time{}, time.Time{}); !errs.Is(err, errs.InsufficientScope) {
		t.Fatalf("analytics needs analytics:read: %v", err)
	}
	if _, err := s.Analytics(ctx, keyWith(apikey.AnalyticsRead), time.Time{}, time.Time{}); err != nil {
		t.Fatalf("analytics:read suffices: %v", err)
	}
	if _, err := s.Dashboard(ctx, keyWith(apikey.AnalyticsRead)); !errs.Is(err, errs.InsufficientScope) {
		t.Fatalf("dashboard needs posts:read: %v", err)
	}
	if _, err := s.Dashboard(ctx, keyWith(apikey.PostsRead)); err != nil {
		t.Fatalf("posts:read suffices: %v", err)
	}
	if _, err := s.Analytics(ctx, actor.Actor{}, time.Time{}, time.Time{}); !errs.Is(err, errs.Unauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
}

func TestDashboardMonthWindowIsUTC(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, fakeClock{time.Date(2026, 12, 31, 23, 59, 59, 0, time.FixedZone("x", -8*3600))})
	sum, err := s.Dashboard(context.Background(), user())
	if err != nil {
		t.Fatal(err)
	}
	// 23:59 in UTC-8 on Dec 31 is already January in UTC.
	if want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC); !repo.gotMonth.Equal(want) {
		t.Fatalf("month start %v, want %v", repo.gotMonth, want)
	}
	if sum.ConnectedAccounts != 2 || sum.Drafts != 3 || repo.gotLimit != 5 {
		t.Fatalf("%+v limit=%d", sum, repo.gotLimit)
	}
}
