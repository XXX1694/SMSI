package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/emailtoken"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

var base = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func newUser(t *testing.T, db *postgres.DB, email string) *user.User {
	t.Helper()
	u := &user.User{Email: email, PasswordHash: "x", Status: user.StatusActive}
	if err := postgres.NewUsers(db).Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func mint(t *testing.T, r *postgres.EmailTokens, u *user.User, p emailtoken.Purpose, hash string, at time.Time) {
	t.Helper()
	tok := &emailtoken.Token{UserID: u.ID, Purpose: p, Hash: hash, Email: u.Email, ExpiresAt: at.Add(p.TTL()), CreatedAt: at}
	if err := r.Create(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
}

func TestEmailTokenConsumeIsAtomicSingleUse(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewEmailTokens(db), context.Background()
	u := newUser(t, db, "a@example.com")
	mint(t, r, u, emailtoken.VerifyEmail, "h1", base)

	var won, lost atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tok, err := r.Consume(ctx, emailtoken.VerifyEmail, "h1", base.Add(time.Minute)); err == nil {
				if tok.UserID != u.ID || tok.Email != u.Email {
					t.Errorf("wrong token row: %+v", tok)
				}
				won.Add(1)
			} else if errs.Is(err, errs.NotFound) {
				lost.Add(1)
			} else {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 || lost.Load() != 11 {
		t.Fatalf("won=%d lost=%d", won.Load(), lost.Load())
	}
}

func TestEmailTokenExpiryPurposeAndRetirement(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewEmailTokens(db), context.Background()
	u, other := newUser(t, db, "a@example.com"), newUser(t, db, "b@example.com")

	mint(t, r, u, emailtoken.ResetPassword, "reset-1", base)
	if _, err := r.Consume(ctx, emailtoken.VerifyEmail, "reset-1", base); !errs.Is(err, errs.NotFound) {
		t.Fatalf("other purpose redeemed: %v", err)
	}
	if _, err := r.Consume(ctx, emailtoken.ResetPassword, "reset-1", base.Add(emailtoken.ResetTTL)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("expired token redeemed: %v", err)
	}
	// A newer token retires the older one, but only for the same user and purpose.
	mint(t, r, other, emailtoken.ResetPassword, "reset-other", base)
	mint(t, r, u, emailtoken.VerifyEmail, "verify-1", base)
	mint(t, r, u, emailtoken.ResetPassword, "reset-2", base.Add(time.Minute))
	if _, err := r.Consume(ctx, emailtoken.ResetPassword, "reset-2", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("newest token: %v", err)
	}
	mint(t, r, u, emailtoken.ResetPassword, "reset-3", base.Add(3*time.Minute))
	if _, err := r.Consume(ctx, emailtoken.ResetPassword, "reset-3", base.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, h := range []struct {
		p emailtoken.Purpose
		h string
	}{{emailtoken.VerifyEmail, "verify-1"}, {emailtoken.ResetPassword, "reset-other"}} {
		if _, err := r.Consume(ctx, h.p, h.h, base.Add(5*time.Minute)); err != nil {
			t.Fatalf("%s should be untouched: %v", h.h, err)
		}
	}
}

func TestEmailTokenRetireAllAndLatest(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewEmailTokens(db), context.Background()
	u := newUser(t, db, "a@example.com")
	if at, err := r.LatestCreatedAt(ctx, u.ID, emailtoken.ResetPassword); err != nil || !at.IsZero() {
		t.Fatalf("no tokens: %v %v", at, err)
	}
	mint(t, r, u, emailtoken.ResetPassword, "r", base)
	if at, _ := r.LatestCreatedAt(ctx, u.ID, emailtoken.ResetPassword); !at.Equal(base) {
		t.Fatalf("latest %v", at)
	}
	if err := r.RetireAll(ctx, u.ID, emailtoken.ResetPassword, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Consume(ctx, emailtoken.ResetPassword, "r", base.Add(time.Minute)); !errs.Is(err, errs.NotFound) {
		t.Fatalf("retired token redeemed: %v", err)
	}
}

func TestDeleteAllSessionsKeepsOnlyTheExcepted(t *testing.T) {
	db := testutil.OpenDB(t)
	sess, ctx := postgres.NewSessions(db), context.Background()
	a, b := newUser(t, db, "a@example.com"), newUser(t, db, "b@example.com")
	mk := func(u *user.User, hash string) *auth.Session {
		s := &auth.Session{CreatedAt: base, UserID: u.ID, TokenHash: hash, CSRFToken: "c", ExpiresAt: base.Add(time.Hour)}
		if err := sess.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	keep, _, _ := mk(a, "a1"), mk(a, "a2"), mk(a, "a3")
	bs := mk(b, "b1")
	n, err := sess.DeleteAllForUser(ctx, a.ID, keep.ID)
	if err != nil || n != 2 {
		t.Fatalf("deleted %d: %v", n, err)
	}
	if _, err := sess.GetByTokenHash(ctx, "a1"); err != nil {
		t.Fatalf("kept session gone: %v", err)
	}
	if _, err := sess.GetByTokenHash(ctx, bs.TokenHash); err != nil {
		t.Fatalf("another user's session deleted: %v", err)
	}
	if n, _ := sess.DeleteAllForUser(ctx, a.ID, uuid.Nil); n != 1 {
		t.Fatalf("uuid.Nil must keep none, deleted %d", n)
	}
}

func TestUserPasswordAndVerification(t *testing.T) {
	db := testutil.OpenDB(t)
	r, ctx := postgres.NewUsers(db), context.Background()
	u := newUser(t, db, "a@example.com")
	got, _ := r.GetByID(ctx, u.ID)
	if got.EmailVerified() || got.Plan != "free" || got.DeletedAt != nil {
		t.Fatalf("defaults: %+v", got)
	}
	if err := r.MarkEmailVerified(ctx, u.ID, base); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkEmailVerified(ctx, u.ID, base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(ctx, u.ID)
	if got.EmailVerifiedAt == nil || !got.EmailVerifiedAt.Equal(base) {
		t.Fatalf("first verification time must stick: %v", got.EmailVerifiedAt)
	}
	if err := r.SetPassword(ctx, u.ID, "newhash"); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.GetByID(ctx, u.ID); got.PasswordHash != "newhash" {
		t.Fatal("password not set")
	}
	if err := r.RehashPassword(ctx, u.ID, "stale", "upgraded"); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.GetByID(ctx, u.ID); got.PasswordHash != "newhash" {
		t.Fatalf("rehash against a stale hash must be a no-op, got %q", got.PasswordHash)
	}
	if err := r.RehashPassword(ctx, u.ID, "newhash", "upgraded"); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.GetByID(ctx, u.ID); got.PasswordHash != "upgraded" {
		t.Fatalf("rehash against the current hash must apply, got %q", got.PasswordHash)
	}
	if err := r.SetPassword(ctx, uuid.New(), "x"); !errs.Is(err, errs.NotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}
