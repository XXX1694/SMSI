package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/developer"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

// TouchLastUsed writes the first use, then at most once a minute; the clock is the parameter.
func TestTouchLastUsedIsThrottledToOncePerMinute(t *testing.T) {
	db := testutil.OpenDB(t)
	ctx := context.Background()
	u := newUser(t, db, "touch@example.com")
	keys := postgres.NewAPIKeys(db)
	k := &apikey.Key{ID: uuid.New(), UserID: u.ID, Name: "k", Prefix: "sk_test", KeyHash: "touch-hash", Scopes: []apikey.Scope{}}
	if err := keys.Create(ctx, k); err != nil {
		t.Fatal(err)
	}
	conn := &developer.MCPConnection{ID: uuid.New(), UserID: u.ID, APIKeyID: k.ID, Name: "c", ClientName: "cli"}
	if err := postgres.NewMCPConnections(db).Create(ctx, conn); err != nil {
		t.Fatal(err)
	}

	read := func() (key, seen time.Time) {
		t.Helper()
		got, err := keys.GetByHash(ctx, "touch-hash")
		if err != nil || got.LastUsedAt == nil {
			t.Fatalf("key after touch: %v %+v", err, got)
		}
		c, err := postgres.NewMCPConnections(db).Get(ctx, u.ID, conn.ID)
		if err != nil || c.LastSeenAt == nil {
			t.Fatalf("connection after touch: %v %+v", err, c)
		}
		return got.LastUsedAt.UTC(), c.LastSeenAt.UTC()
	}
	touch := func(at time.Time) {
		t.Helper()
		if err := keys.TouchLastUsed(ctx, k.ID, at); err != nil {
			t.Fatal(err)
		}
	}

	touch(base)
	if used, seen := read(); !used.Equal(base) || !seen.Equal(base) {
		t.Fatalf("first use not recorded: %v %v", used, seen)
	}
	touch(base.Add(30 * time.Second))
	touch(base.Add(60 * time.Second)) // exactly one minute is still inside the window
	if used, seen := read(); !used.Equal(base) || !seen.Equal(base) {
		t.Fatalf("throttle failed, key=%v conn=%v", used, seen)
	}
	later := base.Add(61 * time.Second)
	touch(later)
	if used, seen := read(); !used.Equal(later) || !seen.Equal(later) {
		t.Fatalf("update after the window missing, key=%v conn=%v", used, seen)
	}
}
