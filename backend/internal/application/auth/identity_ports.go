package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/identity"
)

// Identities persists the external accounts linked to users.
type Identities interface {
	// Create links an identity. CONFLICT when the (provider, subject) belongs to any user, or the user already has an
	// identity for that provider.
	Create(ctx context.Context, i *identity.Identity) error
	// GetBySubject is the sign-in lookup. It is deliberately not scoped to a user: the identity decides who signs in.
	// NOT_FOUND when nobody has linked it.
	GetBySubject(ctx context.Context, p identity.Provider, subject string) (*identity.Identity, error)
	// ListByUser returns the user's identities, oldest first. The result is bounded by the number of providers.
	ListByUser(ctx context.Context, userID uuid.UUID) ([]identity.Identity, error)
	// Delete unlinks the user's identity for a provider. NOT_FOUND when the user has none (also for another user's).
	Delete(ctx context.Context, userID uuid.UUID, p identity.Provider) error
	// TouchLogin records a sign-in and refreshes the email the provider last reported.
	TouchLogin(ctx context.Context, userID uuid.UUID, p identity.Provider, email string, verified bool, at time.Time) error
}

// OAuthFlows persists sign-in and link attempts, then the pending sign-up tickets that follow them.
type OAuthFlows interface {
	Create(ctx context.Context, f *identity.Flow) error
	// ConsumeState atomically marks an unused, unexpired flow of the provider as used and returns it. Anything else
	// (unknown, replayed, expired, started for another provider) is NOT_FOUND, so a state works exactly once.
	ConsumeState(ctx context.Context, p identity.Provider, stateHash string, now time.Time) (*identity.Flow, error)
	// SetPending attaches a sign-up ticket to a consumed flow. NOT_FOUND when the flow is unknown or already has one.
	SetPending(ctx context.Context, id uuid.UUID, ticketHash string, expiresAt time.Time, p identity.PendingSignup) error
	// GetByTicket reads a live ticket without using it (the sign-up form shows what will be created).
	GetByTicket(ctx context.Context, ticketHash string, now time.Time) (*identity.Flow, error)
	// ConsumeTicket atomically redeems a live ticket: the row is deleted and returned, so it works exactly once.
	ConsumeTicket(ctx context.Context, ticketHash string, now time.Time) (*identity.Flow, error)
	// DeleteExpired removes flows whose state and ticket have both expired.
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// AuthorizeRequest is what a provider needs to build its consent URL.
type AuthorizeRequest struct {
	State string
	// Nonce is the raw nonce; only its hash is stored, and the ID token must echo it (OIDC providers).
	Nonce string
	// CodeVerifier is the raw PKCE verifier; the provider sends its S256 challenge.
	CodeVerifier string
	RedirectURI  string
}

// ExchangeRequest is what a provider needs to turn the callback's code into verified claims.
type ExchangeRequest struct {
	Code         string
	CodeVerifier string
	RedirectURI  string
	// NonceHash is hex SHA-256 of the nonce sent in AuthorizeRequest (crypto.SHA256Hex). OIDC providers compare it
	// to the ID token's nonce and reject a mismatch. Providers without ID tokens ignore it.
	NonceHash string
}

// IdentityProvider is one external sign-in provider (Google, GitHub). It talks to the provider and reduces the answer
// to Claims; whether those claims may sign anyone in is decided by identity.Decide.
type IdentityProvider interface {
	ID() identity.Provider
	AuthorizeURL(r AuthorizeRequest) string
	// Exchange redeems the code and verifies what the provider returned. Every failure is a PROVIDER_ERROR whose
	// cause (kept out of client messages) says what was wrong. The provider's access token is discarded.
	Exchange(ctx context.Context, r ExchangeRequest) (identity.Claims, error)
}
