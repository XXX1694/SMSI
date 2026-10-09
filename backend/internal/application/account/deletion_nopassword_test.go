package account

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
)

// socialSession is the password-less owner's session, signed in `age` ago.
func (r *delRig) socialSession(age time.Duration) actor.Actor {
	r.u.PasswordHash = ""
	a := r.session()
	a.SessionCreatedAt = r.clock.t.Add(-age)
	return a
}

func TestPasswordlessOwnerCanDeleteWithAFreshSession(t *testing.T) {
	r := newDelRig(t)
	for _, age := range []time.Duration{0, 9*time.Minute + 59*time.Second, actor.FreshSessionWindow} {
		r2 := newDelRig(t)
		if _, err := r2.svc.Request(context.Background(), r2.socialSession(age), "", r2.u.Email); err != nil {
			t.Fatalf("session %s old: %v", age, err)
		}
		if len(r2.w.scheduled) != 1 {
			t.Fatalf("session %s old: deletion not scheduled", age)
		}
	}
	// The typed email is still required.
	_, err := r.svc.Request(context.Background(), r.socialSession(time.Minute), "", "someone-else@example.com")
	if e := wantCode(t, err, errs.Validation); e.Fields["confirm"] == "" {
		t.Fatalf("no confirm field error: %v", e.Fields)
	}
}

func TestPasswordlessOwnerWithAStaleSessionMustSignInAgain(t *testing.T) {
	for name, age := range map[string]time.Duration{"just over the window": actor.FreshSessionWindow + time.Second, "days": 72 * time.Hour} {
		t.Run(name, func(t *testing.T) {
			r := newDelRig(t)
			_, err := r.svc.Request(context.Background(), r.socialSession(age), "", r.u.Email)
			_ = wantCode(t, err, errs.ReauthRequired) // 403 REAUTH_REQUIRED, not a 500
			if len(r.w.scheduled) != 0 || r.w.sessions[r.u.ID] != 2 || len(r.mail.sent) != 0 {
				t.Fatalf("a refused request changed state: %+v", r.w)
			}
		})
	}
	// A session with no creation time (never trust a zero value) is stale too.
	r := newDelRig(t)
	r.u.PasswordHash = ""
	_, err := r.svc.Request(context.Background(), actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: uuid.New()}, "", r.u.Email)
	_ = wantCode(t, err, errs.ReauthRequired)
}

func TestFreshSessionDoesNotLetAPasswordOwnerSkipThePassword(t *testing.T) {
	r := newDelRig(t)
	a := r.session()
	a.SessionCreatedAt = r.clock.t
	_, err := r.svc.Request(context.Background(), a, "", r.u.Email)
	if e := wantCode(t, err, errs.Validation); e.Fields["password"] == "" {
		t.Fatalf("a user with a password must still type it: %v", e.Fields)
	}
}
