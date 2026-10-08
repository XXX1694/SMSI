package app

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/telegram"
	"github.com/socialos/backend/internal/application/accounts"
	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	dmedia "github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/socialaccount"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/infrastructure/redis"
)

// accountsAdapter satisfies scheduler.Accounts.
type accountsAdapter struct {
	repo *postgres.Accounts
	svc  *accounts.Service
}

func (a accountsAdapter) Get(ctx context.Context, userID, id uuid.UUID) (*socialaccount.Account, error) {
	return a.repo.Get(ctx, userID, id)
}

func (a accountsAdapter) MarkExpired(ctx context.Context, act actor.Actor, acc *socialaccount.Account, reason string) error {
	return a.svc.MarkExpired(ctx, act, acc, reason)
}

// verifiedOwners satisfies accounts.OwnerGate: with verification enforced, an owner must have a verified address.
type verifiedOwners struct {
	users   *postgres.Users
	enforce bool
}

func (v verifiedOwners) RequireVerifiedOwner(ctx context.Context, userID uuid.UUID) error {
	if !v.enforce {
		return nil
	}
	u, err := v.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.EmailVerified() {
		return errs.New(errs.EmailNotVerified, "verify your email address to use this feature")
	}
	return nil
}

// mediaAdapter satisfies scheduler.MediaStore.
type mediaAdapter struct {
	repo    *postgres.Media
	storage media.Storage
}

func (m mediaAdapter) GetMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]dmedia.Media, error) {
	return m.repo.GetMany(ctx, userID, ids)
}

func (m mediaAdapter) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return m.storage.Get(ctx, key)
}

// telegramPollStore satisfies telegram.PollerStore with Redis.
type telegramPollStore struct{ s *redis.PollState }

func (p telegramPollStore) Offset(ctx context.Context) (int64, error) { return p.s.Offset(ctx) }

func (p telegramPollStore) Acquire(ctx context.Context, ttl time.Duration) (telegram.Lease, bool, error) {
	l, ok, err := p.s.Acquire(ctx, ttl)
	if err != nil || !ok {
		return nil, false, err
	}
	return l, true, nil
}
