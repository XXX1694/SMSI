package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/socialos/backend/internal/domain/dataexport"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

// A build that outlived the sweep must not overwrite the row the sweep already failed.
func TestMarkReadyOnlyFromRunning(t *testing.T) {
	db := testutil.OpenDB(t)
	ctx := context.Background()
	u := newUser(t, db, "export-ready@example.com")
	repo := postgres.NewExports(db)
	exp := time.Now().Add(time.Hour)

	e, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkReady(ctx, e.ID, "k", 1, exp); !errs.Is(err, errs.NotFound) {
		t.Fatalf("MarkReady on a pending export: %v, want NotFound", err)
	}
	if _, err := repo.Claim(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkFailed(ctx, e.ID, dataexport.ErrTimedOut); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkReady(ctx, e.ID, "k", 1, exp); !errs.Is(err, errs.NotFound) {
		t.Fatalf("MarkReady on a failed export: %v, want NotFound", err)
	}
	got, err := repo.Get(ctx, u.ID, e.ID)
	if err != nil || got.Status != dataexport.StatusFailed || got.StorageKey != "" {
		t.Fatalf("row = %+v, %v; want it still failed with no key", got, err)
	}

	e2, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Claim(ctx, e2.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkReady(ctx, e2.ID, "k2", 7, exp); err != nil {
		t.Fatalf("MarkReady on a running export: %v", err)
	}
	if got, _ := repo.Get(ctx, u.ID, e2.ID); got == nil || got.Status != dataexport.StatusReady || got.StorageKey != "k2" {
		t.Fatalf("row = %+v, want ready with key k2", got)
	}
}
