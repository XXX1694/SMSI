package accounts

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Service is the accounts use-case facade.
type Service struct {
	repo            Repo
	states          States
	links           LinkCodes
	log             *slog.Logger
	vault           *Vault
	registry        *provider.Registry
	tx              port.TxRunner
	audit           port.AuditRecorder
	clock           port.Clock
	enc             port.Encryptor
	redirectBaseURL string
	gate            OwnerGate
	approvals       port.ApprovalGate
}

// Deps bundles dependencies.
type Deps struct {
	Repo     Repo
	States   States
	Links    LinkCodes
	Log      *slog.Logger
	Registry *provider.Registry
	Tx       port.TxRunner
	Audit    port.AuditRecorder
	Clock    port.Clock
	Enc      port.Encryptor
	// RedirectBaseURL is the public base of the API, e.g. https://app.example.com
	RedirectBaseURL string
	// Gate, when set, is consulted before any account is stored (see OwnerGate).
	Gate OwnerGate
	// Approvals asks the owner to approve dangerous actions of API keys; nil refuses them (fail closed).
	Approvals port.ApprovalGate
}

// NewService creates the service.
func NewService(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	if d.Approvals == nil {
		d.Approvals = port.FailClosedGate{}
	}
	return &Service{repo: d.Repo, states: d.States, links: d.Links, log: log, vault: NewVault(d.Repo, d.Enc), registry: d.Registry,
		tx: d.Tx, audit: d.Audit, clock: d.Clock, enc: d.Enc, redirectBaseURL: d.RedirectBaseURL, gate: d.Gate, approvals: d.Approvals}
}

// Vault exposes credential storage to other use cases (scheduler).
func (s *Service) Vault() *Vault { return s.vault }

// ProviderInfo is one entry of GET /social/providers.
type ProviderInfo struct {
	Name         string
	DisplayName  string
	Supported    bool
	Configured   bool
	Capabilities provider.Capabilities
}

// Providers lists registered providers with capabilities.
func (s *Service) Providers(_ context.Context, a actor.Actor) ([]ProviderInfo, error) {
	if err := a.Require(apikey.SocialRead); err != nil {
		return nil, err
	}
	var out []ProviderInfo
	for _, p := range s.registry.List() {
		out = append(out, ProviderInfo{Name: p.Name(), DisplayName: p.DisplayName(), Supported: p.Supported(),
			Configured: p.Configured(), Capabilities: p.Capabilities()})
	}
	return out, nil
}

// List returns the tenant's non-revoked accounts.
func (s *Service) List(ctx context.Context, a actor.Actor) ([]socialaccount.Account, error) {
	if err := a.Require(apikey.SocialRead); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, a.UserID)
}

// Get returns one account of the tenant.
func (s *Service) Get(ctx context.Context, a actor.Actor, id uuid.UUID) (*socialaccount.Account, error) {
	if err := a.Require(apikey.SocialRead); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

// Disconnect revokes the account and destroys its stored tokens.
func (s *Service) Disconnect(ctx context.Context, a actor.Actor, id uuid.UUID) error {
	if err := a.Require(apikey.SocialDisconnect); err != nil {
		return err
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		acc, err := s.repo.Get(ctx, a.UserID, id)
		if err != nil {
			return err
		}
		if err := s.approvals.Require(ctx, a, disconnectRequest(acc)); err != nil {
			return err
		}
		if err := s.repo.SetStatus(ctx, a.UserID, id, socialaccount.StatusRevoked); err != nil {
			return err
		}
		if err := s.repo.DeleteCredentials(ctx, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionAccountRemoved, "social_account", id.String(),
			map[string]any{"provider": acc.Provider, "username": acc.Username})
	})
}

// MarkExpired flags an account whose tokens no longer work.
func (s *Service) MarkExpired(ctx context.Context, a actor.Actor, acc *socialaccount.Account, reason string) error {
	if err := s.repo.SetStatus(ctx, acc.UserID, acc.ID, socialaccount.StatusExpired); err != nil {
		return err
	}
	return s.audit.Record(ctx, a, audit.ActionAccountExpired, "social_account", acc.ID.String(),
		map[string]any{"provider": acc.Provider, "reason": reason})
}

func (s *Service) connectAccount(ctx context.Context, a actor.Actor, acc *socialaccount.Account, tok *provider.Token) error {
	// The owner may have been verified when the flow began and not now (verification was switched on in between),
	// and the system actor of a chat link carries no verification of its own.
	if s.gate != nil {
		if err := s.gate.RequireVerifiedOwner(ctx, acc.UserID); err != nil {
			return err
		}
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Upsert(ctx, acc); err != nil {
			return err
		}
		if tok != nil {
			if err := s.vault.Save(ctx, acc.ID, *tok); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, a, audit.ActionAccountConnected, "social_account", acc.ID.String(),
			map[string]any{"provider": acc.Provider, "username": acc.Username, "provider_account_id": acc.ProviderAccountID})
	})
}
