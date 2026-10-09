package posts

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
)

type countingQuota struct {
	calls int
	err   error
}

func (q *countingQuota) EnforcePost(context.Context, uuid.UUID) error { q.calls++; return q.err }
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
