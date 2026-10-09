// Package quota holds the plan limits and the pure rule that compares usage with a limit (D-014).
package quota

import (
	"fmt"
	"time"

	"github.com/socialos/backend/internal/domain/errs"
)

// Metric names one limited resource. The values are the keys of GET /account/usage.
type Metric string

const (
	ConnectedAccounts   Metric = "connected_accounts"
	ScheduledPostsMonth Metric = "scheduled_posts_month"
	MediaBytes          Metric = "media_bytes"
	AgentRPM            Metric = "agent_requests_per_minute"
)

// Unlimited is the limit value that switches a limit off.
const Unlimited = -1

// Limits are the caps of one plan. Unlimited (-1) switches a cap off.
type Limits struct {
	Accounts      int
	PostsPerMonth int
	MediaBytes    int64
	AgentRPM      int
}

// For returns the cap of a metric.
func (l Limits) For(m Metric) int64 {
	switch m {
	case ConnectedAccounts:
		return int64(l.Accounts)
	case ScheduledPostsMonth:
		return int64(l.PostsPerMonth)
	case MediaBytes:
		return l.MediaBytes
	case AgentRPM:
		return int64(l.AgentRPM)
	}
	return Unlimited
}

// MonthStart is the first instant of t's UTC month: the period the monthly post limit counts in.
func MonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Check returns QUOTA_EXCEEDED when used+delta would pass limit. A negative limit never fails.
func Check(m Metric, used, delta, limit int64) error {
	if limit < 0 || used+delta <= limit {
		return nil
	}
	return errs.New(errs.QuotaExceeded, message(m, used, delta, limit)).WithField("quota", string(m))
}

func message(m Metric, used, delta, limit int64) string {
	switch m {
	case ConnectedAccounts:
		return fmt.Sprintf("connected accounts limit reached (%d of %d used). Disconnect an account to connect another.", used, limit)
	case ScheduledPostsMonth:
		return fmt.Sprintf("monthly post limit reached (%d of %d used). The count restarts on the first day of next month (UTC).", used, limit)
	case MediaBytes:
		return fmt.Sprintf("storage limit reached (%d MB used of %d MB; this file adds %d MB). Delete media you no longer need.",
			used>>20, limit>>20, (delta+(1<<20)-1)>>20)
	}
	return fmt.Sprintf("limit reached (%d of %d used)", used, limit)
}
