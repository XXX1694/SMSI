package posts

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
)

type countingQuota struct {
	calls int
	err   error
}

func (q *countingQuota) PrecheckAccount(context.Context, uuid.UUID, string) error { return nil }
func (q *countingQuota) EnforcePost(context.Context, uuid.UUID) error             { q.calls++; return q.err }
func (q *countingQuota) EnforceAccount(context.Context, uuid.UUID, string, string) error {
	return nil
}
func (q *countingQuota) EnforceMedia(context.Context, uuid.UUID, int64) error { return nil }

func TestCountQuotaCountsAPostOnlyOnce(t *testing.T) {
	q := &countingQuota{}
	s := &Service{clock: fixedNow{now0}, quota: q}
	p := testPost()
	if err := s.countQuota(context.Background(), p); err != nil || p.QuotaCountedAt == nil || !p.QuotaCountedAt.Equal(now0) {
		t.Fatalf("first: %v %v", err, p.QuotaCountedAt)
	}
	// Scheduling again after unschedule, retrying, publishing a scheduled post: already counted.
	if err := s.countQuota(context.Background(), p); err != nil || q.calls != 1 {
		t.Fatalf("second: %v calls=%d", err, q.calls)
	}
}

func TestCountQuotaRefusalLeavesThePostUncounted(t *testing.T) {
	q := &countingQuota{err: errs.New(errs.QuotaExceeded, "full")}
	s := &Service{clock: fixedNow{now0}, quota: q}
	p := testPost()
	if err := s.countQuota(context.Background(), p); !errs.Is(err, errs.QuotaExceeded) || p.QuotaCountedAt != nil {
		t.Fatalf("%v %v", err, p.QuotaCountedAt)
	}
}

// A post counted in an earlier month is counted again when it is scheduled or published in a later one.
func TestCountQuotaCountsAgainInANewMonth(t *testing.T) {
	q := &countingQuota{}
	s := &Service{clock: fixedNow{now0}, quota: q}
	p := testPost()
	lastMonth := now0.AddDate(0, -1, 0)
	p.QuotaCountedAt = &lastMonth
	if err := s.countQuota(context.Background(), p); err != nil || q.calls != 1 || !p.QuotaCountedAt.Equal(now0) {
		t.Fatalf("calls=%d counted=%v err=%v", q.calls, p.QuotaCountedAt, err)
	}
	earlier := now0.Add(-time.Hour) // same month
	p.QuotaCountedAt = &earlier
	if err := s.countQuota(context.Background(), p); err != nil || q.calls != 1 {
		t.Fatalf("same month must stay free: calls=%d err=%v", q.calls, err)
	}
}
