package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Quota implements quota.Usage. Counts use these indexes: social_accounts(user_id, ...), posts_user_quota_idx,
// media_user_created_idx.
type Quota struct{ db *DB }

// NewQuota creates the repo.
func NewQuota(db *DB) *Quota { return &Quota{db: db} }

// LockUser takes a row lock that conflicts only with itself (not with the key-share locks that inserts of child rows
// take), so concurrent quota changes of one user queue up while everything else proceeds. Call it inside a transaction.
func (r *Quota) LockUser(ctx context.Context, userID uuid.UUID) error {
	var one int
	return mapErr(r.db.q(ctx).QueryRow(ctx, `SELECT 1 FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&one), "user")
}

// CountAccounts counts non-revoked accounts, leaving out one provider identity.
func (r *Quota) CountAccounts(ctx context.Context, userID uuid.UUID, exceptProvider, exceptProviderAccountID string) (int64, error) {
	var n int64
	err := r.db.q(ctx).QueryRow(ctx, `SELECT count(*) FROM social_accounts
		WHERE user_id = $1 AND status <> 'revoked' AND NOT (provider = $2 AND provider_account_id = $3)`,
		userID, exceptProvider, exceptProviderAccountID).Scan(&n)
	return n, mapErr(err, "social account")
}

// CountPostsSince counts posts counted against the monthly quota at or after t (deleted posts included on purpose).
func (r *Quota) CountPostsSince(ctx context.Context, userID uuid.UUID, t time.Time) (int64, error) {
	var n int64
	err := r.db.q(ctx).QueryRow(ctx, `SELECT count(*) FROM posts WHERE user_id = $1 AND quota_counted_at >= $2`, userID, t).Scan(&n)
	return n, mapErr(err, "post")
}

// SumMediaBytes adds up the user's media sizes.
func (r *Quota) SumMediaBytes(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.q(ctx).QueryRow(ctx, `SELECT COALESCE(sum(size_bytes), 0)::bigint FROM media WHERE user_id = $1`, userID).Scan(&n)
	return n, mapErr(err, "media")
}

// Plan returns the user's plan.
func (r *Quota) Plan(ctx context.Context, userID uuid.UUID) (string, error) {
	var p string
	err := r.db.q(ctx).QueryRow(ctx, `SELECT plan FROM users WHERE id = $1`, userID).Scan(&p)
	return p, mapErr(err, "user")
}
