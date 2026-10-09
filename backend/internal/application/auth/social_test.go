package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// flowsFake keeps the documented OAuthFlows semantics: a state works once, for its own provider, until it expires.
type flowsFake struct {
	mu       sync.Mutex
	rows     []*identity.Flow
	consumed int
}

func (f *flowsFake) Create(_ context.Context, fl *identity.Flow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl.ID = uuid.New()
	c := *fl
	f.rows = append(f.rows, &c)
	return nil
}

func (f *flowsFake) ConsumeState(_ context.Context, p identity.Provider, h string, now time.Time) (*identity.Flow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.StateHash == h && r.Provider == p && r.UsedAt == nil && now.Before(r.ExpiresAt) {
			r.UsedAt = &now
			f.consumed++
			c := *r
			return &c, nil
		}
	}
	return nil, errs.NotFoundf("oauth flow")
}

func (*flowsFake) SetPending(context.Context, uuid.UUID, string, time.Time, identity.PendingSignup) error {
	return nil
}
func (*flowsFake) GetByTicket(context.Context, string, time.Time) (*identity.Flow, error) {
	return nil, errs.NotFoundf("oauth flow")
}
func (*flowsFake) ConsumeTicket(context.Context, string, time.Time) (*identity.Flow, error) {
	return nil, errs.NotFoundf("oauth flow")
}
func (*flowsFake) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }

type identitiesFake struct{}

func (identitiesFake) Create(context.Context, *identity.Identity) error { return nil }
func (identitiesFake) GetBySubject(context.Context, identity.Provider, string) (*identity.Identity, error) {
	return nil, errs.NotFoundf("identity")
}
func (identitiesFake) ListByUser(context.Context, uuid.UUID) ([]identity.Identity, error) {
	return nil, nil
}
func (identitiesFake) Delete(context.Context, uuid.UUID, identity.Provider) error { return nil }
func (identitiesFake) TouchLogin(context.Context, uuid.UUID, identity.Provider, string, bool, time.Time) error {
	return nil
}

// providerFake records what it was asked and answers with fixed claims or an error.
type providerFake struct {
	id        identity.Provider
	authorize AuthorizeRequest
	exchanged []ExchangeRequest
	claims    identity.Claims
	err       error
}

func (p *providerFake) ID() identity.Provider { return p.id }
func (p *providerFake) AuthorizeURL(r AuthorizeRequest) string {
	p.authorize = r
	return "https://idp.test/authorize?state=" + url.QueryEscape(r.State) + "&redirect_uri=" + url.QueryEscape(r.RedirectURI)
}
func (p *providerFake) Exchange(_ context.Context, r ExchangeRequest) (identity.Claims, error) {
	p.exchanged = append(p.exchanged, r)
	return p.claims, p.err
}

// reversibleCipher is a stand-in for the AES-GCM cipher that is visibly not the identity function.
type reversibleCipher struct{}

func (reversibleCipher) Encrypt(s string) (string, error) { return "enc:" + s, nil }
func (reversibleCipher) Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, "enc:") {
		return "", errors.New("not ciphertext")
	}
	return strings.TrimPrefix(s, "enc:"), nil
}
func (reversibleCipher) KeyVersion() string { return "test" }

func socialRig(t *testing.T) (*rig, *flowsFake, *providerFake) {
	t.Helper()
	r := newRig(t, false)
	flows, gh := &flowsFake{}, &providerFake{id: identity.GitHub}
	svc, err := NewService(Deps{Users: r.users, Sessions: r.sessions, APIKeys: r.keys, Hasher: plainHasher{}, Tx: inlineTx{}, Audit: r.audit,
		Clock: r.clock, Tokens: r.tokens, Mail: r.mail, Forgot: r.forgotQ, WebBaseURL: "https://app.example/",
		Social: &SocialDeps{Identities: identitiesFake{}, Flows: flows, Providers: []IdentityProvider{gh}, Cipher: reversibleCipher{}, APIPublicURL: "https://api.example/"}})
	if err != nil {
		t.Fatal(err)
	}
	r.svc = svc
	return r, flows, gh
}

func TestStartSocialStoresHashesAndEncryptedVerifierAndBuildsTheRedirectFromConfig(t *testing.T) {
	r, flows, gh := socialRig(t)
	res, err := r.svc.StartSocial(context.Background(), identity.GitHub, "https://evil.test/")
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://api.example/api/v1/auth/oauth/github/callback"; gh.authorize.RedirectURI != want {
		t.Fatalf("redirect uri %q, want %q", gh.authorize.RedirectURI, want)
	}
	if len(flows.rows) != 1 {
		t.Fatalf("flows: %d", len(flows.rows))
	}
	f := flows.rows[0]
	if f.StateHash != crypto.SHA256Hex(res.State) || f.NonceHash != crypto.SHA256Hex(gh.authorize.Nonce) {
		t.Fatal("state and nonce are stored as hashes of the raw values")
	}
	if f.CodeVerifierEnc != "enc:"+gh.authorize.CodeVerifier || strings.Contains(f.CodeVerifierEnc, res.State) {
		t.Fatalf("the verifier must be stored encrypted: %q", f.CodeVerifierEnc)
	}
	if f.RedirectAfter != DefaultLoginRedirect {
		t.Fatalf("an off-site next must fall back to %s, got %q", DefaultLoginRedirect, f.RedirectAfter)
	}
	if !f.ExpiresAt.Equal(r.clock.now.Add(FlowTTL)) {
		t.Fatalf("expires at %s", f.ExpiresAt)
	}
	if _, err := r.svc.StartSocial(context.Background(), identity.Google, ""); !errs.Is(err, errs.NotFound) {
		t.Fatalf("a provider that is not configured: %v", err)
	}
}

