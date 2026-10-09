package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

// The API (cancel/unschedule/publish) locks post -> target; the worker must take the same order or the two deadlock
// (issue #38). The test replays the losing interleaving deterministically: the API holds the post lock, the worker
// asks for the target, then the API updates the target. Before the fix the worker held the target while waiting for
// the post, and Postgres aborted one side with 40P01.
func TestLockTargetFollowsPostThenTargetOrder(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenDB(t)
	u := newUser(t, db, "lockorder@example.com")
	repo := postgres.NewPosts(db)

	var acc uuid.UUID
	if err := db.Pool.QueryRow(ctx, `INSERT INTO social_accounts (user_id, provider, provider_account_id) VALUES ($1,'mock','a') RETURNING id`, u.ID).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	newPost := func() *post.Post {
		p := &post.Post{ID: uuid.New(), UserID: u.ID, Content: "x", Status: post.StatusScheduled, CreatedBy: post.CreatedByUser}
		p.Targets = []post.Target{{ID: uuid.New(), PostID: p.ID, UserID: u.ID, SocialAccountID: acc, Platform: "mock", Content: "x", Status: post.TargetPending, IdempotencyKey: uuid.NewString()}}
		if err := repo.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	for round := 0; round < 3; round++ {
		p := newPost()
		apiHasPost := make(chan struct{})
		apiErr, workerErr := make(chan error, 1), make(chan error, 1)
		go func() { // API: cancel
			apiErr <- db.InTx(ctx, func(ctx context.Context) error {
				locked, err := repo.GetForUpdate(ctx, u.ID, p.ID)
				if err != nil {
					return err
				}
				close(apiHasPost)
				time.Sleep(300 * time.Millisecond) // let the worker reach for the target
				tg := locked.Targets[0]
				tg.Status = post.TargetCancelled
				return repo.UpdateTarget(ctx, &tg)
			})
		}()
		go func() { // worker: begin / lockedRun
			<-apiHasPost
			workerErr <- db.InTx(ctx, func(ctx context.Context) error {
				if _, ok, err := repo.LockTarget(ctx, p.Targets[0].ID); err != nil || !ok {
					return errors.Join(err, errors.New("target not locked"))
				}
				_, err := repo.GetForUpdate(ctx, u.ID, p.ID)
				return err
			})
		}()
		for _, ch := range []chan error{apiErr, workerErr} {
			if err := <-ch; err != nil {
				var pg *pgconn.PgError
				if errors.As(err, &pg) && pg.Code == "40P01" {
					t.Fatalf("round %d: deadlock between API and worker lock order: %v", round, err)
				}
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
}
