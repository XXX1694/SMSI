package auth

import (
	"context"
	"crypto/subtle"
	"net/url"
	"strings"
	"time"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/domain/redirect"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// Where the browser lands, and how long a flow or a sign-up ticket lives.
const (
	// DefaultLoginRedirect is where a finished sign-in goes when the start request named no safe `next`.
	DefaultLoginRedirect = "/dashboard"
	// SignupCompletePath is the web page where a new social user accepts the Terms.
	SignupCompletePath = "/signup/complete"
	// FlowTTL bounds the round trip to the provider; TicketTTL bounds the Terms form after it.
	FlowTTL   = 10 * time.Minute
	TicketTTL = 10 * time.Minute
)

// Callback failure codes. They double as the `?error=` value on /login and are the only thing a failed flow tells the
// browser: never provider text, never an address.
const (
	ErrCancelled        = "oauth_cancelled"
	ErrStateInvalid     = "oauth_state_invalid"
	ErrProviderError    = "oauth_provider_error"
	ErrEmailUnverified  = string(identity.ReasonEmailUnverified)
	ErrAccountExists    = string(identity.ReasonAccountExists)
	ErrAccountGone      = string(identity.ReasonAccountUnavailable)
	maxCallbackParamLen = 2048
)

// SocialDeps enables sign-in with external providers (D-023).
type SocialDeps struct {
	Identities Identities
	Flows      OAuthFlows
	// Providers are the configured providers; one that is not listed is off.
	Providers []IdentityProvider
	// Cipher encrypts the PKCE verifier at rest with the same key as provider credentials.
	Cipher port.Encryptor
	// APIPublicURL builds the redirect URI sent to providers. It comes from configuration only, never from a request's
	// Host header, so a forged Host cannot make a provider redirect somewhere else.
	APIPublicURL string
}

type socialState struct {
	identities Identities
	flows      OAuthFlows
	providers  map[identity.Provider]IdentityProvider
	order      []identity.Provider
	cipher     port.Encryptor
	apiURL     string
}

func newSocialState(d SocialDeps) (*socialState, error) {
	if d.Identities == nil || d.Flows == nil || d.Cipher == nil || d.APIPublicURL == "" {
		return nil, errs.New(errs.Internal, "auth: social sign-in needs Identities, Flows, Cipher and APIPublicURL")
	}
	st := &socialState{identities: d.Identities, flows: d.Flows, cipher: d.Cipher, apiURL: strings.TrimRight(d.APIPublicURL, "/"),
		providers: map[identity.Provider]IdentityProvider{}}
	for _, p := range d.Providers {
		if !p.ID().Valid() {
			return nil, errs.Newf(errs.Internal, "auth: unknown sign-in provider %q", p.ID())
		}
		st.providers[p.ID()] = p
	}
	for _, id := range []identity.Provider{identity.Google, identity.GitHub} { // the order buttons appear in
		if _, ok := st.providers[id]; ok {
			st.order = append(st.order, id)
		}
	}
	return st, nil
}

// ProviderInfo is a provider the user can sign in with.
type ProviderInfo struct {
	ID   identity.Provider
	Name string
}

var providerNames = map[identity.Provider]string{identity.Google: "Google", identity.GitHub: "GitHub"}

// SocialProviders lists the enabled sign-in providers.
func (s *Service) SocialProviders() []ProviderInfo {
	out := []ProviderInfo{}
	if s.social != nil {
		for _, id := range s.social.order {
			out = append(out, ProviderInfo{ID: id, Name: providerNames[id]})
		}
	}
	return out
}

func (s *Service) socialProvider(id identity.Provider) (IdentityProvider, error) {
	if s.social != nil {
		if p, ok := s.social.providers[id]; ok {
			return p, nil
		}
	}
	return nil, errs.NotFoundf("sign-in provider")
}

func (s *socialState) redirectURI(id identity.Provider) string {
	return s.apiURL + "/api/v1/auth/oauth/" + string(id) + "/callback"
}

// StartResult is where to send the browser, and the raw state the transport must keep in a cookie.
type StartResult struct {
	URL   string
	State string
}

// StartSocial begins a sign-in: it stores the hashed state and nonce and the encrypted PKCE verifier, and returns the
// provider's consent URL. A provider that is unknown or not configured is NOT_FOUND.
func (s *Service) StartSocial(ctx context.Context, id identity.Provider, next string) (StartResult, error) {
	p, err := s.socialProvider(id)
	if err != nil {
		return StartResult{}, err
	}
	state, err := crypto.RandomToken(32)
	if err != nil {
		return StartResult{}, err
	}
	nonce, err := crypto.RandomToken(32)
	if err != nil {
		return StartResult{}, err
	}
	verifier, err := crypto.RandomToken(48)
	if err != nil {
		return StartResult{}, err
	}
	verifierEnc, err := s.social.cipher.Encrypt(verifier)
	if err != nil {
		return StartResult{}, err
	}
	f := &identity.Flow{Provider: id, Intent: identity.IntentLogin, StateHash: crypto.SHA256Hex(state), NonceHash: crypto.SHA256Hex(nonce),
		CodeVerifierEnc: verifierEnc, RedirectAfter: redirect.SafePath(next, DefaultLoginRedirect), ExpiresAt: s.clock.Now().Add(FlowTTL)}
	if err := s.social.flows.Create(ctx, f); err != nil {
		return StartResult{}, err
	}
	u := p.AuthorizeURL(AuthorizeRequest{State: state, Nonce: nonce, CodeVerifier: verifier, RedirectURI: s.social.redirectURI(id)})
	return StartResult{URL: u, State: state}, nil
}

// CallbackInput is what the provider's redirect carried, plus the browser's state cookie.
type CallbackInput struct {
	Code, State, CookieState string
	// ProviderError is the provider's `error` parameter (access_denied when the user said no).
	ProviderError string
}

// CallbackResult is what the transport does next. A callback never fails with an error body: it is a browser
// navigation, so every outcome is a redirect to the web app.
type CallbackResult struct {
	// Redirect is an in-app path: the user's `next`, the sign-up page, or /login?error=<code>.
	Redirect string
	// Session is set when the user is signed in.
	Session *IssuedSession
	// Ticket is the raw sign-up ticket for the cookie when a new user must accept the Terms.
	Ticket string
	// StateSpent is true once the flow's state was consumed: only then does the state cookie belong to a finished flow.
	// A callback that fails the state checks must leave the cookie alone, or anyone who can make a browser load a
	// callback URL could break the sign-in that browser has under way.
	StateSpent bool
}

func failed(code string) CallbackResult {
	return CallbackResult{Redirect: "/login?error=" + url.QueryEscape(code)}
}

// SocialCallback finishes a sign-in. The state must match three things at once: the query, the browser's cookie, and
// an unused, unexpired row of this provider. Nothing is looked up from the provider before that holds.
func (s *Service) SocialCallback(ctx context.Context, id identity.Provider, in CallbackInput, ci ClientInfo) CallbackResult {
	p, err := s.socialProvider(id)
	if err != nil {
		return failed(ErrProviderError)
	}
	if in.State == "" || len(in.State) > 256 || len(in.Code) > maxCallbackParamLen ||
		subtle.ConstantTimeCompare([]byte(in.State), []byte(in.CookieState)) != 1 {
		return failed(ErrStateInvalid)
	}
	fl, err := s.social.flows.ConsumeState(ctx, id, crypto.SHA256Hex(in.State), s.clock.Now())
	if errs.Is(err, errs.NotFound) || (err == nil && fl.Intent != identity.IntentLogin) {
		return failed(ErrStateInvalid)
	}
	if err != nil {
		s.logSocial(ctx, id, "state lookup failed", err)
		return failed(ErrProviderError)
	}
	res := s.finishFlow(ctx, p, fl, in, ci)
	res.StateSpent = true
	return res
}

// finishFlow runs after the state was consumed: whatever happens now ends the flow.
func (s *Service) finishFlow(ctx context.Context, p IdentityProvider, fl *identity.Flow, in CallbackInput, ci ClientInfo) CallbackResult {
	id := fl.Provider
	if in.ProviderError != "" {
		// The user declined (or the provider refused).
		if in.ProviderError == "access_denied" {
			return failed(ErrCancelled)
		}
		return failed(ErrProviderError)
	}
	if in.Code == "" {
		return failed(ErrStateInvalid)
	}
	claims, err := s.exchange(ctx, p, fl, in.Code)
	if err != nil {
		s.logSocial(ctx, id, "code exchange failed", err)
		return failed(ErrProviderError)
	}
	// A lost race with a concurrent sign-in (the unique constraints stop the loser) ends like any other failure.
	res, err := s.decide(ctx, fl, claims, ci)
	if err != nil {
		s.logSocial(ctx, id, "sign-in failed", err)
		return failed(ErrProviderError)
	}
	return res
}

func (s *Service) exchange(ctx context.Context, p IdentityProvider, fl *identity.Flow, code string) (identity.Claims, error) {
	verifier, err := s.social.cipher.Decrypt(fl.CodeVerifierEnc)
	if err != nil {
		return identity.Claims{}, errs.Wrap(errs.Internal, "cannot read the PKCE verifier", err)
	}
	return p.Exchange(ctx, ExchangeRequest{Code: code, CodeVerifier: verifier, NonceHash: fl.NonceHash, RedirectURI: s.social.redirectURI(fl.Provider)})
}

// PurgeExpiredOAuthFlows removes flows whose state and ticket have both expired (called by the worker).
func (s *Service) PurgeExpiredOAuthFlows(ctx context.Context) (int64, error) {
	if s.social == nil {
		return 0, nil
	}
	return s.social.flows.DeleteExpired(ctx, s.clock.Now())
}
