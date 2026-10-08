package accounts

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/socialaccount"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// StateTTL is how long an OAuth state is valid.
const StateTTL = 10 * time.Minute

// DefaultRedirect is where the browser returns after connecting.
const DefaultRedirect = "/accounts"

// SafeRedirectPath allow-lists post-connect redirects to same-site relative paths.
func SafeRedirectPath(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") ||
		strings.ContainsAny(p, "\\\r\n\t") || strings.Contains(p, "://") || len(p) > 512 {
		return DefaultRedirect
	}
	return p
}

// CallbackURL returns the redirect_uri registered with the provider.
func (s *Service) CallbackURL(providerName string) string {
	return strings.TrimRight(s.redirectBaseURL, "/") + "/api/v1/social/" + providerName + "/callback"
}

// loginHintRe bounds the optional account hint forwarded to providers.
var loginHintRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// BeginOAuth creates a state + PKCE verifier and returns the provider consent URL.
func (s *Service) BeginOAuth(ctx context.Context, a actor.Actor, providerName, redirect, loginHint string) (string, error) {
	if err := a.RequireSession(); err != nil {
		return "", err
	}
	if err := a.RequireVerified(); err != nil {
		return "", err
	}
	if loginHint != "" && !loginHintRe.MatchString(loginHint) {
		return "", errs.Validationf("account hint must be 1-32 characters of a-z, 0-9, _ or -")
	}
	_, oauth, err := s.registry.OAuth(providerName)
	if err != nil {
		return "", err
	}
	state, err := crypto.RandomToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := crypto.RandomToken(48)
	if err != nil {
		return "", err
	}
	verifierEnc, err := s.enc.Encrypt(verifier)
	if err != nil {
		return "", err
	}
	st := &OAuthState{
		UserID: a.UserID, Provider: providerName, StateHash: crypto.SHA256Hex(state),
		CodeVerifierEnc: verifierEnc, RedirectAfter: SafeRedirectPath(redirect),
		ExpiresAt: s.clock.Now().Add(StateTTL),
	}
	if err := s.states.Create(ctx, st); err != nil {
		return "", err
	}
	return oauth.AuthorizeURL(provider.AuthorizeParams{
		State: state, CodeChallenge: crypto.PKCEChallenge(verifier), RedirectURI: s.CallbackURL(providerName),
		LoginHint: loginHint,
	}), nil
}

// CompleteOAuth validates state, exchanges the code, fetches the profile and
// stores the account with encrypted tokens. Returns the post-connect redirect path.
func (s *Service) CompleteOAuth(ctx context.Context, a actor.Actor, providerName, code, state string) (string, *socialaccount.Account, error) {
	if err := a.RequireSession(); err != nil {
		return "", nil, err
	}
	if code == "" || state == "" || len(state) > 256 {
		return "", nil, errs.Validationf("missing code or state")
	}
	p, oauth, err := s.registry.OAuth(providerName)
	if err != nil {
		return "", nil, err
	}
	st, err := s.states.Consume(ctx, a.UserID, providerName, crypto.SHA256Hex(state), s.clock.Now())
	if errs.Is(err, errs.NotFound) {
		return "", nil, errs.New(errs.Validation, "invalid or expired OAuth state")
	}
	if err != nil {
		return "", nil, err
	}
	verifier, err := s.enc.Decrypt(st.CodeVerifierEnc)
	if err != nil {
		return "", nil, errs.Wrap(errs.Internal, "cannot read OAuth state", err)
	}
	tok, err := oauth.Exchange(ctx, code, verifier, s.CallbackURL(providerName))
	if err != nil {
		return st.RedirectAfter, nil, providerFailure(err)
	}
	prof, err := oauth.Profile(ctx, tok.AccessToken)
	if err != nil {
		return st.RedirectAfter, nil, providerFailure(err)
	}
	acc := accountFromProfile(a, p.Name(), prof, tok.Scopes, s.clock.Now())
	if err := s.connectAccount(ctx, a, acc, &tok); err != nil {
		return st.RedirectAfter, nil, err
	}
	return st.RedirectAfter, acc, nil
}

func accountFromProfile(a actor.Actor, providerName string, prof provider.Profile, scopes []string, now time.Time) *socialaccount.Account {
	meta := prof.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	if scopes == nil {
		scopes = []string{}
	}
	return &socialaccount.Account{
		UserID: a.UserID, Provider: providerName, ProviderAccountID: prof.ID, Username: prof.Username,
		DisplayName: prof.DisplayName, AvatarURL: prof.AvatarURL, Scopes: scopes, Metadata: meta,
		Status: socialaccount.StatusActive, ConnectedAt: now,
	}
}

// providerFailure maps provider errors to API errors without leaking details.
func providerFailure(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	switch provider.Classify(err) {
	case provider.KindAuth:
		return errs.Wrap(errs.SocialAccountExpired, provider.SafeMessage(err), err)
	case provider.KindPermanent:
		return errs.Wrap(errs.Validation, provider.SafeMessage(err), err)
	case provider.KindUnsupported:
		return errs.Wrap(errs.ProviderNotAvailable, provider.SafeMessage(err), err)
	default:
		return errs.Wrap(errs.ProviderError, provider.SafeMessage(err), err)
	}
}
