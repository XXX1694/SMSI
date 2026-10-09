package postgres_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func TestMigration00005AddsExportIndexesAndRollsBack(t *testing.T) {
	url := testutil.FreshDatabase(t)
	ctx := context.Background()
	db, err := postgres.Open(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	indexes := func() int {
		return scalar[int](t, db, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN
			('posts_user_id_idx','audit_logs_user_id_idx','media_user_id_idx','action_approvals_user_id_idx','data_exports_sweep_idx')`)
	}
	if got := indexes(); got != 5 {
		t.Fatalf("up: %d of 5 indexes", got)
	}
	if err := postgres.Migrate(ctx, url, "down", testutil.Logger()); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := indexes(); got != 0 {
		t.Fatalf("down left %d indexes", got)
	}
	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil || indexes() != 5 {
		t.Fatalf("re-up: %v, %d indexes", err, indexes())
	}
}
