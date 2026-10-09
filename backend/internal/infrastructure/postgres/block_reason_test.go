package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func TestUsersBlockReasonNamesWhyAnOwnerCannotPublish(t *testing.T) {
	db := testutil.OpenDB(t)
	ctx := context.Background()
	users := postgres.NewUsers(db)
	mk := func(email, set string) uuid.UUID {
		u := newUser(t, db, email)
		if set != "" {
			if _, err := db.Pool.Exec(ctx, `UPDATE users SET `+set+` WHERE id = $1`, u.ID); err != nil {
				t.Fatal(err)
			}
		}
		return u.ID
	}
	for name, c := range map[string]struct {
		id   uuid.UUID
		want string
	}{
		"active":    {mk("br-active@example.com", ""), ""},
		"scheduled": {mk("br-sched@example.com", "deletion_scheduled_at = now() + interval '7 days'"), scheduler.SkipAccountDeletion},
		"deleted":   {mk("br-del@example.com", "status = 'deleted'"), scheduler.SkipAccountDeletion},
		"disabled":  {mk("br-dis@example.com", "status = 'disabled'"), scheduler.SkipOwnerDisabled},
		"missing":   {uuid.New(), scheduler.SkipAccountDeletion},
	} {
		if got, err := users.BlockReason(ctx, c.id); err != nil || got != c.want {
			t.Errorf("%s: reason = %q, err = %v, want %q", name, got, err, c.want)
		}
	}
}

// The purge's claim must wait for a publisher transaction that has already passed the owner check.
func TestClaimWaitsForAPublisherTransactionHoldingTheOwnerRow(t *testing.T) {
	db := testutil.OpenDB(t)
	ctx := context.Background()
	u := newUser(t, db, "br-claim@example.com")
	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deletion_scheduled_at = now() - interval '1 minute' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	users, dels := postgres.NewUsers(db), postgres.NewDeletions(db)

	release, claimed := make(chan struct{}), make(chan struct{})
	held := make(chan struct{})
	go func() {
		_ = db.InTx(ctx, func(ctx context.Context) error {
			if _, err := users.BlockReason(ctx, u.ID); err != nil {
				t.Error(err)
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	go func() {
		defer close(claimed)
		if _, ok, err := dels.Claim(ctx, u.ID, time.Now()); err != nil || !ok {
			t.Errorf("claim: ok=%v err=%v", ok, err)
		}
	}()
	select {
	case <-claimed:
		t.Fatal("the claim did not wait for the publisher transaction")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case <-claimed:
	case <-time.After(5 * time.Second):
		t.Fatal("the claim never finished after the publisher transaction ended")
	}
}
