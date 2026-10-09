package oidcfake

import (
	"encoding/json"
	"time"
)

// idToken returns the ID token for g: a valid one, or the one g.Fault describes.
func (f *Fake) idToken(g Grant) string {
	payload, err := json.Marshal(f.claims(g))
	if err != nil {
		f.t.Fatal(err)
	}
	return f.sign(g.Fault, payload)
}

// claims builds the claim set, with the claim-level faults applied. Signature-level faults are applied by sign.
func (f *Fake) claims(g Grant) map[string]any {
	now := f.Now()
	c := map[string]any{
		"iss": f.Issuer(), "aud": f.ClientID, "sub": g.Subject, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": g.Nonce, "email": g.Email, "email_verified": g.EmailVerified, "name": g.Name,
	}
	if g.HostedDomain != "" {
		c["hd"] = g.HostedDomain
	}
	switch g.Fault {
	case FaultWrongAudience:
		c["aud"] = "another-client"
	case FaultWrongIssuer:
		c["iss"] = "https://evil.example"
	case FaultExpired:
		c["iat"], c["exp"] = now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix()
	case FaultWrongNonce:
		c["nonce"] = "nonce-of-another-login"
	case FaultNoNonce:
		delete(c, "nonce")
	case FaultNoSubject:
		c["sub"] = ""
	case FaultTwoAudiences:
		c["aud"] = []string{f.ClientID, "another-client"}
	}
	return c
}
