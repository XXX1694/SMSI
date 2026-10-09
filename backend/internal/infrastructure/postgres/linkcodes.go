package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/linkcode"
)

// LinkCodes implements accounts.LinkCodes.
type LinkCodes struct{ db *DB }

// NewLinkCodes creates the repo.
func NewLinkCodes(db *DB) *LinkCodes { return &LinkCodes{db: db} }

const linkCols = `id, user_id, code_hash, expires_at, used_at, chat_id, social_account_id, created_at`

func scanLink(row interface{ Scan(...any) error }) (*linkcode.Code, error) {
	var c linkcode.Code
	if err := row.Scan(&c.ID, &c.UserID, &c.Hash, &c.ExpiresAt, &c.UsedAt, &c.ChatID, &c.SocialAccountID, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// staleAfter is how long finished rows are kept before a new link purges them.
const staleAfter = 24 * time.Hour

// Create stores c for its user. In one transaction (serialised per user by a
// lock on the user row) it purges the user's long-finished codes and retires
// the oldest active ones so that at most maxActive-1 remain before the insert.
func (r *LinkCodes) Create(ctx context.Context, c *linkcode.Code, maxActive int, now time.Time) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		q := r.db.q(ctx)
		if err := lockUser(ctx, q, c.UserID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM telegram_link_codes WHERE user_id = $1 AND expires_at < $2`, c.UserID, now.Add(-staleAfter)); err != nil {
			return mapErr(err, "link code")
		}
		if _, err := q.Exec(ctx, `
			UPDATE telegram_link_codes SET expires_at = $2
			WHERE id IN (
			  SELECT id FROM telegram_link_codes
			  WHERE user_id = $1 AND used_at IS NULL AND expires_at > $2
			  ORDER BY created_at DESC, id DESC OFFSET $3)`, c.UserID, now, maxActive-1); err != nil {
			return mapErr(err, "link code")
		}
		row := q.QueryRow(ctx, `INSERT INTO telegram_link_codes (user_id, code_hash, expires_at, created_at)
			VALUES ($1,$2,$3,$4) RETURNING `+linkCols, c.UserID, c.Hash, c.ExpiresAt, now)
		got, err := scanLink(row)
		if err != nil {
			return mapErr(err, "link code")
		}
		*c = *got
		return nil
	})
}

// Get returns one of the user's codes; another user's id is NOT_FOUND.
func (r *LinkCodes) Get(ctx context.Context, userID, id uuid.UUID) (*linkcode.Code, error) {
	c, err := scanLink(r.db.q(ctx).QueryRow(ctx, `SELECT `+linkCols+` FROM telegram_link_codes WHERE id = $1 AND user_id = $2`, id, userID))
	return c, mapErr(err, "link")
}

// FindByHash is the one cross-tenant lookup ("system"): the inbound bot message
// carries no user, the code itself identifies its owner. It is reachable only
// from the update handler, never from an HTTP request.
func (r *LinkCodes) FindByHash(ctx context.Context, hash string) (*linkcode.Code, error) {
	c, err := scanLink(r.db.q(ctx).QueryRow(ctx, `SELECT `+linkCols+` FROM telegram_link_codes WHERE code_hash = $1`, hash))
	return c, mapErr(err, "link code")
}

// MarkUsed atomically redeems an unused, unexpired code. NOT_FOUND means it was
// already used (or expired) in the meantime.
func (r *LinkCodes) MarkUsed(ctx context.Context, id uuid.UUID, now time.Time, chatID string, accountID uuid.UUID) error {
	tag, err := r.db.q(ctx).Exec(ctx, `
		UPDATE telegram_link_codes SET used_at = $2, chat_id = $3, social_account_id = $4
		WHERE id = $1 AND used_at IS NULL AND expires_at > $2`, id, now, chatID, accountID)
	return mustAffect(tag, err, "link code")
}
