package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

func TestPasswordlessUserRoundTripsAsNullHash(t *testing.T) {
	db := testutil.OpenDB(t)
	users, ctx := postgres.NewUsers(db), context.Background()
	u := &user.User{Email: "social@example.com", Status: user.StatusActive}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int](t, db, `SELECT count(*) FROM users WHERE id = '`+u.ID.String()+`' AND password_hash IS NULL`); n != 1 {
		t.Fatal("an empty hash must be stored as NULL, not as an empty string")
	}
	got, err := users.GetByEmail(ctx, "social@example.com")
	if err != nil || got.HasPassword() || got.PasswordHash != "" {
		t.Fatalf("passwordless user read back as %+v (%v)", got, err)
	}
	// Conditional rehash on a NULL hash must not invent one; SetPassword (reset flow) gives the user a password.
	if err := users.RehashPassword(ctx, u.ID, "", "new"); err != nil {
		t.Fatal(err)
	}
	if got, _ = users.GetByID(ctx, u.ID); got.HasPassword() {
		t.Fatal("rehash created a password")
	}
	if err := users.SetPassword(ctx, u.ID, "hash"); err != nil {
		t.Fatal(err)
	}
	if got, _ = users.GetByID(ctx, u.ID); !got.HasPassword() {
		t.Fatal("password not set")
	}
}

