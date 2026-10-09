package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/identity"
)

// Identities implements auth.Identities.
type Identities struct{ db *DB }

var _ auth.Identities = (*Identities)(nil)

// NewIdentities creates the repo.
func NewIdentities(db *DB) *Identities { return &Identities{db: db} }

const identityCols = `id, user_id, provider, subject, email, email_verified, linked_at, last_login_at`

func scanIdentity(row interface{ Scan(...any) error }) (*identity.Identity, error) {
	var i identity.Identity
	if err := row.Scan(&i.ID, &i.UserID, &i.Provider, &i.Subject, &i.Email, &i.EmailVerified, &i.LinkedAt, &i.LastLoginAt); err != nil {
		return nil, err
	}
	return &i, nil
}

// Create links an identity; a taken (provider, subject) or a second identity of the user for the provider is CONFLICT.
func (r *Identities) Create(ctx context.Context, i *identity.Identity) error {
	got, err := scanIdentity(r.db.q(ctx).QueryRow(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, email, email_verified, linked_at)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+identityCols,
		i.UserID, i.Provider, i.Subject, i.Email, i.EmailVerified, i.LinkedAt))
	if err != nil {
		return mapErr(err, "identity")
	}
	*i = *got
	return nil
}

// GetBySubject finds the identity by the provider's stable id. It is the one lookup that is not scoped to a user.
func (r *Identities) GetBySubject(ctx context.Context, p identity.Provider, subject string) (*identity.Identity, error) {
	i, err := scanIdentity(r.db.q(ctx).QueryRow(ctx,
		`SELECT `+identityCols+` FROM user_identities WHERE provider = $1 AND subject = $2`, p, subject))
	return i, mapErr(err, "identity")
}

// ListByUser returns the user's identities, oldest first (served by the (user_id, provider) unique index).
func (r *Identities) ListByUser(ctx context.Context, userID uuid.UUID) ([]identity.Identity, error) {
	rows, err := r.db.q(ctx).Query(ctx,
		`SELECT `+identityCols+` FROM user_identities WHERE user_id = $1 ORDER BY linked_at, provider`, userID)
	if err != nil {
		return nil, mapErr(err, "identity")
	}
	defer rows.Close()
	var out []identity.Identity
	for rows.Next() {
		i, err := scanIdentity(rows)
		if err != nil {
			return nil, mapErr(err, "identity")
		}
		out = append(out, *i)
	}
	return out, mapErr(rows.Err(), "identity")
}

// LockUser takes the user's row lock, the same lock the e-mail token and link-code repositories use.
func (r *Identities) LockUser(ctx context.Context, userID uuid.UUID) error {
	var locked uuid.UUID
	return mapErr(r.db.q(ctx).QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&locked), "user")
}

// Delete unlinks the user's identity for a provider; another user's identity is NOT_FOUND.
func (r *Identities) Delete(ctx context.Context, userID uuid.UUID, p identity.Provider) error {
	tag, err := r.db.q(ctx).Exec(ctx, `DELETE FROM user_identities WHERE user_id = $1 AND provider = $2`, userID, p)
	return mustAffect(tag, err, "identity")
}

// TouchLogin records a sign-in and the email the provider reported.
func (r *Identities) TouchLogin(ctx context.Context, userID uuid.UUID, p identity.Provider, email string, verified bool, at time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx,
		`UPDATE user_identities SET last_login_at = $3, email = $4, email_verified = $5 WHERE user_id = $1 AND provider = $2`,
		userID, p, at, email, verified)
	return mustAffect(tag, err, "identity")
}
