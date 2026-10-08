// Package auth implements registration, login, sessions and API-key authentication.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/emailtoken"
	"github.com/socialos/backend/internal/domain/user"
)

// Session is a server-side browser session (token stored hashed).
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	CSRFToken string
	ExpiresAt time.Time
	UserAgent string
	IP        string
}

// Users persists users.
type Users interface {
	Create(ctx context.Context, u *user.User) error // CONFLICT on duplicate email
	GetByEmail(ctx context.Context, email string) (*user.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
	SetPassword(ctx context.Context, id uuid.UUID, hash string) error
	// MarkEmailVerified sets email_verified_at once; calling it again is a no-op.
	MarkEmailVerified(ctx context.Context, id uuid.UUID, at time.Time) error
}

// Sessions persists sessions.
type Sessions interface {
	Create(ctx context.Context, s *Session) error
	GetByTokenHash(ctx context.Context, hash string) (*Session, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
	// DeleteAllForUser removes every session of the user except `except`
	// (uuid.Nil keeps none) and returns how many it removed.
	DeleteAllForUser(ctx context.Context, userID, except uuid.UUID) (int64, error)
}

// EmailTokens persists the one-time tokens mailed to users.
type EmailTokens interface {
	// Create stores t and, in the same transaction, retires the user's earlier
	// unused tokens of the same purpose so that only the newest link works.
	Create(ctx context.Context, t *emailtoken.Token) error
	// Consume atomically redeems an unused, unexpired token. Anything else
	// (unknown, used, expired, retired, other purpose) is NOT_FOUND.
	Consume(ctx context.Context, purpose emailtoken.Purpose, hash string, now time.Time) (*emailtoken.Token, error)
	// RetireAll invalidates the user's unused tokens of a purpose.
	RetireAll(ctx context.Context, userID uuid.UUID, purpose emailtoken.Purpose, now time.Time) error
	// LatestCreatedAt is when the user's newest token of a purpose was made (zero if none).
	LatestCreatedAt(ctx context.Context, userID uuid.UUID, purpose emailtoken.Purpose) (time.Time, error)
	// CountSince counts the user's tokens of a purpose created at or after `since`.
	CountSince(ctx context.Context, userID uuid.UUID, purpose emailtoken.Purpose, since time.Time) (int, error)
	// RetireByID invalidates one unused token (used when its mail could not be delivered).
	RetireByID(ctx context.Context, id uuid.UUID, now time.Time) error
}

// APIKeys is the read side needed for authentication.
type APIKeys interface {
	GetByHash(ctx context.Context, hash string) (*apikey.Key, error)
	// TouchLastUsed updates last_used_at (and the MCP connection's last_seen_at), throttled.
	TouchLastUsed(ctx context.Context, keyID uuid.UUID, now time.Time) error
	// RevokeAllForUser revokes every API key and MCP connection of the user and returns how many keys it revoked.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) (int64, error)
}

// ForgotQueue hands a password-reset request to the worker, so the HTTP request does the same
// work for every address (validate, enqueue, answer) and its timing reveals nothing.
type ForgotQueue interface {
	EnqueueForgot(ctx context.Context, email string) error
}

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}

// ClientInfo describes the caller for session metadata and audit.
type ClientInfo struct {
	UserAgent string
	IP        string
	RequestID string
}

// IssuedSession is returned to the transport layer to set cookies.
type IssuedSession struct {
	Token     string // raw token: cookie value only
	CSRFToken string
	ExpiresAt time.Time
}
