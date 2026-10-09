package quota

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
	domain "github.com/socialos/backend/internal/domain/quota"
)

type fakeUsage struct {
	accounts, posts, bytes   int64
	locks                    int
	hasProvider              bool
	since                    time.Time
	exceptProvider, exceptID string
}

func (f *fakeUsage) LockUser(context.Context, uuid.UUID) error { f.locks++; return nil }
func (f *fakeUsage) CountAccounts(_ context.Context, _ uuid.UUID, p, id string) (int64, error) {
	f.exceptProvider, f.exceptID = p, id
	return f.accounts, nil
}
func (f *fakeUsage) HasProvider(context.Context, uuid.UUID, string) (bool, error) {
	return f.hasProvider, nil
}
func (f *fakeUsage) CountPostsSince(_ context.Context, _ uuid.UUID, t time.Time) (int64, error) {
	f.since = t
	return f.posts, nil
}
func (f *fakeUsage) SumMediaBytes(context.Context, uuid.UUID) (int64, error) { return f.bytes, nil }
func (f *fakeUsage) Plan(context.Context, uuid.UUID) (string, error)         { return "free", nil }

type clk struct{ t time.Time }

func (c clk) Now() time.Time { return c.t }

var oct = time.Date(2026, 10, 31, 23, 59, 0, 0, time.FixedZone("x", 3*3600)) // 20:59 UTC on the 31st

func TestEnforceLocksCountsAndRefuses(t *testing.T) {
	u := &fakeUsage{accounts: 5, posts: 60, bytes: 10 << 20}
	s := NewService(u, domain.Limits{Accounts: 5, PostsPerMonth: 60, MediaBytes: 10 << 20}, clk{oct})
	ctx := context.Background()
	if err := s.EnforceAccount(ctx, uuid.New(), "mock", "id1"); !errs.Is(err, errs.QuotaExceeded) {
		t.Fatalf("accounts: %v", err)
	}
	if u.exceptProvider != "mock" || u.exceptID != "id1" {
		t.Fatalf("the identity being connected must be left out of the count: %q %q", u.exceptProvider, u.exceptID)
	}
	if err := s.EnforcePost(ctx, uuid.New()); !errs.Is(err, errs.QuotaExceeded) {
		t.Fatalf("posts: %v", err)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !u.since.Equal(want) {
		t.Fatalf("month starts %v, want %v (UTC)", u.since, want)
	}
	if err := s.EnforceMedia(ctx, uuid.New(), 1); !errs.Is(err, errs.QuotaExceeded) {
		t.Fatalf("media: %v", err)
	}
	if u.locks != 3 {
		t.Fatalf("each check must lock the user: %d", u.locks)
	}
	u.accounts, u.posts, u.bytes = 4, 59, 9<<20
	for name, err := range map[string]error{"accounts": s.EnforceAccount(ctx, uuid.New(), "m", "i"), "posts": s.EnforcePost(ctx, uuid.New()),
		"media": s.EnforceMedia(ctx, uuid.New(), 1<<20)} {
		if err != nil {
			t.Errorf("%s one below the limit: %v", name, err)
		}
	}
}

func TestUnlimitedSkipsEverything(t *testing.T) {
	u := &fakeUsage{accounts: 1000, posts: 1000, bytes: 1 << 40}
	s := NewService(u, domain.Limits{Accounts: -1, PostsPerMonth: -1, MediaBytes: -1}, clk{oct})
	ctx := context.Background()
	if s.EnforceAccount(ctx, uuid.New(), "m", "i") != nil || s.EnforcePost(ctx, uuid.New()) != nil || s.EnforceMedia(ctx, uuid.New(), 1<<40) != nil || u.locks != 0 {
		t.Fatalf("unlimited must not check or lock (locks=%d)", u.locks)
	}
}

func TestReportNeedsAnalyticsScope(t *testing.T) {
	s := NewService(&fakeUsage{posts: 3}, domain.Limits{PostsPerMonth: 60}, clk{oct})
	key := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, Scopes: []apikey.Scope{apikey.PostsRead}}
	if _, err := s.Report(context.Background(), key); !errs.Is(err, errs.InsufficientScope) {
		t.Fatalf("err=%v", err)
	}
	key.Scopes = append(key.Scopes, apikey.AnalyticsRead)
	rep, err := s.Report(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.PeriodEnd.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) || *rep.Items[domain.ScheduledPostsMonth].Used != 3 {
		t.Fatalf("%+v", rep)
	}
}

func TestPrecheckAccountOnlyRefusesWhatCannotBeAReconnect(t *testing.T) {
	u := &fakeUsage{accounts: 5}
	s := NewService(u, domain.Limits{Accounts: 5}, clk{oct})
	ctx := context.Background()
	if err := s.PrecheckAccount(ctx, uuid.New(), "mock"); !errs.Is(err, errs.QuotaExceeded) {
		t.Fatalf("at the limit with no account on the provider: %v", err)
	}
	u.hasProvider = true // may be a reconnect: the authoritative check in the transaction decides
	if err := s.PrecheckAccount(ctx, uuid.New(), "mock"); err != nil {
		t.Fatalf("provider already connected: %v", err)
	}
	u.hasProvider, u.accounts = false, 4
	if err := s.PrecheckAccount(ctx, uuid.New(), "mock"); err != nil || u.locks != 0 {
		t.Fatalf("below the limit: %v (locks=%d)", err, u.locks)
	}
}
