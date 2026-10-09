package app

import (
	"fmt"

	"github.com/socialos/backend/internal/adapters/identity/github"
	"github.com/socialos/backend/internal/adapters/identity/oidc"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/infrastructure/postgres"
)

// buildSignIn wires the social sign-in providers that the configuration turns on (D-023). The override replaces them
// in tests, which point the adapters at fake providers. With no provider sign-in is off and GET /auth/providers
// answers an empty list, but the repositories stay wired, so identities linked earlier remain listed and removable
// after a provider is switched off.
func buildSignIn(cfg *config.Config, db *postgres.DB, enc *crypto.Cipher, override []auth.IdentityProvider) (*auth.SocialDeps, error) {
	providers := override
	if providers == nil {
		if cfg.GoogleSignIn() {
			p, err := oidc.New(oidc.Google(cfg.GoogleClientID, cfg.GoogleClientSecret))
			if err != nil {
				return nil, fmt.Errorf("google sign-in: %w", err)
			}
			providers = append(providers, p)
		}
		if cfg.GitHubSignIn() {
			p, err := github.New(github.Config{ClientID: cfg.GitHubClientID, ClientSecret: cfg.GitHubClientSecret})
			if err != nil {
				return nil, fmt.Errorf("github sign-in: %w", err)
			}
			providers = append(providers, p)
		}
	}
	return &auth.SocialDeps{Identities: postgres.NewIdentities(db), Flows: postgres.NewOAuthFlows(db), Providers: providers,
		Cipher: enc, APIPublicURL: cfg.APIPublicURL}, nil
}
