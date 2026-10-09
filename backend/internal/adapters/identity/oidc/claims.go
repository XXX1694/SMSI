package oidc

import (
	"encoding/json"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/socialos/backend/internal/domain/identity"
)

// lenientBool reads a JSON boolean that some providers send as the string "true" or "false".
type lenientBool bool

func (b *lenientBool) UnmarshalJSON(raw []byte) error {
	switch strings.Trim(string(raw), `"`) {
	case "true":
		*b = true
	case "false", "null":
		*b = false
	default:
		return json.Unmarshal(raw, (*bool)(b)) // reports a proper error
	}
	return nil
}

func (a *Adapter) claims(idt *gooidc.IDToken) (identity.Claims, error) {
	var c struct {
		Email           string      `json:"email"`
		EmailVerified   lenientBool `json:"email_verified"`
		HostedDomain    string      `json:"hd"`
		Name            string      `json:"name"`
		AuthorizedParty string      `json:"azp"`
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
		Provider: a.cfg.ID, Subject: idt.Subject,
		Email: strings.ToLower(strings.TrimSpace(c.Email)), EmailVerified: bool(c.EmailVerified),
		// An OIDC token carries exactly one address, which is the account's own.
		EmailPrimary: true, HostedDomain: c.HostedDomain, DisplayName: strings.TrimSpace(c.Name),
	}, nil
}
