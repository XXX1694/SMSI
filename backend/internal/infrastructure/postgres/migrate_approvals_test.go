package postgres_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func TestMigration00004ApprovesExistingKeysAndRollsBack(t *testing.T) {
	url := testutil.FreshDatabase(t) // migrated up
	ctx := context.Background()
	db, err := postgres.Open(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	has := func() (bool, bool) {
		return scalar[bool](t, db, `SELECT to_regclass('public.action_approvals') IS NOT NULL`),
			scalar[bool](t, db, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='api_keys' AND column_name='dangerous_policy')`)
	}
	if tbl, col := has(); !tbl || !col {
		t.Fatalf("up: table=%v column=%v", tbl, col)
	}

	// 00007, 00006 and 00005 sit on top of 00004 (they have their own tests); roll them back first, then 00004 itself.
	// A key that exists before the migration must come out with the safe policy.
	rollBack(t, url, 4)
	if tbl, col := has(); tbl || col {
		t.Fatalf("down: table=%v column=%v", tbl, col)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ('11111111-1111-1111-1111-111111111111','k@example.com','x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO api_keys (user_id, name, prefix, key_hash) VALUES ('11111111-1111-1111-1111-111111111111','old','sk_live_x','hash-old')`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if got := scalar[string](t, db, `SELECT dangerous_policy FROM api_keys WHERE name='old'`); got != "approve" {
		t.Fatalf("existing key policy %q, want approve", got)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE api_keys SET dangerous_policy='anything'`); err == nil {
		t.Fatal("dangerous_policy must only accept approve or trusted")
	}
}
