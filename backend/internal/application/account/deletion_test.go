package account

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/storage"
)

type delRig struct {
	svc   *DeletionService
	w     *world
	store *storage.Memory
	mail  *mailSink
	queue *purgeQueued
	audit *auditLog
	clock *clockAt
	u     *user.User
}

func newDelRig(t *testing.T) *delRig {
	t.Helper()
	clk := &clockAt{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	r := &delRig{w: newWorld(clk), store: storage.NewMemory(), mail: &mailSink{}, queue: &purgeQueued{}, audit: &auditLog{}, clock: clk}
	r.u = r.w.addUser("owner@example.com")
	r.svc = NewDeletionService(DeletionDeps{Repo: r.w, Users: r.w, Passwords: r.w, Sessions: r.w, Keys: r.w, Posts: r.w, Queue: r.queue,
		Store: r.store, Mail: r.mail, Tx: passTx{}, Audit: r.audit, Clock: clk, WebBaseURL: "https://app.test/", Grace: 7 * 24 * time.Hour})
	return r
}

func (r *delRig) session() actor.Actor {
	return actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, ID: r.u.ID.String(), SessionID: uuid.New()}
}

func (r *delRig) seed(t *testing.T) (mediaKey, exportKey string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < purgeBatch+30; i++ { // more than one batch
		r.w.posts[r.u.ID] = append(r.w.posts[r.u.ID], uuid.New())
	}
	r.w.audits[r.u.ID] = 2*purgeBatch + 1
	mediaKey = "users/" + r.u.ID.String() + "/media/a.png"
	exportKey = "users/" + r.u.ID.String() + "/exports/e.zip"
	for _, k := range []string{mediaKey, exportKey, "users/other/media/keep.png"} {
		if err := r.store.Put(ctx, k, strings.NewReader("x"), 1, "image/png"); err != nil {
			t.Fatal(err)
		}
	}
	r.w.media[r.u.ID] = []MediaObject{{ID: uuid.New(), Key: mediaKey}}
	r.w.exports[r.u.ID] = []string{exportKey}
	return mediaKey, exportKey
}

func TestRequestNeedsSessionPasswordAndTypedEmail(t *testing.T) {
	r := newDelRig(t)
	ctx := context.Background()
	// API keys can never delete an account.
	key := actor.Actor{UserID: r.u.ID, Type: actor.TypeAPIKey, APIKeyID: uuid.New()}
	_, err := r.svc.Request(ctx, key, "secret", r.u.Email)
	_ = wantCode(t, err, errs.Forbidden)

	_, err = r.svc.Request(ctx, r.session(), "wrong", r.u.Email)
	if e := wantCode(t, err, errs.Validation); e.Fields["password"] == "" {
		t.Fatalf("no password field error: %v", e.Fields)
	}
	_, err = r.svc.Request(ctx, r.session(), "secret", "someone-else@example.com")
	if e := wantCode(t, err, errs.Validation); e.Fields["confirm"] == "" {
		t.Fatalf("no confirm field error: %v", e.Fields)
	}
	// Nothing was touched by the refused attempts.
	if r.w.sessions[r.u.ID] != 2 || r.w.keysLeft[r.u.ID] != 3 || len(r.w.scheduled) != 0 || len(r.mail.sent) != 0 {
		t.Fatalf("refused requests changed state: %+v", r.w)
	}
}

func TestRequestSchedulesRevokesAndNotifies(t *testing.T) {
	r := newDelRig(t)
	ctx := context.Background()
	s, err := r.svc.Request(ctx, r.session(), "secret", "  OWNER@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if want := r.clock.t.Add(7 * 24 * time.Hour); !s.PurgeAt.Equal(want) {
		t.Fatalf("purge at %v want %v", s.PurgeAt, want)
	}
	if r.w.sessions[r.u.ID] != 0 || r.w.keysLeft[r.u.ID] != 0 {
		t.Fatal("sessions and keys must be revoked at request time")
	}
	if got := strings.Join(r.w.order, ","); got != "unschedule,schedule" {
		t.Fatalf("posts must be stopped before anything is revoked: %s", got)
	}
	if r.mail.templates() != "account_deletion_scheduled" || !strings.Contains(r.mail.sent[0].Text, "https://app.test/login") {
		t.Fatalf("mail: %q", r.mail.templates())
	}
	if a := r.audit.actions(); len(a) != 1 || a[0] != audit.ActionDeletionScheduled {
		t.Fatalf("audit: %v", a)
	}
	// Still a normal active account until the grace period ends.
	if r.w.users[r.u.ID].Status != user.StatusActive {
		t.Fatal("account must stay active during the grace period")
	}
	_, err = r.svc.Request(ctx, r.session(), "secret", r.u.Email)
	_ = wantCode(t, err, errs.Conflict)
}

