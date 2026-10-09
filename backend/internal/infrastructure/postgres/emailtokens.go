package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/emailtoken"
)

// EmailTokens implements auth.EmailTokens.
type EmailTokens struct{ db *DB }

// NewEmailTokens creates the repo.
func NewEmailTokens(db *DB) *EmailTokens { return &EmailTokens{db: db} }

const tokenCols = `id, user_id, purpose, token_hash, email, expires_at, used_at, created_at`

func scanToken(row interface{ Scan(...any) error }) (*emailtoken.Token, error) {
	var t emailtoken.Token
	if err := row.Scan(&t.ID, &t.UserID, &t.Purpose, &t.Hash, &t.Email, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// Create stores t and retires the user's earlier unused tokens of the same
// purpose. The user row is locked so two concurrent requests cannot leave two
// live links.
func (r *EmailTokens) Create(ctx context.Context, t *emailtoken.Token) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		q := r.db.q(ctx)
		if err := lockUser(ctx, q, t.UserID); err != nil {
			return err
		}
		if err := r.RetireAll(ctx, t.UserID, t.Purpose, t.CreatedAt); err != nil {
			return err
		}
		got, err := scanToken(q.QueryRow(ctx, `INSERT INTO email_tokens (user_id, purpose, token_hash, email, expires_at, created_at)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+tokenCols, t.UserID, t.Purpose, t.Hash, t.Email, t.ExpiresAt, t.CreatedAt))
		if err != nil {
			return mapErr(err, "email token")
		}
		*t = *got
		return nil
	})
}

// Consume redeems a token in a single statement, so two concurrent redemptions
// cannot both succeed. The token itself identifies its owner (no user id is
// available to the caller of a mailed link).
func (r *EmailTokens) Consume(ctx context.Context, purpose emailtoken.Purpose, hash string, now time.Time) (*emailtoken.Token, error) {
	t, err := scanToken(r.db.q(ctx).QueryRow(ctx, `
		UPDATE email_tokens SET used_at = $3
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > $3
		RETURNING `+tokenCols, hash, purpose, now))
	return t, mapErr(err, "link")
}

// RetireAll marks the user's unused tokens of a purpose as used.
func (r *EmailTokens) RetireAll(ctx context.Context, userID uuid.UUID, purpose emailtoken.Purpose, now time.Time) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE email_tokens SET used_at = $3 WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`,
		userID, purpose, now)
	return mapErr(err, "email token")
}

// LatestCreatedAt returns the creation time of the user's newest token of a purpose (zero if none).
func (r *EmailTokens) LatestCreatedAt(ctx context.Context, userID uuid.UUID, purpose emailtoken.Purpose) (time.Time, error) {
	var at *time.Time
	err := r.db.q(ctx).QueryRow(ctx, `SELECT max(created_at) FROM email_tokens WHERE user_id = $1 AND purpose = $2`, userID, purpose).Scan(&at)
	if err != nil || at == nil {
		return time.Time{}, mapErr(err, "email token")
	}
	return *at, nil
}

// CountSince counts the user's tokens of a purpose created at or after `since`.
func (r *EmailTokens) CountSince(ctx context.Context, userID uuid.UUID, purpose emailtoken.Purpose, since time.Time) (int, error) {
	var n int
	err := r.db.q(ctx).QueryRow(ctx, `SELECT count(*) FROM email_tokens WHERE user_id = $1 AND purpose = $2 AND created_at >= $3`,
		userID, purpose, since).Scan(&n)
	return n, mapErr(err, "email token")
}

// RetireByID marks one unused token as used. An unknown or already used id is not an error.
func (r *EmailTokens) RetireByID(ctx context.Context, id uuid.UUID, now time.Time) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE email_tokens SET used_at = $2 WHERE id = $1 AND used_at IS NULL`, id, now)
	return mapErr(err, "email token")
}
