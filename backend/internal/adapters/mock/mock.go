// Package mock is a deterministic, in-memory provider for tests and local
// development (SOCIAL_MOCK_PROVIDERS=true). It never talks to a real network.
//
// Behaviour controls inside post text (for tests/demo):
//
//	#mock-fail      → permanent provider error
//	#mock-retry     → retryable error on the first attempt of each idempotency key
//	#mock-auth      → auth error (account expires)
//	#mock-unknown   → outcome unknown on first attempt (post IS recorded)
package mock

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// Name is the provider identifier.
const Name = "mock"

// Provider is the mock network.
type Provider struct {
	mu        sync.Mutex
	published map[string]provider.PublishResult // by idempotency key
	calls     map[string]int
}

// New returns a mock provider.
func New() *Provider {
	return &Provider{published: map[string]provider.PublishResult{}, calls: map[string]int{}}
}

func (p *Provider) Name() string        { return Name }
func (p *Provider) DisplayName() string { return "Mock Network (dev/test)" }
func (p *Provider) Configured() bool    { return true }
func (p *Provider) Supported() bool     { return true }

// Capabilities of the mock network.
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		CanPublishText: true, CanPublishImage: true, CanPublishVideo: true, CanDelete: true,
		MaxTextLength: 5000, MaxMediaCount: 10, SafeToRetryAfterUnknown: false,
		ConnectMethod: provider.ConnectOAuth,
		Notes:         "MOCK provider for development and tests only. Nothing is posted anywhere.",
	}
}

// AuthorizeURL skips consent and redirects straight back to the callback.
func (p *Provider) AuthorizeURL(a provider.AuthorizeParams) string {
	sep := "?"
	if strings.Contains(a.RedirectURI, "?") {
		sep = "&"
	}
	code := "mock-code-" + crypto.SHA256Hex(a.CodeChallenge)[:12]
	if a.LoginHint != "" {
		code += hintSep + a.LoginHint
	}
	return a.RedirectURI + sep + "code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(a.State)
}

// hintSep separates the optional account hint inside mock codes and tokens.
const hintSep = "~"

// Exchange returns a deterministic token that expires in one hour.
func (p *Provider) Exchange(_ context.Context, code, verifier, _ string) (provider.Token, error) {
	if !strings.HasPrefix(code, "mock-code-") || verifier == "" {
		return provider.Token{}, &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "invalid_grant", Message: "invalid authorization code"}
	}
	exp := time.Now().Add(time.Hour)
	return provider.Token{AccessToken: "mock-access-" + code, RefreshToken: "mock-refresh-" + code, ExpiresAt: &exp, Scopes: []string{"publish"}}, nil
}

// Profile returns a fixed identity unless the consent carried an account hint,
// which lets developers connect several distinct mock accounts.
func (p *Provider) Profile(_ context.Context, accessToken string) (provider.Profile, error) {
	if accessToken == "" {
		return provider.Profile{}, &provider.Error{Kind: provider.KindAuth, Provider: Name, Message: "missing token"}
	}
	if i := strings.LastIndex(accessToken, hintSep); i >= 0 {
		h := accessToken[i+len(hintSep):]
		return provider.Profile{ID: "mock-" + h, Username: h, DisplayName: "Mock " + h, Metadata: map[string]any{"mock": true}}, nil
	}
	return provider.Profile{ID: "mock-user-1", Username: "mock_user", DisplayName: "Mock User", Metadata: map[string]any{"mock": true}}, nil
}

// Refresh issues a new access token.
func (p *Provider) Refresh(_ context.Context, refreshToken string) (provider.Token, error) {
	if !strings.HasPrefix(refreshToken, "mock-refresh-") {
		return provider.Token{}, &provider.Error{Kind: provider.KindAuth, Provider: Name, Message: "refresh token invalid"}
	}
	exp := time.Now().Add(time.Hour)
	return provider.Token{AccessToken: "mock-access-refreshed", RefreshToken: refreshToken, ExpiresAt: &exp}, nil
}

// Publish records the post in memory; repeated keys return the same result.
func (p *Provider) Publish(_ context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[req.IdempotencyKey]++
	first := p.calls[req.IdempotencyKey] == 1
	switch {
	case strings.Contains(req.Text, "#mock-fail"):
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "MOCK_REJECTED", Message: "mock provider rejected the post"}
	case strings.Contains(req.Text, "#mock-auth"):
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindAuth, Provider: Name, HTTPStatus: 401, Message: "mock token revoked"}
	case strings.Contains(req.Text, "#mock-retry") && first:
		return provider.PublishResult{}, provider.FromHTTPStatus(Name, 503, "mock temporarily unavailable", 0)
	}
	if prev, ok := p.published[req.IdempotencyKey]; ok {
		return prev, nil
	}
	res := provider.PublishResult{
		ExternalID: "mock-" + req.IdempotencyKey,
		URL:        "https://mock.socialos.local/posts/" + req.IdempotencyKey,
		Metadata:   map[string]any{"media_count": len(req.Media)},
	}
	p.published[req.IdempotencyKey] = res
	if strings.Contains(req.Text, "#mock-unknown") && first {
		return provider.PublishResult{}, fmt.Errorf("mock: %w", context.DeadlineExceeded)
	}
	return res, nil
}

// Lookup finds a post by idempotency key (resolves unknown outcomes).
func (p *Provider) Lookup(_ context.Context, req provider.PublishRequest) (provider.PublishResult, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	res, ok := p.published[req.IdempotencyKey]
	return res, ok, nil
}

// Delete removes a mock post.
func (p *Provider) Delete(_ context.Context, req provider.DeleteRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.published {
		if v.ExternalID == req.ExternalID {
			delete(p.published, k)
		}
	}
	return nil
}

// Calls returns how many times Publish was invoked for a key (tests).
func (p *Provider) Calls(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[key]
}