func TestCancelKeepsEverything(t *testing.T) {
	r := newDelRig(t)
	ctx := context.Background()
	r.seed(t)
	if _, err := r.svc.Request(ctx, r.session(), "secret", r.u.Email); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Cancel(ctx, r.session()); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Cancel(ctx, r.session()); err == nil {
		t.Fatal("cancelling twice must say nothing is scheduled")
	}
	r.clock.t = r.clock.t.Add(30 * 24 * time.Hour)
	if n, _ := r.svc.Sweep(ctx); n != 0 {
		t.Fatalf("sweep queued %d purges for a cancelled deletion", n)
	}
	if err := r.svc.Purge(ctx, r.u.ID); err != nil || len(r.w.posts[r.u.ID]) == 0 || r.w.users[r.u.ID] == nil {
		t.Fatalf("a purge of a cancelled deletion deleted data: %v", err)
	}
	// And the owner can ask again.
	if _, err := r.svc.Request(ctx, r.session(), "secret", r.u.Email); err != nil {
		t.Fatalf("second request: %v", err)
	}
}

func TestPurgeWaitsForTheGracePeriodThenDeletesInOrder(t *testing.T) {
	r := newDelRig(t)
	ctx := context.Background()
	mediaKey, exportKey := r.seed(t)
	other := r.w.addUser("other@example.com")
	r.w.posts[other.ID] = []uuid.UUID{uuid.New()}
	if _, err := r.svc.Request(ctx, r.session(), "secret", r.u.Email); err != nil {
		t.Fatal(err)
	}
	r.w.order = nil

	// Not yet due: nothing happens, whatever is queued.
	r.clock.t = r.clock.t.Add(6 * 24 * time.Hour)
	if n, _ := r.svc.Sweep(ctx); n != 0 {
		t.Fatalf("sweep before the end of the grace period queued %d", n)
	}
	if err := r.svc.Purge(ctx, r.u.ID); err != nil || len(r.w.posts[r.u.ID]) == 0 {
		t.Fatalf("early purge: %v", err)
	}

	r.clock.t = r.clock.t.Add(2 * 24 * time.Hour)
	if n, _ := r.svc.Sweep(ctx); n != 1 || r.queue.ids[0] != r.u.ID {
		t.Fatalf("sweep queued %v", r.queue.ids)
	}
	if err := r.svc.Purge(ctx, r.u.ID); err != nil {
		t.Fatal(err)
	}
	if r.w.users[r.u.ID] != nil || len(r.w.posts[r.u.ID]) != 0 || r.w.audits[r.u.ID] != 0 || len(r.w.media[r.u.ID]) != 0 {
		t.Fatalf("data left behind: %+v", r.w)
	}
	if r.store.Has(mediaKey) || r.store.Has(exportKey) || !r.store.Has("users/other/media/keep.png") {
		t.Fatal("storage must lose exactly this user's objects")
	}
	rec := r.w.records[r.u.ID]
	if rec.purged == nil || rec.counts.Posts != int64(purgeBatch+30) || rec.counts.Media != 1 {
		t.Fatalf("deletion record: %+v", rec)
	}
	// Posts (and what cascades from them) go before media, the user row goes last.
	got := strings.Join(r.w.order, ",")
	if !strings.HasPrefix(got, "batch:posts") || strings.Index(got, "media") < strings.LastIndex(got, "batch:posts") || !strings.HasSuffix(got, ",user") {
		t.Fatalf("order: %s", got)
	}
	if r.mail.templates() != "account_deletion_scheduled,account_deleted" || r.mail.sent[1].To != "owner@example.com" {
		t.Fatalf("mail: %s", r.mail.templates())
	}
	// Another tenant is untouched.
	if r.w.users[other.ID] == nil || len(r.w.posts[other.ID]) != 1 {
		t.Fatal("purge touched another user")
	}
}

func TestPurgeWaitsForARunningPublishAndCanResume(t *testing.T) {
	r := newDelRig(t)
	ctx := context.Background()
	r.seed(t)
	if _, err := r.svc.Request(ctx, r.session(), "secret", r.u.Email); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(8 * 24 * time.Hour)
	r.w.publish[r.u.ID] = true
	if err := r.svc.Purge(ctx, r.u.ID); err == nil || !strings.Contains(err.Error(), "must wait") {
		t.Fatalf("want a retryable error, got %v", err)
	}
	if len(r.w.posts[r.u.ID]) == 0 {
		t.Fatal("posts deleted under a running publish")
	}
	// The account is already unusable, and the sweep keeps it on the list until the purge finishes.
	if r.w.users[r.u.ID].Status != user.StatusDeleted {
		t.Fatal("claimed account must be marked deleted")
	}
	if n, _ := r.svc.Sweep(ctx); n != 1 {
		t.Fatalf("sweep must re-queue a half-done purge, got %d", n)
	}
	r.w.publish[r.u.ID] = false
	if err := r.svc.Purge(ctx, r.u.ID); err != nil || r.w.users[r.u.ID] != nil {
		t.Fatalf("resumed purge: %v", err)
	}
	// Running it again after success is a no-op.
	if err := r.svc.Purge(ctx, r.u.ID); err != nil {
		t.Fatalf("repeat purge: %v", err)
	}
	if got := r.w.records[r.u.ID].counts.Posts; got != int64(purgeBatch+30) {
		t.Fatalf("counts of the first attempt were overwritten: %d", got)
	}
}
