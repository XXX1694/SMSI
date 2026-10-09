package identity

import (
	"time"

	"github.com/google/uuid"
)

// Intent is why a flow was started.
type Intent string

const (
	// IntentLogin signs in or signs up; there is no session yet.
	IntentLogin Intent = "login"
	// IntentLink attaches the identity to the signed-in user (Flow.LinkUserID).
	IntentLink Intent = "link"
)

// PendingSignup is what a provider vouched for, kept until the new user accepts the Terms.
type PendingSignup struct {
	Subject       string `json:"subject"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	// EmailAuthoritative is Claims.AuthoritativeEmail at callback time: the only moment the hd claim is known.
	EmailAuthoritative bool   `json:"email_authoritative"`
	DisplayName        string `json:"name"`
}

// Flow is one sign-in or link round trip. State, nonce and ticket are stored as hashes only.
type Flow struct {
	ID       uuid.UUID
	Provider Provider
	Intent   Intent
	// LinkUserID is set exactly when Intent is IntentLink.
	LinkUserID *uuid.UUID
	StateHash  string
	NonceHash  string
	// CodeVerifierEnc is the PKCE verifier, encrypted by the caller.
	CodeVerifierEnc string
	// RedirectAfter is an already-sanitised in-app path.
	RedirectAfter string
	ExpiresAt     time.Time
	UsedAt        *time.Time
	// TicketHash, TicketExpiresAt and Pending exist once the callback found a new user.
	TicketHash      string
	TicketExpiresAt *time.Time
	Pending         *PendingSignup
}
