package app

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/accounts"
	"github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/domain/actor"
	dmedia "github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/socialaccount"
	"github.com/socialos/backend/internal/infrastructure/postgres"
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
