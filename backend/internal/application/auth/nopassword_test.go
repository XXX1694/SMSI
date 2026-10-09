package auth

import (
	"context"
	"testing"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
)

// verifyLog records which stored hash every Verify call was made against.
type verifyLog struct {
	plainHasher
	against []string
}

func (v *verifyLog) Verify(ctx context.Context, p, enc string) (bool, error) {
	v.against = append(v.against, enc)
	return v.plainHasher.Verify(ctx, p, enc)
}

// socialOnly turns the rig's user into one without a password, as a social sign-up creates them.
func (r *rig) socialOnly(hash string) {
	r.u.PasswordHash = hash
}

func TestLoginOfPasswordlessUserCostsTheSameAsAnyOtherFailure(t *testing.T) {
	for _, hash := range []string{"", user.UnusablePasswordHash} {
		r := newRig(t, false)
		h := &verifyLog{}
		r.svc.hasher = h
		r.socialOnly(hash)

		_, _, err := r.svc.Login(ctx, r.u.Email, "any password at all", ClientInfo{})
		if !errs.Is(err, errs.Unauthenticated) || err.Error() != errs.New(errs.Unauthenticated, "Wrong email or password.").Error() {
			t.Fatalf("hash %q: want the generic credentials error, got %v", hash, err)
		}
		// Exactly one verification, against the dummy hash: the same work as for an unknown address.
		if len(h.against) != 1 || h.against[0] != r.svc.dummyHash {
			t.Fatalf("hash %q: verified against %q, want one check against the dummy hash", hash, h.against)
		}
		if len(r.sessions.ids) != 0 || len(r.audit.got) != 0 {
			t.Fatal("a failed login must not create a session or an audit entry")
		}

		h.against = nil
		_, _, _ = r.svc.Login(ctx, "nobody@example.com", "any password at all", ClientInfo{})
		if len(h.against) != 1 {
			t.Fatalf("unknown address: %d verifications, want 1", len(h.against))
		}
	}
}

func TestLoginOfPasswordlessUserSurfacesBusyHasherLikeEveryoneElse(t *testing.T) {
	r := newRig(t, false)
	r.svc.hasher = busyHasher{}
	r.socialOnly("")
	_, _, err := r.svc.Login(ctx, r.u.Email, "whatever password", ClientInfo{})
	if errs.CodeOf(err) != errs.RateLimited {
		t.Fatalf("a saturated hasher must answer the same retryable error for password-less accounts, got %v", err)
	}
}

func TestPasswordlessUserCannotChangePasswordButCanResetIt(t *testing.T) {
	r := newRig(t, false)
	r.socialOnly("")
	a := actor.Actor{UserID: r.u.ID, Type: actor.TypeUser, SessionID: r.addSession()}
	err := r.svc.ChangePassword(ctx, a, "", "brand new password", false)
	wantCode(t, err, errs.Conflict)
	if e, _ := errs.As(err); e.Fields["current_password"] == "" {
		t.Fatalf("the error must point at the current_password field: %+v", e)
	}
	if r.u.HasPassword() || len(r.mail.got) != 0 {
		t.Fatal("a rejected change must not set a password or send a notice")
	}

	// The reset link is the way to a first password until set-password exists.
	_ = r.forgot(r.u.Email)
	if err := r.svc.ResetPassword(ctx, tokenFrom(t, r.mail.got[0]), "brand new password", false, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.svc.Login(ctx, r.u.Email, "brand new password", ClientInfo{}); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestPasswordlessLoginNeverRehashes(t *testing.T) {
	r := newRig(t, false)
	r.socialOnly("")
	r.svc.rehashIfOutdated(ctx, r.u, "x")
	if r.u.PasswordHash != "" {
		t.Fatalf("rehash invented a hash: %q", r.u.PasswordHash)
	}
}