func link(t *testing.T, r *postgres.Identities, u *user.User, p identity.Provider, subject string) *identity.Identity {
	t.Helper()
	i := &identity.Identity{UserID: u.ID, Provider: p, Subject: subject, Email: "Mixed@Example.com", EmailVerified: true, LinkedAt: base}
	if err := r.Create(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	return i
}

func TestIdentitiesLifecycle(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewIdentities(db), context.Background()
	a, b := newUser(t, db, "ident-a@example.com"), newUser(t, db, "ident-b@example.com")

	g := link(t, r, a, identity.Google, "g-1")
	if g.ID == uuid.Nil || g.Email != "Mixed@Example.com" {
		t.Fatalf("created %+v", g) // citext keeps the case it was given
	}
	link(t, r, a, identity.GitHub, "42")

	got, err := r.GetBySubject(ctx, identity.Google, "g-1")
	if err != nil || got.UserID != a.ID || got.LastLoginAt != nil {
		t.Fatalf("lookup: %+v %v", got, err)
	}
	if _, err := r.GetBySubject(ctx, identity.GitHub, "g-1"); !errs.Is(err, errs.NotFound) {
		t.Fatalf("the same subject under another provider is a different account: %v", err)
	}

	// One external account belongs to one user; one user has one identity per provider.
	dup := &identity.Identity{UserID: b.ID, Provider: identity.Google, Subject: "g-1", LinkedAt: base}
	if err := r.Create(ctx, dup); !errs.Is(err, errs.Conflict) {
		t.Fatalf("second user linked the same external account: %v", err)
	}
	second := &identity.Identity{UserID: a.ID, Provider: identity.Google, Subject: "g-2", LinkedAt: base}
	if err := r.Create(ctx, second); !errs.Is(err, errs.Conflict) {
		t.Fatalf("one user linked two google accounts: %v", err)
	}
	if err := r.Create(ctx, &identity.Identity{UserID: a.ID, Provider: "myspace", Subject: "x", LinkedAt: base}); err == nil {
		t.Fatal("unknown provider accepted")
	}

	list, err := r.ListByUser(ctx, a.ID)
	// Same linked_at, so the order falls back to the provider name.
	if err != nil || len(list) != 2 || list[0].Provider != identity.GitHub || list[1].Provider != identity.Google {
		t.Fatalf("list: %+v %v", list, err)
	}

	at := base.Add(time.Hour)
	if err := r.TouchLogin(ctx, a.ID, identity.Google, "new@example.com", false, at); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetBySubject(ctx, identity.Google, "g-1")
	if got.LastLoginAt == nil || !got.LastLoginAt.Equal(at) || got.Email != "new@example.com" || got.EmailVerified {
		t.Fatalf("touch: %+v", got)
	}

	if err := r.Delete(ctx, a.ID, identity.Google); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, a.ID, identity.Google); !errs.Is(err, errs.NotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if list, _ = r.ListByUser(ctx, a.ID); len(list) != 1 {
		t.Fatalf("after unlink: %+v", list)
	}
	// Once unlinked, the external account can be claimed by someone else.
	if err := r.Create(ctx, dup); err != nil {
		t.Fatalf("relink to another user: %v", err)
	}
}

func TestIdentitiesDieWithTheirUser(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewIdentities(db), context.Background()
	u := newUser(t, db, "ident-gone@example.com")
	link(t, r, u, identity.GitHub, "777")
	if _, err := db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetBySubject(ctx, identity.GitHub, "777"); !errs.Is(err, errs.NotFound) {
		t.Fatalf("identity outlived its user: %v", err)
	}
}

func newFlow(state string, at time.Time) *identity.Flow {
	return &identity.Flow{Provider: identity.GitHub, Intent: identity.IntentLogin, StateHash: state, NonceHash: "nonce",
		CodeVerifierEnc: "enc", RedirectAfter: "/dashboard", ExpiresAt: at.Add(10 * time.Minute)}
}

func TestOAuthFlowStateIsSingleUse(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewOAuthFlows(db), context.Background()
	f := newFlow("state-1", base)
	if err := r.Create(ctx, f); err != nil || f.ID == uuid.Nil {
		t.Fatalf("create: %+v %v", f, err)
	}
	if err := r.Create(ctx, newFlow("state-1", base)); !errs.Is(err, errs.Conflict) {
		t.Fatalf("duplicate state: %v", err)
	}
	if _, err := r.ConsumeState(ctx, identity.Google, "state-1", base.Add(time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("a github state was redeemed at the google callback: %v", err)
	}
	if _, err := r.ConsumeState(ctx, identity.GitHub, "state-1", base.Add(11*time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("expired state redeemed: %v", err)
	}
	got, err := r.ConsumeState(ctx, identity.GitHub, "state-1", base.Add(time.Minute))
	if err != nil || got.UsedAt == nil || got.NonceHash != "nonce" || got.CodeVerifierEnc != "enc" || got.RedirectAfter != "/dashboard" || got.Pending != nil {
		t.Fatalf("consume: %+v %v", got, err)
	}
	if _, err := r.ConsumeState(ctx, identity.GitHub, "state-1", base.Add(time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("replayed state redeemed: %v", err)
	}

	// Concurrent callbacks with the same state: exactly one wins.
	if err := r.Create(ctx, newFlow("state-race", base)); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.ConsumeState(ctx, identity.GitHub, "state-race", base); err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d callbacks redeemed one state", wins.Load())
	}
}

func TestOAuthFlowLinkIntentNeedsAUser(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewOAuthFlows(db), context.Background()
	f := newFlow("state-link", base)
	f.Intent = identity.IntentLink
	if err := r.Create(ctx, f); err == nil {
		t.Fatal("a link flow without a user was accepted")
	}
	u := newUser(t, db, "flow-link@example.com")
	f.LinkUserID = &u.ID
	if err := r.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := r.ConsumeState(ctx, identity.GitHub, "state-link", base)
	if err != nil || got.LinkUserID == nil || *got.LinkUserID != u.ID || got.Intent != identity.IntentLink {
		t.Fatalf("%+v %v", got, err)
	}
	login := newFlow("state-login", base)
	login.LinkUserID = &u.ID
	if err := r.Create(ctx, login); err == nil {
		t.Fatal("a login flow bound to a user was accepted")
	}
}

func TestOAuthFlowTicketIsSingleUse(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewOAuthFlows(db), context.Background()
	f := newFlow("state-t", base)
	if err := r.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	pending := identity.PendingSignup{Subject: "42", Email: "new@example.com", DisplayName: "Ann"}
	if err := r.SetPending(ctx, f.ID, "ticket-1", base.Add(15*time.Minute), pending); !errs.Is(err, errs.NotFound) {
		t.Fatalf("a ticket was attached before the state was used: %v", err)
	}
	if _, err := r.ConsumeState(ctx, identity.GitHub, "state-t", base); err != nil {
		t.Fatal(err)
	}
	if err := r.SetPending(ctx, f.ID, "ticket-1", base.Add(15*time.Minute), pending); err != nil {
		t.Fatal(err)
	}
	if err := r.SetPending(ctx, f.ID, "ticket-2", base.Add(15*time.Minute), pending); !errs.Is(err, errs.NotFound) {
		t.Fatalf("a flow got a second ticket: %v", err)
	}

	got, err := r.GetByTicket(ctx, "ticket-1", base.Add(time.Minute))
	if err != nil || got.Pending == nil || *got.Pending != pending || got.TicketHash != "ticket-1" {
		t.Fatalf("peek: %+v %v", got, err)
	}
	if _, err := r.GetByTicket(ctx, "ticket-1", base.Add(16*time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("expired ticket readable: %v", err)
	}
	if _, err := r.ConsumeTicket(ctx, "ticket-1", base.Add(16*time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("expired ticket redeemed: %v", err)
	}
	if _, err := r.ConsumeTicket(ctx, "no-such-ticket", base); !errs.Is(err, errs.NotFound) {
		t.Fatalf("unknown ticket redeemed: %v", err)
	}
	if got, err = r.ConsumeTicket(ctx, "ticket-1", base.Add(2*time.Minute)); err != nil || got.Pending.Email != "new@example.com" {
		t.Fatalf("redeem: %+v %v", got, err)
	}
	if _, err := r.ConsumeTicket(ctx, "ticket-1", base.Add(2*time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("ticket redeemed twice: %v", err)
	}
}

func TestOAuthFlowsPurgeKeepsLiveTickets(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewOAuthFlows(db), context.Background()
	for _, s := range []string{"p-old", "p-ticket", "p-fresh"} {
		at := base
		if s == "p-fresh" {
			at = base.Add(time.Hour)
		}
		if err := r.Create(ctx, newFlow(s, at)); err != nil {
			t.Fatal(err)
		}
	}
	f, err := r.ConsumeState(ctx, identity.GitHub, "p-ticket", base)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetPending(ctx, f.ID, "t", base.Add(30*time.Minute), identity.PendingSignup{Subject: "1"}); err != nil {
		t.Fatal(err)
	}
	// 20 minutes in: p-old expired (state only), p-ticket's state expired but its ticket is live, p-fresh untouched.
	n, err := r.DeleteExpired(ctx, base.Add(20*time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("purged %d (%v), want only p-old", n, err)
	}
	if _, err := r.GetByTicket(ctx, "t", base.Add(20*time.Minute)); err != nil {
		t.Fatalf("a live ticket was purged: %v", err)
	}
	if n, _ = r.DeleteExpired(ctx, base.Add(40*time.Minute)); n != 1 {
		t.Fatalf("purged %d after the ticket expired, want 1", n)
	}
}
