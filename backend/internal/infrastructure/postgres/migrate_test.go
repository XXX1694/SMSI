package postgres_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func scalar[T any](t *testing.T, db *postgres.DB, sql string) T {
	t.Helper()
	var v T
	if err := db.Pool.QueryRow(context.Background(), sql).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

// rollBack undoes the newest n migrations.
func rollBack(t *testing.T, url string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := postgres.Migrate(context.Background(), url, "down", testutil.Logger()); err != nil {
			t.Fatalf("rollback %d of %d: %v", i+1, n, err)
		}
	}
}

func tableCount(t *testing.T, db *postgres.DB) int {
	return scalar[int](t, db, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'
		AND table_name IN ('email_tokens','data_exports','account_deletions')`)
}

func columnCount(t *testing.T, db *postgres.DB) int {
	return scalar[int](t, db, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND
		((table_name='users' AND column_name IN ('email_verified_at','terms_accepted_at','terms_version','plan','deleted_at'))
		 OR (table_name='posts' AND column_name='quota_counted_at'))`)
}

// canSetStatus reports whether users.status accepts the value (inside a rolled-back statement).
func canSetStatus(t *testing.T, db *postgres.DB, status string) bool {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO users (email, password_hash, status) VALUES ('s@example.com','x',$1)`, status)
	return err == nil
}

func TestMigration00003UpAndDown(t *testing.T) {
	url := testutil.FreshDatabase(t) // already migrated up
	ctx := context.Background()
	db, err := postgres.Open(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if tableCount(t, db) != 3 || columnCount(t, db) != 6 {
		t.Fatalf("up: tables=%d columns=%d", tableCount(t, db), columnCount(t, db))
	}
	if !canSetStatus(t, db, "deleted") || canSetStatus(t, db, "bogus") {
		t.Fatal("up: users.status must accept 'deleted' and still reject unknown values")
	}
	if got := scalar[string](t, db, `SELECT column_default FROM information_schema.columns
		WHERE table_name='users' AND column_name='plan'`); got != "'free'::text" {
		t.Fatalf("plan default %q", got)
	}

	// 00007, 00006, 00005 and 00004 sit on top of 00003; roll them back first so the "down" below undoes 00003 (they have their own tests).
	rollBack(t, url, 4)
	// A deleted user must not block the rollback.
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users (email, password_hash, status) VALUES ('gone@example.com','x','deleted')`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, url, "down", testutil.Logger()); err != nil {
		t.Fatalf("down: %v", err)
	}
	if tableCount(t, db) != 0 || columnCount(t, db) != 0 {
		t.Fatalf("down: tables=%d columns=%d", tableCount(t, db), columnCount(t, db))
	}
	if canSetStatus(t, db, "deleted") || !canSetStatus(t, db, "disabled") {
		t.Fatal("down: users.status must be back to active/disabled")
	}
	if got := scalar[string](t, db, `SELECT status FROM users WHERE email='gone@example.com'`); got != "disabled" {
		t.Fatalf("down: deleted user became %q", got)
	}
	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if tableCount(t, db) != 3 || columnCount(t, db) != 6 {
		t.Fatal("re-up incomplete")
	}
}
