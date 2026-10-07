package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/analytics"
	"github.com/socialos/backend/internal/domain/post"
)

// Analytics implements analytics.Repo.
type Analytics struct {
	db    *DB
	posts *Posts
}

// NewAnalytics creates the repo.
func NewAnalytics(db *DB) *Analytics { return &Analytics{db: db, posts: NewPosts(db)} }

// Counts computes dashboard counters in one round trip.
func (r *Analytics) Counts(ctx context.Context, userID uuid.UUID, monthStart time.Time) (analytics.Counts, error) {
	var c analytics.Counts
	err := r.db.q(ctx).QueryRow(ctx, `SELECT
		(SELECT count(*) FROM social_accounts WHERE user_id = $1 AND status = 'active'),
		(SELECT count(*) FROM posts WHERE user_id = $1 AND deleted_at IS NULL AND status = 'scheduled'),
		(SELECT count(*) FROM posts WHERE user_id = $1 AND deleted_at IS NULL AND status = 'draft'),
		(SELECT count(*) FROM posts WHERE user_id = $1 AND deleted_at IS NULL AND status IN ('published','partially_published') AND published_at >= $2),
		(SELECT count(*) FROM posts WHERE user_id = $1 AND deleted_at IS NULL AND status = 'failed')`,
		userID, monthStart).Scan(&c.ConnectedAccounts, &c.ScheduledPosts, &c.Drafts, &c.PublishedThisMonth, &c.Failed)
	return c, err
}

// Upcoming returns the next scheduled posts.
func (r *Analytics) Upcoming(ctx context.Context, userID uuid.UUID, now time.Time, limit int) ([]post.Post, error) {
	return r.posts.queryPosts(ctx, userID, `SELECT `+postCols+` FROM posts p WHERE p.user_id = $1 AND p.deleted_at IS NULL
		AND p.status = 'scheduled' AND p.scheduled_at >= $2 ORDER BY p.scheduled_at LIMIT $3`, userID, now, limit)
}

// Recent returns the latest finished publications (published, partially
// published or failed); drafts and scheduled posts are not publications.
func (r *Analytics) Recent(ctx context.Context, userID uuid.UUID, limit int) ([]post.Post, error) {
	return r.posts.queryPosts(ctx, userID, `SELECT `+postCols+` FROM posts p WHERE p.user_id = $1 AND p.deleted_at IS NULL
		AND p.status IN ('published', 'partially_published', 'failed')
		ORDER BY COALESCE(p.published_at, p.updated_at) DESC LIMIT $2`, userID, limit)
}

// Metrics aggregates analytics by platform, metric and UTC day.
func (r *Analytics) Metrics(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]analytics.MetricRow, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT s.provider, a.metric, date_trunc('day', a.captured_at AT TIME ZONE 'UTC') AS day, sum(a.value)::bigint
		FROM analytics a JOIN social_accounts s ON s.id = a.social_account_id AND s.user_id = a.user_id
		WHERE a.user_id = $1 AND a.captured_at >= $2 AND a.captured_at < $3
		GROUP BY 1, 2, 3 ORDER BY 3, 1, 2`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analytics.MetricRow{}
	for rows.Next() {
		var m analytics.MetricRow
		if err := rows.Scan(&m.Platform, &m.Metric, &m.Day, &m.Value); err != nil {
			return nil, err
		}
		m.Day = m.Day.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// Increment inserts a counter sample.
func (r *Analytics) Increment(ctx context.Context, userID, accountID, targetID uuid.UUID, metric string, value int64) error {
	var tid any
	if targetID != uuid.Nil {
		tid = targetID
	}
	_, err := r.db.q(ctx).Exec(ctx, `INSERT INTO analytics (user_id, social_account_id, post_target_id, metric, value) VALUES ($1,$2,$3,$4,$5)`,
		userID, accountID, tid, metric, value)
	return mapErr(err, "analytics")
}
