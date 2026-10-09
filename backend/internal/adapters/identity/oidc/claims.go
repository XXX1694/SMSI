package oidc

import (
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/socialos/backend/internal/domain/identity"
)

func (a *Adapter) claims(idt *gooidc.IDToken) (identity.Claims, error) {
	var c struct {
		Email           string `json:"email"`
		EmailVerified   bool   `json:"email_verified"`
		HostedDomain    string `json:"hd"`
		Name            string `json:"name"`
		AuthorizedParty string `json:"azp"`
	}
	if err := idt.Claims(&c); err != nil {
		return identity.Claims{}, rejected("claims unreadable", err)
	}
	if idt.Subject == "" {
		return identity.Claims{}, rejected("id token has no subject", nil)
	}
	// With several audiences the token must say which party it was issued to, and that is us (OIDC Core 3.1.3.7).
	if len(idt.Audience) > 1 && c.AuthorizedParty != a.cfg.ClientID {
		return identity.Claims{}, rejected("id token authorized party is not this client", nil)
	}
	return identity.Claims{
		Provider: a.cfg.ID, Issuer: a.cfg.Issuer, Subject: idt.Subject,
		Email: strings.ToLower(strings.TrimSpace(c.Email)), EmailVerified: c.EmailVerified,
		// An OIDC token carries exactly one address, which is the account's own.
		EmailPrimary: true, HostedDomain: c.HostedDomain, DisplayName: strings.TrimSpace(c.Name),
	}, nil
}
