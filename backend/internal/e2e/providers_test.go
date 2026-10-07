package e2e

import (
	"context"
	"sync/atomic"

	"github.com/socialos/backend/internal/adapters/mock"
	"github.com/socialos/backend/internal/adapters/provider"
)

// noLookup behaves like the mock but cannot look posts up and is not
// idempotent, like LinkedIn/Telegram: unknown outcomes must go to review.
type noLookup struct {
	m     *mock.Provider
	name  string
	fail  error // when set, Publish always returns it
	calls atomic.Int32
}

func newNoLookup(name string) *noLookup { return &noLookup{m: mock.New(), name: name} }

func (p *noLookup) Name() string        { return p.name }
func (p *noLookup) DisplayName() string { return p.name }
func (p *noLookup) Configured() bool    { return true }
func (p *noLookup) Supported() bool     { return true }
func (p *noLookup) Capabilities() provider.Capabilities {
	c := p.m.Capabilities()
	c.SafeToRetryAfterUnknown = false
	return c
}
func (p *noLookup) AuthorizeURL(a provider.AuthorizeParams) string { return p.m.AuthorizeURL(a) }
func (p *noLookup) Exchange(ctx context.Context, code, v, r string) (provider.Token, error) {
	return p.m.Exchange(ctx, code, v, r)
}
func (p *noLookup) Profile(ctx context.Context, tok string) (provider.Profile, error) {
	prof, err := p.m.Profile(ctx, tok)
	prof.ID = p.name + "-user"
	return prof, err
}
func (p *noLookup) Refresh(ctx context.Context, rt string) (provider.Token, error) {
	return p.m.Refresh(ctx, rt)
}
func (p *noLookup) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	p.calls.Add(1)
	if p.fail != nil {
		return provider.PublishResult{}, p.fail
	}
	return p.m.Publish(ctx, req)
}
func (p *noLookup) Delete(ctx context.Context, req provider.DeleteRequest) error {
	return p.m.Delete(ctx, req)
}
