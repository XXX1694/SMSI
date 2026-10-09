package schedulertest

import (
	"context"

	"github.com/socialos/backend/internal/adapters/provider"
)

// Provider is a scriptable provider.Provider + provider.Publisher.
// Publish pops PublishErrs/PublishResults in order; when both are exhausted it
// succeeds with ExternalID "ext-<n>". It is NOT a Lookuper or OAuth provider:
// use LookupProvider / OAuthProvider for those capabilities.
type Provider struct {
	ProvName string
	Caps     provider.Capabilities
	Stub     bool // Supported() == !Stub

	// PublishErrs[i] is the error of the i-th Publish call (nil entry = success).
	PublishErrs []error
	// Requests records every PublishRequest received.
	Requests []provider.PublishRequest
	calls    int
}

// NewProvider creates a text-publishing provider with ConnectNone (no token handling).
func NewProvider(name string) *Provider {
	return &Provider{ProvName: name, Caps: provider.Capabilities{CanPublishText: true, ConnectMethod: provider.ConnectNone}}
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return p.ProvName }

// DisplayName implements provider.Provider.
func (p *Provider) DisplayName() string { return p.ProvName }

// Capabilities implements provider.Provider.
func (p *Provider) Capabilities() provider.Capabilities { return p.Caps }

// Configured implements provider.Provider.
func (p *Provider) Configured() bool { return true }

// Supported implements provider.Provider.
func (p *Provider) Supported() bool { return !p.Stub }

// Publish implements provider.Publisher.
func (p *Provider) Publish(_ context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	p.Requests = append(p.Requests, req)
	i := p.calls
	p.calls++
	if i < len(p.PublishErrs) && p.PublishErrs[i] != nil {
		return provider.PublishResult{}, p.PublishErrs[i]
	}
	return provider.PublishResult{ExternalID: "ext-" + itoa(i+1), URL: "https://example.test/" + itoa(i+1),
		Metadata: map[string]any{"call": i + 1}}, nil
}

// Delete implements provider.Publisher.
func (p *Provider) Delete(context.Context, provider.DeleteRequest) error { return nil }

// PublishCalls is the number of Publish calls received.
func (p *Provider) PublishCalls() int { return p.calls }

// LookupProvider adds provider.Lookuper: it finds a previous post when Found is set.
type LookupProvider struct {
	*Provider
	Found     *provider.PublishResult
	LookupErr error
	Lookups   int
}

// NewLookupProvider creates a provider that supports Lookup.
func NewLookupProvider(name string) *LookupProvider {
	return &LookupProvider{Provider: NewProvider(name)}
}

// Lookup implements provider.Lookuper.
func (l *LookupProvider) Lookup(context.Context, provider.PublishRequest) (provider.PublishResult, bool, error) {
	l.Lookups++
	if l.LookupErr != nil {
		return provider.PublishResult{}, false, l.LookupErr
	}
	if l.Found == nil {
		return provider.PublishResult{}, false, nil
	}
	return *l.Found, true, nil
}

// OAuthProvider adds provider.OAuth; only Refresh is scriptable, the rest are inert.
type OAuthProvider struct {
	*Provider
	RefreshTok  provider.Token
	RefreshErr  error
	RefreshedBy []string // refresh tokens passed to Refresh
}

// NewOAuthProvider creates an OAuth-connected provider (ConnectMethod oauth).
func NewOAuthProvider(name string) *OAuthProvider {
	p := NewProvider(name)
	p.Caps.ConnectMethod = provider.ConnectOAuth
	return &OAuthProvider{Provider: p}
}

// AuthorizeURL implements provider.OAuth.
func (o *OAuthProvider) AuthorizeURL(provider.AuthorizeParams) string { return "" }

// Exchange implements provider.OAuth.
func (o *OAuthProvider) Exchange(context.Context, string, string, string) (provider.Token, error) {
	return provider.Token{}, nil
}

// Profile implements provider.OAuth.
func (o *OAuthProvider) Profile(context.Context, string) (provider.Profile, error) {
	return provider.Profile{}, nil
}

// Refresh implements provider.OAuth.
func (o *OAuthProvider) Refresh(_ context.Context, refresh string) (provider.Token, error) {
	o.RefreshedBy = append(o.RefreshedBy, refresh)
	return o.RefreshTok, o.RefreshErr
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
