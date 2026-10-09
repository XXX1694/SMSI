// Package identity defines external sign-in identities (Google, GitHub) and the rules for linking them to users.
package identity

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Provider is an external identity provider that can sign users in.
type Provider string

const (
	Google Provider = "google"
	GitHub Provider = "github"
)

// Valid reports whether p is a provider this build knows.
func (p Provider) Valid() bool { return p == Google || p == GitHub }

// Identity links one external account to one user.
type Identity struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// Subject is the provider's stable account id (Google "sub", GitHub numeric id). Never an email or a login name.
	Subject       string
	Provider      Provider
	Email         string
	EmailVerified bool
	LinkedAt      time.Time
	LastLoginAt   *time.Time
}

// GoogleIssuer is the only issuer whose gmail.com and hd rules we trust. Any other OIDC issuer (Keycloak, Authentik)
// must get its own Provider id and is never authoritative.
const GoogleIssuer = "https://accounts.google.com"

// Claims is what a provider vouches for after a successful sign-in, already reduced to the facts the linking rules need.
type Claims struct {
	Provider Provider
	// Issuer is the ID token issuer (OIDC providers); empty for GitHub.
	Issuer  string
	Subject string
	// Email is lower-cased; empty when the provider gave none.
	Email string
	// EmailVerified is the provider's own statement that the user controls Email.
	EmailVerified bool
	// EmailPrimary is true when Email is the account's primary address. GitHub lists several addresses; Google has one.
	EmailPrimary bool
	// HostedDomain is Google's "hd" claim: set only for Google Workspace accounts.
	HostedDomain string
	DisplayName  string
}

// AuthoritativeEmail reports whether the provider is the authority for Email, so that a match with a local account
// proves the same person owns both. A merely verified address is not enough: an address on a custom domain can
// change hands while the old Google account still carries a verified flag.
//
//   - Google (issuer accounts.google.com only): a gmail.com address (Google is the only one who can issue it), or a verified address inside a
//     Workspace domain (the "hd" claim), per Google's own guidance for ID tokens.
//   - GitHub: the primary address, and verified.
func (c Claims) AuthoritativeEmail() bool {
	if c.Email == "" || !c.EmailVerified {
		return false
	}
	switch c.Provider {
	case Google:
		return c.Issuer == GoogleIssuer && (strings.HasSuffix(strings.ToLower(c.Email), "@gmail.com") || c.HostedDomain != "")
	case GitHub:
		return c.EmailPrimary
	default:
		return false
	}
}
