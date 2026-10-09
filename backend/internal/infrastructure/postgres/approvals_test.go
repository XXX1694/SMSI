package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/approvals"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

var t0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func approvalRepo(t *testing.T) (*postgres.Approvals, *postgres.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	db, err := postgres.Open(ctx, testutil.FreshDatabase(t), 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var users [2]uuid.UUID
	for i, email := range []string{"a@example.com", "b@example.com"} {
		if err := db.Pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, email).Scan(&users[i]); err != nil {
			t.Fatal(err)
		}
	}
	return postgres.NewApprovals(db), db, users[0], users[1]
}

func newApproval(user uuid.UUID, resource string) *approval.Approval {
	return &approval.Approval{ID: uuid.New(), UserID: user, ActorType: "api_key", ActorID: "key-1", ActorLabel: "agent",
		Action: approval.ActionPostPublish, ResourceType: "post", ResourceID: resource, Fingerprint: "fp-" + resource,
		Summary: map[string]any{"title": "Hi"}, Status: approval.StatusPending, ExpiresAt: t0.Add(10 * time.Minute), CreatedAt: t0}
}

func bindingFor(a *approval.Approval) approvals.Binding {
	return approvals.Binding{ActorType: a.ActorType, ActorID: a.ActorID, Action: a.Action, ResourceType: a.ResourceType,
		ResourceID: a.ResourceID, Fingerprint: a.Fingerprint}
}

func TestApprovalsLifecycleAndTenantScope(t *testing.T) {
	repo, _, alice, bob := approvalRepo(t)
	ctx := context.Background()
	a := newApproval(alice, "p1")
	if err := repo.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindPending(ctx, alice, bindingFor(a), t0)
	if err != nil || got.ID != a.ID || got.Summary["title"] != "Hi" {
		t.Fatalf("find: %+v %v", got, err)
	}
	if _, err := repo.FindPending(ctx, alice, bindingFor(a), t0.Add(11*time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("an expired approval is not pending: %v", err)
	}
	// Bob sees and decides nothing of Alice's.
	if _, err := repo.Get(ctx, bob, a.ID); !errs.Is(err, errs.NotFound) {
		t.Fatalf("get as bob: %v", err)
	}
	if ok, _ := repo.Decide(ctx, bob, a.ID, approval.StatusApproved, t0); ok {
		t.Fatal("bob approved alice's approval")
	}
	if n, _ := repo.CountPending(ctx, bob, t0); n != 0 {
		t.Fatalf("bob counts %d", n)
	}
	// Not consumable while pending.
	if ok, _ := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0); ok {
		t.Fatal("a pending approval was consumed")
	}
	if ok, err := repo.Decide(ctx, alice, a.ID, approval.StatusApproved, t0); !ok || err != nil {
		t.Fatalf("decide: %v %v", ok, err)
	}
	if ok, _ := repo.Decide(ctx, alice, a.ID, approval.StatusDenied, t0); ok {
		t.Fatal("a decided approval was decided again")
	}
	wrong := bindingFor(a)
	wrong.Fingerprint = "other"
	if ok, _ := repo.Consume(ctx, alice, a.ID, wrong, t0); ok {
		t.Fatal("consumed with another fingerprint")
	}
	if ok, _ := repo.Consume(ctx, bob, a.ID, bindingFor(a), t0); ok {
		t.Fatal("bob consumed alice's approval")
	}
	if ok, _ := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0.Add(10*time.Minute)); ok {
		t.Fatal("consumed at the deadline")
	}
	if ok, err := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0.Add(time.Minute)); !ok || err != nil {
		t.Fatalf("consume: %v %v", ok, err)
	}
	if ok, _ := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0.Add(time.Minute)); ok {
		t.Fatal("consumed twice")
	}
}

func TestApprovalConsumeHasExactlyOneWinner(t *testing.T) {
	repo, _, alice, _ := approvalRepo(t)
	ctx := context.Background()
	a := newApproval(alice, "p1")
	if err := repo.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Decide(ctx, alice, a.ID, approval.StatusApproved, t0); !ok || err != nil {
		t.Fatal(ok, err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0); err == nil && ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d concurrent consumers succeeded, want exactly 1", wins.Load())
	}
}

func TestApprovalConsumeRollsBackWithTheCallersTransaction(t *testing.T) {
	repo, db, alice, _ := approvalRepo(t)
	ctx := context.Background()
	a := newApproval(alice, "p1")
	if err := repo.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Decide(ctx, alice, a.ID, approval.StatusApproved, t0); err != nil {
		t.Fatal(err)
	}
	errBoom := errs.New(errs.Internal, "action failed after consume")
	err := db.InTx(ctx, func(ctx context.Context) error {
		if ok, err := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0); !ok || err != nil {
			t.Fatalf("consume in tx: %v %v", ok, err)
		}
		return errBoom
	})
	if err != errBoom {
		t.Fatal(err)
	}
	if ok, _ := repo.Consume(ctx, alice, a.ID, bindingFor(a), t0); !ok {
		t.Fatal("a failed action must not burn the approval")
	}
}

func TestNewApprovalSurvivesTheRollbackOfTheRequestThatMadeIt(t *testing.T) {
	repo, db, alice, _ := approvalRepo(t)
	ctx := context.Background()
	a := newApproval(alice, "p1")
	_ = db.InTx(ctx, func(ctx context.Context) error {
		if err := repo.Create(db.WithoutTx(ctx), a); err != nil {
			t.Fatal(err)
		}
		return errs.New(errs.ApprovalRequired, "428 rolls the surrounding transaction back")
	})
	if _, err := repo.Get(ctx, alice, a.ID); err != nil {
		t.Fatalf("the pending approval vanished with the transaction: %v", err)
	}
}

func TestApprovalsListPagesNewestFirstAndFiltersOpenOnes(t *testing.T) {
	repo, _, alice, bob := approvalRepo(t)
	ctx := context.Background()
	var ids []uuid.UUID
	for i, res := range []string{"p1", "p2", "p3"} {
		a := newApproval(alice, res)
		a.CreatedAt = t0.Add(time.Duration(i) * time.Second)
		if err := repo.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.ID)
	}
	if err := repo.Create(ctx, newApproval(bob, "p1")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Decide(ctx, alice, ids[0], approval.StatusDenied, t0); err != nil {
		t.Fatal(err)
	}
	open, err := repo.List(ctx, alice, true, t0, port.Page{Limit: 10})
	if err != nil || len(open) != 2 || open[0].ID != ids[2] || open[1].ID != ids[1] {
		t.Fatalf("open: %v %v", open, err)
	}
	all, _ := repo.List(ctx, alice, false, t0, port.Page{Limit: 2})
	if len(all) != 2 || all[0].ID != ids[2] {
		t.Fatalf("all page 1: %v", all)
	}
	next, _ := repo.List(ctx, alice, false, t0, port.Page{Limit: 2, Cursor: &port.Cursor{At: all[1].CreatedAt, ID: all[1].ID}})
	if len(next) != 1 || next[0].ID != ids[0] {
		t.Fatalf("all page 2: %v", next)
	}
	expired, _ := repo.List(ctx, alice, true, t0.Add(time.Hour), port.Page{Limit: 10})
	if len(expired) != 0 {
		t.Fatalf("expired approvals listed as open: %v", expired)
	}
}
