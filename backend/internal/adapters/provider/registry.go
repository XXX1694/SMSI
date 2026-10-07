package provider

import (
	"sort"
	"sync"

	"github.com/socialos/backend/internal/domain/errs"
)

// Registry maps provider names to implementations.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry builds a registry from providers (later duplicates win).
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{providers: map[string]Provider{}}
	for _, p := range ps {
		r.Register(p)
	}
	return r
}

// Register adds or replaces a provider.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get returns a provider or PROVIDER_NOT_AVAILABLE for unknown names.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, errs.Newf(errs.ProviderNotAvailable, "provider %q is not available", name)
	}
	return p, nil
}

// Publisher returns the provider as a Publisher, or PROVIDER_NOT_AVAILABLE.
func (r *Registry) Publisher(name string) (Provider, Publisher, error) {
	p, err := r.Get(name)
	if err != nil {
		return nil, nil, err
	}
	pub, ok := p.(Publisher)
	if !ok || !p.Supported() {
		return nil, nil, errs.Newf(errs.ProviderNotAvailable, "%s publishing is not available", p.DisplayName())
	}
	return p, pub, nil
}

// OAuth returns the provider as an OAuth provider, or PROVIDER_NOT_AVAILABLE.
func (r *Registry) OAuth(name string) (Provider, OAuth, error) {
	p, err := r.Get(name)
	if err != nil {
		return nil, nil, err
	}
	o, ok := p.(OAuth)
	if !ok || !p.Supported() {
		return nil, nil, errs.Newf(errs.ProviderNotAvailable, "%s does not support OAuth connect", p.DisplayName())
	}
	if !p.Configured() {
		return nil, nil, errs.Newf(errs.ProviderNotAvailable, "%s is not configured on this server", p.DisplayName())
	}
	return p, o, nil
}

// List returns providers sorted: supported first, then by name.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Supported() != out[j].Supported() {
			return out[i].Supported()
		}
		return out[i].Name() < out[j].Name()
	})
	return out
}
