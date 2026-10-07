// Package auth implements registration, login, sessions and API-key authentication.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/apikey"
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
}

// Sessions persists sessions.
type Sessions interface {
	Create(ctx context.Context, s *Session) error
	GetByTokenHash(ctx context.Context, hash string) (*Session, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// APIKeys is the read side needed for authentication.
type APIKeys interface {
	GetByHash(ctx context.Context, hash string) (*apikey.Key, error)
	// TouchLastUsed updates last_used_at (and the MCP connection's last_seen_at), throttled.
	TouchLastUsed(ctx context.Context, keyID uuid.UUID, now time.Time) error
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
