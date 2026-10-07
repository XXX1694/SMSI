// Package accounts connects, lists and disconnects social accounts and
// manages their encrypted OAuth credentials.
package accounts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// EncryptedCredentials is the stored (ciphertext) form of OAuth tokens.
type EncryptedCredentials struct {
	AccessTokenEnc   string
	RefreshTokenEnc  string
	ExpiresAt        *time.Time
	RefreshExpiresAt *time.Time
	KeyVersion       string
}

// Repo persists social accounts and credentials. Every method is tenant-scoped
// except credential methods, which are keyed by an account id the caller
// already resolved through a tenant-scoped read.
type Repo interface {
	Upsert(ctx context.Context, a *socialaccount.Account) error
	List(ctx context.Context, userID uuid.UUID) ([]socialaccount.Account, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*socialaccount.Account, error)
	SetStatus(ctx context.Context, userID, id uuid.UUID, st socialaccount.Status) error
	SaveCredentials(ctx context.Context, accountID uuid.UUID, c EncryptedCredentials) error
	GetCredentials(ctx context.Context, accountID uuid.UUID) (*EncryptedCredentials, error)
	DeleteCredentials(ctx context.Context, accountID uuid.UUID) error
}

// OAuthState is a pending authorization (state stored as SHA-256 hash).
type OAuthState struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Provider        string
	StateHash       string
	CodeVerifierEnc string
	RedirectAfter   string
	ExpiresAt       time.Time
}

// States persists OAuth states.
type States interface {
	Create(ctx context.Context, s *OAuthState) error
	// Consume atomically marks the state used; returns NOT_FOUND when it is
	// unknown, already used, expired, for another provider or another user.
	Consume(ctx context.Context, userID uuid.UUID, provider, stateHash string, now time.Time) (*OAuthState, error)
}
