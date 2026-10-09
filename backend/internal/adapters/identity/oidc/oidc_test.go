package oidc_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/identity/oidc"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/testutil/oidcfake"
)

const (
	verifier    = "verifier-0123456789012345678901234567890123456789"
	nonce       = "nonce-abc"
	redirectURI = "https://api.example.test/api/v1/auth/oauth/google/callback"
)

func setup(t *testing.T) (*oidcfake.Fake, *oidc.Adapter) {
	t.Helper()
	f := oidcfake.New(t)
	a, err := oidc.New(oidc.ProviderConfig{ID: identity.Google, Issuer: f.Issuer(), AuthURL: f.AuthURL(), TokenURL: f.TokenURL(),
		JWKSURL: f.JWKSURL(), ClientID: f.ClientID, ClientSecret: f.ClientSecret})
	if err != nil {
		t.Fatal(err)
	}
	return f, a
}

func grant() oidcfake.Grant {
	return oidcfake.Grant{Subject: "sub-1", Email: " Ann@Gmail.com ", EmailVerified: true, Name: " Ann ", Nonce: nonce,
		CodeChallenge: s256(verifier), RedirectURI: redirectURI}
}

func s256(v string) string { return crypto.PKCEChallenge(v) }

func exchange(a *oidc.Adapter, code string) (identity.Claims, error) {
	return a.Exchange(context.Background(), auth.ExchangeRequest{Code: code, CodeVerifier: verifier, RedirectURI: redirectURI,
		NonceHash: crypto.SHA256Hex(nonce)})
}

func TestExchangeReturnsVerifiedClaims(t *testing.T) {
	f, a := setup(t)
	g := grant()
	g.HostedDomain = "corp.example"
	c, err := exchange(a, f.Code(g))
	if err != nil {
		t.Fatal(err)
	}
	want := identity.Claims{Provider: identity.Google, Subject: "sub-1", Email: "ann@gmail.com", EmailVerified: true,
		EmailPrimary: true, HostedDomain: "corp.example", DisplayName: "Ann"}
	if c != want {
		t.Fatalf("got %+v want %+v", c, want)
	}
}

func TestEmailVerifiedAsStringIsAccepted(t *testing.T) {
	f, a := setup(t)
	g := grant()
	g.EmailVerifiedAsString = true
	if c, err := exchange(a, f.Code(g)); err != nil || !c.EmailVerified {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestExchangeRejectsBadTokens(t *testing.T) {
	for _, fault := range []oidcfake.Fault{
		oidcfake.FaultWrongAudience, oidcfake.FaultWrongIssuer, oidcfake.FaultExpired, oidcfake.FaultAlgNone,
		oidcfake.FaultHS256PubKey, oidcfake.FaultWrongNonce, oidcfake.FaultNoNonce, oidcfake.FaultUnknownKey,
		oidcfake.FaultBadSignature, oidcfake.FaultNoIDToken, oidcfake.FaultNoSubject, oidcfake.FaultTwoAudiences,
		oidcfake.FaultTokenEndpoint,
	} {
		t.Run(string(fault), func(t *testing.T) {
			f, a := setup(t)
			g := grant()
			g.Fault = fault
			c, err := exchange(a, f.Code(g))
			if !errs.Is(err, errs.ProviderError) || c != (identity.Claims{}) {
				t.Fatalf("want PROVIDER_ERROR and no claims, got %+v %v", c, err)
			}
			if strings.Contains(err.(*errs.Error).Message, "nonce") || strings.Contains(err.(*errs.Error).Message, "token") {
				t.Fatalf("client message leaks details: %q", err.(*errs.Error).Message)
			}
		})
	}
}

func TestRequestNonceMustMatch(t *testing.T) {
	f, a := setup(t)
	_, err := a.Exchange(context.Background(), auth.ExchangeRequest{Code: f.Code(grant()), CodeVerifier: verifier,
		RedirectURI: redirectURI, NonceHash: crypto.SHA256Hex("some other nonce")})
	if !errs.Is(err, errs.ProviderError) {
		t.Fatalf("a token for another nonce was accepted: %v", err)
	}
}

func TestPKCEAndCodeSingleUse(t *testing.T) {
	f, a := setup(t)
	_, err := a.Exchange(context.Background(), auth.ExchangeRequest{Code: f.Code(grant()), CodeVerifier: "a-different-verifier-0123456789012345678901",
		RedirectURI: redirectURI, NonceHash: crypto.SHA256Hex(nonce)})
	if !errs.Is(err, errs.ProviderError) {
		t.Fatalf("wrong PKCE verifier accepted: %v", err)
	}
	code := f.Code(grant())
	if _, err := exchange(a, code); err != nil {
		t.Fatal(err)
	}
	if _, err := exchange(a, code); !errs.Is(err, errs.ProviderError) {
		t.Fatalf("code redeemed twice: %v", err)
	}
	if _, err := exchange(a, "never-issued"); !errs.Is(err, errs.ProviderError) {
		t.Fatalf("unknown code accepted: %v", err)
	}
}

func TestKeyRotationIsFollowed(t *testing.T) {
	f, a := setup(t)
	if _, err := exchange(a, f.Code(grant())); err != nil {
		t.Fatal(err)
	}
	f.Rotate() // the provider starts signing with a new key; the cached key set does not know it yet
	if _, err := exchange(a, f.Code(grant())); err != nil {
		t.Fatalf("token signed with a rotated key rejected: %v", err)
	}
	if f.KeyFetches < 2 {
		t.Fatalf("key set was not refetched (%d fetches)", f.KeyFetches)
	}
}

func TestExpiryUsesTheInjectedClock(t *testing.T) {
	f := oidcfake.New(t)
	a, err := oidc.New(oidc.ProviderConfig{ID: identity.Google, Issuer: f.Issuer(), AuthURL: f.AuthURL(), TokenURL: f.TokenURL(),
		JWKSURL: f.JWKSURL(), ClientID: f.ClientID, ClientSecret: f.ClientSecret, Now: func() time.Time { return time.Now().Add(3 * time.Hour) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exchange(a, f.Code(grant())); !errs.Is(err, errs.ProviderError) {
		t.Fatalf("a token past its expiry was accepted: %v", err)
	}
}

func TestAuthorizeURL(t *testing.T) {
	_, a := setup(t)
	raw := a.AuthorizeURL(auth.AuthorizeRequest{State: "st", Nonce: nonce, CodeVerifier: verifier, RedirectURI: redirectURI})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for k, want := range map[string]string{"response_type": "code", "client_id": "fake-client-id", "state": "st", "nonce": nonce,
		"redirect_uri": redirectURI, "code_challenge": s256(verifier), "code_challenge_method": "S256", "scope": "openid email profile"} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if strings.Contains(raw, verifier) || strings.Contains(raw, "secret") {
		t.Fatal("verifier or secret in the URL")
	}
	if a.ID() != identity.Google {
		t.Fatal("id")
	}
}

func TestNewValidatesConfig(t *testing.T) {
	good := oidc.Google("id", "secret")
	if _, err := oidc.New(good); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*oidc.ProviderConfig){
		"no secret": func(c *oidc.ProviderConfig) { c.ClientSecret = "" },
		"no jwks":   func(c *oidc.ProviderConfig) { c.JWKSURL = "" },
		"bad id":    func(c *oidc.ProviderConfig) { c.ID = "myspace" },
		"alg none":  func(c *oidc.ProviderConfig) { c.SigningAlgs = []string{"RS256", "none"} },
	} {
		c := good
		mut(&c)
		if _, err := oidc.New(c); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
