package e2e

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/socialos/backend/internal/application/scheduler"
)

// TestCancelRacingPublishNeverDeadlocks is the regression test for issue #38: the API locked post -> target while the
// worker locked target -> post, so a cancel racing a due publish could deadlock (40P01) and surface as a 500.
// Rows are locked post first everywhere now; see "Lock order" in docs/ARCHITECTURE.md.
func TestCancelRacingPublishNeverDeadlocks(t *testing.T) {
	const rounds = 60
	w := newWorkerEnv(t)
	acc := w.connect("mock")
	ctx := context.Background()
	deadlocks := func() int {
		var n int
		if err := w.app.DB.Pool.QueryRow(ctx, `SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()`).Scan(&n); err != nil {
			t.Fatalf("read deadlock counter: %v", err)
		}
		return n
	}
	before := deadlocks()

	type round struct {
		postID string
		pl     scheduler.Payload
	}
	rs := make([]round, rounds)
	for i := range rs {
		p := w.c.must("POST", "/api/v1/posts", map[string]any{"content": "race", "social_account_ids": []string{acc},
			"scheduled_at": fmtTime(time.Now().Add(10 * time.Minute)), "schedule": true}, 201)
		id := p["id"].(string)
		// Make the job due now so Run publishes immediately instead of holding until scheduled_at.
		w.exec(`UPDATE scheduled_jobs SET run_at = now() - interval '1 minute' WHERE post_target_id IN (SELECT id FROM post_targets WHERE post_id = $1)`, id)
		rs[i] = round{postID: id, pl: w.activePayload(id)}
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	cancelStatus := make([]int, rounds)
	runErr := make([]error, rounds)
	for i := range rs {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cancelStatus[i] = w.c.do("POST", "/api/v1/posts/"+rs[i].postID+"/cancel", nil).status
		}()
		go func() {
			defer wg.Done()
			<-start
			runErr[i] = w.run(rs[i].pl, 0)
		}()
	}
	close(start)
	wg.Wait()

	if got := deadlocks() - before; got != 0 {
		t.Errorf("postgres reported %d deadlocks, want 0", got)
	}
	m := mockOf(t, w)
	for i, r := range rs {
		var pgErr *pgconn.PgError
		if errors.As(runErr[i], &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001") {
			t.Errorf("round %d: publish hit pg %s", i, pgErr.Code)
		}
		if s := cancelStatus[i]; s != 200 && s != 409 {
			t.Errorf("round %d: cancel returned %d, want 200 or 409 (never 500)", i, s)
		}
		st, tg := w.status(r.postID)
		calls := m.Calls(r.pl.TargetID.String())
		switch {
		case cancelStatus[i] == 200 && (st != "cancelled" || tg["status"] != "cancelled" || calls != 0):
			t.Errorf("round %d: cancel won but post=%s target=%v provider calls=%d", i, st, tg["status"], calls)
		case cancelStatus[i] == 409 && (st != "published" || calls != 1):
			t.Errorf("round %d: publish won but post=%s provider calls=%d", i, st, calls)
		}
	}
}
