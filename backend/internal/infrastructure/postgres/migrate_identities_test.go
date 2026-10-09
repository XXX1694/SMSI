package postgres_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func TestMigration00008UpAndDown(t *testing.T) {
	url := testutil.FreshDatabase(t) // migrated up
	ctx := context.Background()
	db, err := postgres.Open(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	state := func() (tables int, nullable bool) {
		return scalar[int](t, db, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'
				AND table_name IN ('user_identities','auth_oauth_flows')`),
			scalar[bool](t, db, `SELECT is_nullable = 'YES' FROM information_schema.columns
				WHERE table_name='users' AND column_name='password_hash'`)
	}
	if tables, nullable := state(); tables != 2 || !nullable {
		t.Fatalf("up: tables=%d password_hash nullable=%v", tables, nullable)
	}
	// An existing password user is untouched by either direction.
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES ('keeps@example.com','$argon2id$x')`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, url, "down", testutil.Logger()); err != nil {
		t.Fatalf("down: %v", err)
	}
	if tables, nullable := state(); tables != 0 || nullable {
		t.Fatalf("down: tables=%d password_hash nullable=%v", tables, nullable)
	}
	if got := scalar[string](t, db, `SELECT password_hash FROM users WHERE email='keeps@example.com'`); got != "$argon2id$x" {
		t.Fatalf("down changed a real hash: %q", got)
	}
	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if tables, nullable := state(); tables != 2 || !nullable {
		t.Fatal("re-up incomplete")
	}
}
