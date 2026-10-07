package accounts

import (
	"context"
	"strings"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// ConnectChat connects a non-OAuth chat provider (Telegram) after verifying
// that the configured bot can post there. No token is stored per account.
func (s *Service) ConnectChat(ctx context.Context, a actor.Actor, providerName, chat string) (*socialaccount.Account, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	chat = strings.TrimSpace(chat)
	if chat == "" {
		return nil, errs.Validationf("chat is required").WithField("chat", "required")
	}
	p, err := s.registry.Get(providerName)
	if err != nil {
		return nil, err
	}
	verifier, ok := p.(provider.ChatVerifier)
	if !ok || !p.Supported() {
		return nil, errs.Newf(errs.ProviderNotAvailable, "%s does not support chat connections", p.DisplayName())
	}
	if !p.Configured() {
		return nil, errs.Newf(errs.ProviderNotAvailable, "%s is not configured on this server", p.DisplayName())
	}
	prof, err := verifier.VerifyChat(ctx, chat)
	if err != nil {
		return nil, providerFailure(err)
	}
	acc := accountFromProfile(a, p.Name(), prof, []string{"post_messages"}, s.clock.Now())
	if err := s.connectAccount(ctx, a, acc, nil); err != nil {
		return nil, err
	}
	return acc, nil
}
