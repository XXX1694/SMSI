// Package oidc is the sign-in adapter for OpenID Connect providers (Google today; Keycloak or Authentik through
// configuration later). It runs Authorization Code + PKCE and verifies the ID token with coreos/go-oidc.
//
// go-oidc checks the signature against the provider's keys (fetched and cached, refetched on an unknown key id),
// the algorithm against an allow-list, issuer, audience and expiry. It does not check the nonce; this package does.
package oidc

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// signingAlgs is the allow-list for the ID token's alg. Only RS256 (what Google signs with): "none" and the HMAC
// family are never acceptable, and a provider that needs another algorithm gets that code reviewed here.
var signingAlgs = []string{"RS256"}

// requestTimeout bounds every call to the provider (token endpoint, key set).
const requestTimeout = 10 * time.Second

// ProviderConfig is a static description of the provider, so starting the API never depends on reaching it.
type ProviderConfig struct {
	ID           identity.Provider
	Issuer       string
	AuthURL      string
	TokenURL     string
	JWKSURL      string
	ClientID     string
	ClientSecret string
	// HTTPClient is used for the token and key requests; it defaults to a client with a 10 s timeout.
	HTTPClient *http.Client
	// Now is the clock for expiry checks; it defaults to time.Now.
	Now func() time.Time
}

// Google returns the static configuration of Google Sign-In. Only tokens from this issuer get the gmail.com and hd
// rules (identity.Claims.AuthoritativeEmail).
func Google(clientID, clientSecret string) ProviderConfig {
	return ProviderConfig{
		ID: identity.Google, Issuer: "https://accounts.google.com",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		JWKSURL:  "https://www.googleapis.com/oauth2/v3/certs",
		ClientID: clientID, ClientSecret: clientSecret,
	}
}

// Adapter implements auth.IdentityProvider for one OIDC provider.
type Adapter struct {
	cfg      ProviderConfig
	oauth    *oauth2.Config
	verifier *gooidc.IDTokenVerifier
	client   *http.Client
}

var _ auth.IdentityProvider = (*Adapter)(nil)

// New builds the adapter. It makes no network call.
func New(cfg ProviderConfig) (*Adapter, error) {
	if !cfg.ID.Valid() || cfg.Issuer == "" || cfg.AuthURL == "" || cfg.TokenURL == "" || cfg.JWKSURL == "" ||
		cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errs.New(errs.Internal, "oidc: provider id, endpoints and client credentials are required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	// The key set keeps this context for its later fetches, so it must not be a request's.
	long := gooidc.ClientContext(context.Background(), client)
	provider := (&gooidc.ProviderConfig{IssuerURL: cfg.Issuer, AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL,
		JWKSURL: cfg.JWKSURL, Algorithms: signingAlgs}).NewProvider(long)
	verifier := provider.VerifierContext(long, &gooidc.Config{ClientID: cfg.ClientID, SupportedSigningAlgs: signingAlgs, Now: cfg.Now})
	return &Adapter{cfg: cfg, client: client, verifier: verifier, oauth: &oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Scopes: []string{gooidc.ScopeOpenID, "email", "profile"},
		Endpoint: oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}}, nil
}

// ID names the provider.
func (a *Adapter) ID() identity.Provider { return a.cfg.ID }

// AuthorizeURL builds the consent URL with PKCE (S256) and the nonce.
func (a *Adapter) AuthorizeURL(r auth.AuthorizeRequest) string {
	c := *a.oauth
	c.RedirectURL = r.RedirectURI
	return c.AuthCodeURL(r.State, oauth2.S256ChallengeOption(r.CodeVerifier), gooidc.Nonce(r.Nonce))
}

// Exchange redeems the code and returns the verified claims of the ID token.
func (a *Adapter) Exchange(ctx context.Context, r auth.ExchangeRequest) (identity.Claims, error) {
	ctx, cancel := context.WithTimeout(gooidc.ClientContext(ctx, a.client), requestTimeout)
	defer cancel()
	c := *a.oauth
	c.RedirectURL = r.RedirectURI
	tok, err := c.Exchange(ctx, r.Code, oauth2.VerifierOption(r.CodeVerifier))
	if err != nil {
		return identity.Claims{}, rejected("token exchange failed", err)
	}
	// The access token is not needed: identity comes from the signed ID token alone, and nothing is stored.
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return identity.Claims{}, rejected("response has no id_token", nil)
	}
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return identity.Claims{}, rejected("id token rejected", err)
	}
	if idt.Nonce == "" || subtle.ConstantTimeCompare([]byte(crypto.SHA256Hex(idt.Nonce)), []byte(r.NonceHash)) != 1 {
		return identity.Claims{}, rejected("id token nonce does not match", nil)
	}
	return a.claims(idt)
}

func rejected(msg string, cause error) error {
	detail := fmt.Errorf("oidc: %s", msg)
	if cause != nil {
		detail = fmt.Errorf("oidc: %s: %w", msg, cause)
	}
	return errs.Wrap(errs.ProviderError, "the sign-in provider could not confirm your identity", detail)
}
