package postgres_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func TestMigration00006AddsTheDeletionScheduleAndRollsBack(t *testing.T) {
	url := testutil.FreshDatabase(t)
	ctx := context.Background()
	db, err := postgres.Open(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	has := func() (bool, int) {
		return scalar[bool](t, db, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='deletion_scheduled_at')`),
			scalar[int](t, db, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN
				('users_deletion_due_idx','account_deletions_user_uniq','scheduled_jobs_target_idx','analytics_target_idx')`)
	}
	if col, idx := has(); !col || idx != 4 {
		t.Fatalf("up: column=%v indexes=%d", col, idx)
	}
	// One deletion record per user.
	if _, err := db.Pool.Exec(ctx, `INSERT INTO account_deletions (user_id, requested_at) VALUES ('22222222-2222-2222-2222-222222222222', now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO account_deletions (user_id, requested_at) VALUES ('22222222-2222-2222-2222-222222222222', now())`); err == nil {
		t.Fatal("a second deletion record for one user must be refused")
	}
	// 00007 owns the two foreign-key indexes (built concurrently); 00006 owns the column and the other two.
	rollBack(t, url, 1)
	if col, idx := has(); !col || idx != 2 {
		t.Fatalf("down 00007: column=%v indexes=%d, want the column and 2 indexes left", col, idx)
	}
	rollBack(t, url, 1)
	if col, idx := has(); col || idx != 0 {
		t.Fatalf("down: column=%v indexes=%d", col, idx)
	}
	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil {
		t.Fatalf("re-up: %v", err)
	}
}
