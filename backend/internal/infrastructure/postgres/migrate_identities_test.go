package postgres_test

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/domain/user"
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
	if err := postgres.MigrateDownTo(ctx, url, 7, testutil.Logger()); err != nil {
		t.Fatalf("down to 00007: %v", err)
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

// A password-less user survives the round trip: Down marks the hash with "!" (NOT NULL needs a value), Up turns the
// marker back into NULL, so the user is still password-less afterwards.
func TestPasswordHashNullableSurvivesDownMigration(t *testing.T) {
	url := testutil.FreshDatabase(t)
	ctx := context.Background()
	db, err := postgres.Open(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := postgres.NewUsers(db).Create(ctx, &user.User{Email: "social-only@example.com", Status: user.StatusActive}); err != nil {
		t.Fatal(err)
	}
	if err := postgres.MigrateDownTo(ctx, url, 7, testutil.Logger()); err != nil {
		t.Fatalf("down with a password-less user: %v", err)
	}
	if got := scalar[string](t, db, `SELECT password_hash FROM users WHERE email='social-only@example.com'`); got != "!" {
		t.Fatalf("marker %q, want !", got)
	}
	if !scalar[bool](t, db, `SELECT is_nullable = 'NO' FROM information_schema.columns
		WHERE table_name='users' AND column_name='password_hash'`) {
		t.Fatal("down must restore NOT NULL")
	}
	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	got, err := postgres.NewUsers(db).GetByEmail(ctx, "social-only@example.com")
	if err != nil || got.HasPassword() {
		t.Fatalf("after re-up: %+v %v", got, err)
	}
	if n := scalar[int](t, db, `SELECT count(*) FROM users WHERE password_hash IS NULL`); n != 1 {
		t.Fatalf("the marker must become NULL again, %d NULL rows", n)
	}
}
