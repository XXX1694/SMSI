package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	quotasvc "github.com/socialos/backend/internal/application/quota"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/quota"
	"github.com/socialos/backend/internal/infrastructure/clock"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

// The upload gate is in memory per instance, so the database must hold the line on its own: concurrent transactions of
// EnforceMedia + Create for one user, with room for exactly three files.
func TestMediaQuotaHoldsAcrossConcurrentTransactions(t *testing.T) {
	db := testutil.OpenDB(t)
	ctx := context.Background()
	u := newUser(t, db, "media-race@example.com")
	repo := postgres.NewMedia(db)
	q := quotasvc.NewService(postgres.NewQuota(db), quota.Limits{Accounts: -1, PostsPerMonth: -1, MediaBytes: 3 << 20}, clock.System{})

	const files = 8
	var ok, refused, other atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < files; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := &media.Media{ID: uuid.New(), UserID: u.ID, Kind: media.KindImage, MimeType: "image/png", SizeBytes: 1 << 20,
				Status: media.StatusReady, StorageKey: "users/" + u.ID.String() + "/media/" + uuid.NewString() + ".png"}
			err := db.InTx(ctx, func(ctx context.Context) error {
				if err := q.EnforceMedia(ctx, u.ID, m.SizeBytes); err != nil {
					return err
				}
				return repo.Create(ctx, m)
			})
			switch {
			case err == nil:
				ok.Add(1)
			case errs.Is(err, errs.QuotaExceeded):
				refused.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	used, err := postgres.NewQuota(db).SumMediaBytes(ctx, u.ID)
	if err != nil || ok.Load() != 3 || refused.Load() != files-3 || other.Load() != 0 || used != 3<<20 {
		t.Fatalf("ok=%d refused=%d other=%d used=%d err=%v; want 3 ok, 5 refused, 3 MiB used", ok.Load(), refused.Load(), other.Load(), used, err)
	}
}
