package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
)

// Users implements auth.Users.
type Users struct{ db *DB }

// NewUsers creates the repo.
func NewUsers(db *DB) *Users { return &Users{db: db} }

// password_hash is NULL for users who signed up through a provider; the domain sees "" (user.HasPassword).
const userCols = `id, email, COALESCE(password_hash, ''), display_name, status, created_at, email_verified_at, plan, deleted_at, terms_accepted_at, terms_version, deletion_scheduled_at`

func scanUser(row interface{ Scan(...any) error }) (*user.User, error) {
	var u user.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status, &u.CreatedAt,
		&u.EmailVerifiedAt, &u.Plan, &u.DeletedAt, &u.TermsAcceptedAt, &u.TermsVersion, &u.DeletionScheduledAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// Create inserts a user; duplicate emails yield CONFLICT.
func (r *Users) Create(ctx context.Context, u *user.User) error {
	err := r.db.q(ctx).QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name, status, terms_accepted_at, terms_version)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at`,
		u.Email, nullIfEmpty(u.PasswordHash), u.DisplayName, u.Status, u.TermsAcceptedAt, u.TermsVersion).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		mapped := mapErr(err, "user")
		if errs.Is(mapped, errs.Conflict) {
			return errs.New(errs.Conflict, "an account with this email already exists")
		}
		return mapped
	}
	return nil
}

// GetByEmail finds a user by (citext) email.
func (r *Users) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	u, err := scanUser(r.db.q(ctx).QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email))
	return u, mapErr(err, "user")
}

// GetByID finds a user by id.
func (r *Users) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	u, err := scanUser(r.db.q(ctx).QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	return u, mapErr(err, "user")
}

// SetPassword replaces the user's password hash.
func (r *Users) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, hash)
	return mustAffect(tag, err, "user")
}

// RehashPassword replaces the hash only if it is still oldHash, so an upgrade of the encoding never overwrites a
// password changed in the meantime. No matching row is not an error.
func (r *Users) RehashPassword(ctx context.Context, id uuid.UUID, oldHash, newHash string) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1 AND password_hash = $3`, id, newHash, oldHash)
	return err
}

// MarkEmailVerified records the verification time once; later calls keep the first time.
func (r *Users) MarkEmailVerified(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE users SET email_verified_at = COALESCE(email_verified_at, $2) WHERE id = $1`, id, at)
	return mustAffect(tag, err, "user")
}

// Sessions implements auth.Sessions.
type Sessions struct{ db *DB }

// NewSessions creates the repo.
func NewSessions(db *DB) *Sessions { return &Sessions{db: db} }

// Create inserts a session.
func (r *Sessions) Create(ctx context.Context, s *auth.Session) error {
	return mapErr(r.db.q(ctx).QueryRow(ctx,
		`INSERT INTO sessions (user_id, token_hash, csrf_token, expires_at, user_agent, ip, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6, $7) RETURNING id`,
		s.UserID, s.TokenHash, s.CSRFToken, s.ExpiresAt, s.UserAgent, s.IP, s.CreatedAt).Scan(&s.ID), "session")
}

// GetByTokenHash finds a session by token hash.
func (r *Sessions) GetByTokenHash(ctx context.Context, hash string) (*auth.Session, error) {
	var s auth.Session
	err := r.db.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, token_hash, csrf_token, expires_at, user_agent, ip, created_at FROM sessions WHERE token_hash = $1`, hash).
		Scan(&s.ID, &s.UserID, &s.TokenHash, &s.CSRFToken, &s.ExpiresAt, &s.UserAgent, &s.IP, &s.CreatedAt)
	if err != nil {
		return nil, mapErr(err, "session")
	}
	return &s, nil
}

// Delete removes one of the user's sessions.
func (r *Sessions) Delete(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.db.q(ctx).Exec(ctx, `DELETE FROM sessions WHERE id = $1 AND user_id = $2`, id, userID)
	return mapErr(err, "session")
}

// DeleteExpired purges expired sessions (system).
func (r *Sessions) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	tag, err := r.db.q(ctx).Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	return tag.RowsAffected(), mapErr(err, "session")
}

// DeleteAllForUser removes the user's sessions except `except` (uuid.Nil keeps none).
func (r *Sessions) DeleteAllForUser(ctx context.Context, userID, except uuid.UUID) (int64, error) {
	tag, err := r.db.q(ctx).Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND id <> $2`, userID, except)
	return tag.RowsAffected(), mapErr(err, "session")
}

// nullIfEmpty maps "" to SQL NULL.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// BlockReason says why the user's due posts must not go out ("" when they may): the publisher's check (system). The row
// is locked FOR SHARE, so a purge claim (UPDATE) waits for a publisher transaction that is already past the check.
func (r *Users) BlockReason(ctx context.Context, id uuid.UUID) (string, error) {
	var status string
	var scheduled bool
	err := r.db.q(ctx).QueryRow(ctx, `SELECT status, deletion_scheduled_at IS NOT NULL FROM users WHERE id = $1 FOR SHARE`, id).Scan(&status, &scheduled)
	if errs.Is(mapErr(err, "user"), errs.NotFound) {
		return scheduler.SkipAccountDeletion, nil
	}
	switch {
	case err != nil:
		return "", err
	case status == string(user.StatusDisabled):
		return scheduler.SkipOwnerDisabled, nil
	case status != string(user.StatusActive) || scheduled:
		return scheduler.SkipAccountDeletion, nil
	}
	return "", nil
}
