package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
)

// Users implements auth.Users.
type Users struct{ db *DB }

// NewUsers creates the repo.
func NewUsers(db *DB) *Users { return &Users{db: db} }

const userCols = `id, email, password_hash, display_name, status, created_at`

func scanUser(row interface{ Scan(...any) error }) (*user.User, error) {
	var u user.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// Create inserts a user; duplicate emails yield CONFLICT.
func (r *Users) Create(ctx context.Context, u *user.User) error {
	err := r.db.q(ctx).QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name, status) VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		u.Email, u.PasswordHash, u.DisplayName, u.Status).Scan(&u.ID, &u.CreatedAt)
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

// Sessions implements auth.Sessions.
type Sessions struct{ db *DB }

// NewSessions creates the repo.
func NewSessions(db *DB) *Sessions { return &Sessions{db: db} }

// Create inserts a session.
func (r *Sessions) Create(ctx context.Context, s *auth.Session) error {
	return mapErr(r.db.q(ctx).QueryRow(ctx,
		`INSERT INTO sessions (user_id, token_hash, csrf_token, expires_at, user_agent, ip) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		s.UserID, s.TokenHash, s.CSRFToken, s.ExpiresAt, s.UserAgent, s.IP).Scan(&s.ID), "session")
}

// GetByTokenHash finds a session by token hash.
func (r *Sessions) GetByTokenHash(ctx context.Context, hash string) (*auth.Session, error) {
	var s auth.Session
	err := r.db.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, token_hash, csrf_token, expires_at, user_agent, ip FROM sessions WHERE token_hash = $1`, hash).
		Scan(&s.ID, &s.UserID, &s.TokenHash, &s.CSRFToken, &s.ExpiresAt, &s.UserAgent, &s.IP)
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
