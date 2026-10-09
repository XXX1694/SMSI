// Package mocktoken is a deterministic, in-memory provider connected with a
// pasted token. It exists for tests and local development
// (SOCIAL_MOCK_PROVIDERS=true) and exercises the whole token flow: form
// fields, live verification, encrypted credential, publishing with a title.
// It never talks to a network.
//
// Credentials: an api_key starting with "mt_" is accepted; "mt_revoked..." is
// accepted at connect time but fails to publish with an auth error (a token
// revoked later); anything else is rejected at connect time.
package mocktoken

import (
	"context"
	"strings"

	"github.com/socialos/backend/internal/adapters/mock"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// Name is the provider identifier.
const Name = "mocktoken"

const (
	keyPrefix     = "mt_"
	revokedPrefix = "mt_revoked"
)

// Provider reuses the publishing behaviour of the mock network.
type Provider struct {
	*mock.Provider
}

// New returns a mock token provider.
func New() *Provider { return &Provider{Provider: mock.New()} }

func (p *Provider) Name() string        { return Name }
func (p *Provider) DisplayName() string { return "Mock Token Network (dev/test)" }

// Capabilities describe an article-style network that needs a title.
func (p *Provider) Capabilities() provider.Capabilities {
	c := p.Provider.Capabilities()
	c.ConnectMethod = provider.ConnectToken
	c.CanPublishVideo = false
	c.RequiresTitle = true
	c.MaxImageBytes = 1 << 20
	c.MaxMediaCount = 4
	c.ConnectFields = []provider.ConnectField{
		{Name: "api_key", Label: "API key", Help: "Any value starting with mt_ is accepted.", Placeholder: "mt_...", Kind: provider.FieldSecret, Required: true},
		{Name: "handle", Label: "Handle", Help: "Optional display handle.", Placeholder: "my_handle", Kind: provider.FieldText},
	}
	c.Notes = "MOCK token provider for development and tests only. Nothing is posted anywhere."
	return c
}

// Verify accepts keys that start with mt_ and returns a non-secret profile.
func (p *Provider) Verify(_ context.Context, fields map[string]string) (provider.Profile, string, error) {
	key := fields["api_key"]
	if !strings.HasPrefix(key, keyPrefix) {
		return provider.Profile{}, "", &provider.Error{Kind: provider.KindAuth, Provider: Name, HTTPStatus: 401, Message: "mock token rejected"}
	}
	handle := fields["handle"]
	if handle == "" {
		handle = "mock_token_user"
	}
	// The account id derives from a hash so that reconnecting the same key
	// updates one account and the id reveals nothing about the key.
	id := "mt-" + crypto.SHA256Hex(key)[:12]
	return provider.Profile{ID: id, Username: handle, DisplayName: "Mock Token " + handle,
		Metadata: map[string]any{"mock": true, "limits": map[string]any{"max_characters": 1000}}}, key, nil
}

// Publish fails with an auth error for a revoked key, otherwise behaves like the mock network.
func (p *Provider) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	if strings.HasPrefix(req.AccessToken, revokedPrefix) || !strings.HasPrefix(req.AccessToken, keyPrefix) {
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindAuth, Provider: Name, HTTPStatus: 401, Message: "mock token revoked"}
	}
	res, err := p.Provider.Publish(ctx, req)
	if err == nil {
		if res.Metadata == nil {
			res.Metadata = map[string]any{}
		}
		res.Metadata["title"] = req.Title
	}
	return res, err
}
