// Package socialaccount defines connected social network accounts.
package socialaccount

import (
	"time"

	"github.com/google/uuid"
)

// Status of a connected account.
type Status string

const (
	StatusActive  Status = "active"
	StatusExpired Status = "expired"
	StatusRevoked Status = "revoked"
	StatusError   Status = "error"
)

// Account is a connected social account (no secrets).
type Account struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	Provider          string
	ProviderAccountID string
	Username          string
	DisplayName       string
	AvatarURL         string
	Scopes            []string
	Metadata          map[string]any
	Status            Status
	ConnectedAt       time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Credentials are decrypted OAuth tokens (kept in memory only).
type Credentials struct {
	AccessToken      string
	RefreshToken     string
	ExpiresAt        *time.Time
	RefreshExpiresAt *time.Time
}

// NeedsRefresh reports whether the access token expires within the window.
func (c Credentials) NeedsRefresh(now time.Time, window time.Duration) bool {
	return c.ExpiresAt != nil && c.ExpiresAt.Before(now.Add(window))
}

// CanRefresh reports whether a usable refresh token exists.
func (c Credentials) CanRefresh(now time.Time) bool {
	if c.RefreshToken == "" {
		return false
	}
	return c.RefreshExpiresAt == nil || c.RefreshExpiresAt.After(now)
}

// Publishable reports whether the account may be used for publishing.
func (a *Account) Publishable() bool { return a.Status == StatusActive }
