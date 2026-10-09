package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/domain/redirect"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// StartResult is where to send the browser, and the raw state the transport must keep in a cookie.
type StartResult struct {
	URL   string
	State string
}

// StartSocial begins a sign-in: it stores the hashed state and nonce and the encrypted PKCE verifier, and returns the
// provider's consent URL. A provider that is unknown or not configured is NOT_FOUND.
func (s *Service) StartSocial(ctx context.Context, id identity.Provider, next string) (StartResult, error) {
	return s.startFlow(ctx, id, identity.IntentLogin, nil, redirect.SafePath(next, DefaultLoginRedirect))
}

// startFlow stores a flow of either intent and builds the provider's consent URL. A link flow is bound to linkUser.
func (s *Service) startFlow(ctx context.Context, id identity.Provider, intent identity.Intent, linkUser *uuid.UUID, after string) (StartResult, error) {
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
	f := &identity.Flow{Provider: id, Intent: intent, LinkUserID: linkUser, StateHash: crypto.SHA256Hex(state), NonceHash: crypto.SHA256Hex(nonce),
		CodeVerifierEnc: verifierEnc, RedirectAfter: after, ExpiresAt: s.clock.Now().Add(FlowTTL)}
	if err := s.social.flows.Create(ctx, f); err != nil {
		return StartResult{}, err
	}
	u := p.AuthorizeURL(AuthorizeRequest{State: state, Nonce: nonce, CodeVerifier: verifier, RedirectURI: s.social.redirectURI(id)})
	return StartResult{URL: u, State: state}, nil
}
