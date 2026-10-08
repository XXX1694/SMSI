// Package emailtoken defines the one-time tokens mailed to users for email
// verification and password reset.
package emailtoken

import (
	"time"

	"github.com/google/uuid"
)

// Purpose says what redeeming a token does. A token of one purpose never works for the other.
type Purpose string

const (
	VerifyEmail   Purpose = "verify_email"
	ResetPassword Purpose = "reset_password"
)

// Lifetimes and the resend cooldown.
const (
	VerifyTTL = 48 * time.Hour
	ResetTTL  = 30 * time.Minute
	// Cooldown is the minimum gap between two mails of the same purpose to one user.
	Cooldown = 60 * time.Second
)

// TTL returns the lifetime of a token of this purpose.
func (p Purpose) TTL() time.Duration {
	if p == ResetPassword {
		return ResetTTL
	}
	return VerifyTTL
}

// Token is the stored form of a mailed token. Only the SHA-256 of the raw
// token is kept; the raw value exists in the mail and nowhere else.
type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Purpose   Purpose
	Hash      string
	Email     string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