func TestSocialCallbackChecksTheStateBeforeAnyProviderCall(t *testing.T) {
	r, flows, gh := socialRig(t)
	res, err := r.svc.StartSocial(context.Background(), identity.GitHub, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]CallbackInput{
		"no cookie":        {Code: "c", State: res.State},
		"other cookie":     {Code: "c", State: res.State, CookieState: "x"},
		"empty state":      {Code: "c"},
		"unknown state":    {Code: "c", State: "nope", CookieState: "nope"},
		"overlong state":   {Code: "c", State: strings.Repeat("a", 300), CookieState: strings.Repeat("a", 300)},
		"overlong code":    {Code: strings.Repeat("a", 3000), State: res.State, CookieState: res.State},
		"cancel w/o state": {ProviderError: "access_denied", State: "nope", CookieState: "nope"},
	} {
		got := r.svc.SocialCallback(context.Background(), identity.GitHub, in, ClientInfo{})
		if got.Redirect != "/login?error=oauth_state_invalid" || got.Session != nil || got.StateSpent {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	if len(gh.exchanged) != 0 || flows.consumed != 0 {
		t.Fatalf("the provider was called with an unproven state: %d", len(gh.exchanged))
	}
}

func TestSocialCallbackOutcomesAreRedirectsNeverErrors(t *testing.T) {
	r, _, gh := socialRig(t)
	start := func() StartResult {
		res, err := r.svc.StartSocial(context.Background(), identity.GitHub, "")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	s := start()
	if got := r.svc.SocialCallback(context.Background(), identity.GitHub, CallbackInput{State: s.State, CookieState: s.State, ProviderError: "access_denied"}, ClientInfo{}); got.Redirect != "/login?error=oauth_cancelled" || !got.StateSpent {
		t.Fatalf("cancel: %+v", got)
	}
	s = start()
	if got := r.svc.SocialCallback(context.Background(), identity.GitHub, CallbackInput{State: s.State, CookieState: s.State, ProviderError: "server_error"}, ClientInfo{}); got.Redirect != "/login?error=oauth_provider_error" {
		t.Fatalf("provider error: %+v", got)
	}
	s = start()
	gh.err = errs.Wrap(errs.ProviderError, "no", errors.New("secret detail"))
	got := r.svc.SocialCallback(context.Background(), identity.GitHub, CallbackInput{Code: "c", State: s.State, CookieState: s.State}, ClientInfo{})
	if got.Redirect != "/login?error=oauth_provider_error" || strings.Contains(got.Redirect, "secret") {
		t.Fatalf("exchange failure: %+v", got)
	}
	if len(gh.exchanged) != 1 || gh.exchanged[0].CodeVerifier != gh.authorize.CodeVerifier || gh.exchanged[0].NonceHash != crypto.SHA256Hex(gh.authorize.Nonce) ||
		gh.exchanged[0].RedirectURI != gh.authorize.RedirectURI {
		t.Fatalf("exchange request does not match the authorize request: %+v", gh.exchanged)
	}
}

func TestSocialProvidersListFollowsConfiguration(t *testing.T) {
	r, _, _ := socialRig(t)
	got := r.svc.SocialProviders()
	if len(got) != 1 || got[0].ID != identity.GitHub || got[0].Name != "GitHub" {
		t.Fatalf("%+v", got)
	}
	plain := newRig(t, false)
	if list := plain.svc.SocialProviders(); list == nil || len(list) != 0 {
		t.Fatalf("with no providers the list is empty, not nil: %#v", list)
	}
	if n, err := plain.svc.PurgeExpiredOAuthFlows(context.Background()); n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
}

func TestCompleteSignupChecksTermsBeforeTouchingTheTicket(t *testing.T) {
	r, _, _ := socialRig(t)
	_, _, err := r.svc.CompleteSignup(context.Background(), CompleteSignupInput{Ticket: "t", DisplayName: "A"}, ClientInfo{})
	if e, ok := errs.As(err); !ok || e.Code != errs.Validation || e.Fields["accept_terms"] == "" {
		t.Fatalf("%v", err)
	}
	if _, _, err := r.svc.CompleteSignup(context.Background(), CompleteSignupInput{AcceptTerms: true}, ClientInfo{}); !errs.Is(err, errs.NotFound) {
		t.Fatalf("no ticket: %v", err)
	}
	if _, err := r.svc.PendingSignup(context.Background(), strings.Repeat("a", 500)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("overlong ticket: %v", err)
	}
}
