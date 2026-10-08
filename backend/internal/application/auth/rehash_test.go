package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/errs"
)

func TestLoginRehashesOutdatedHash(t *testing.T) {
	r := newRig(t, false)
	r.u.PasswordHash = "old:old password 1"
	if _, _, err := r.svc.Login(ctx, r.u.Email, "old password 1", ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if got := r.u.PasswordHash; got != "h:old password 1" {
		t.Fatalf("hash not upgraded, still %q", got)
	}
}

func TestLoginKeepsCurrentHash(t *testing.T) {
	r := newRig(t, false)
	if _, _, err := r.svc.Login(ctx, r.u.Email, "old password 1", ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if r.u.PasswordHash != "h:old password 1" {
		t.Fatalf("current hash must stay, got %q", r.u.PasswordHash)
	}
}

func TestWrongPasswordDoesNotRehash(t *testing.T) {
	r := newRig(t, false)
	r.u.PasswordHash = "old:old password 1"
	if _, _, err := r.svc.Login(ctx, r.u.Email, "nope", ClientInfo{}); err == nil {
		t.Fatal("wrong password must fail")
	}
	if r.u.PasswordHash != "old:old password 1" {
		t.Fatal("hash changed on a failed login")
	}
}

type failingSetPassword struct{ Users }

func (failingSetPassword) SetPassword(context.Context, uuid.UUID, string) error {
	return errors.New("db down")
}

func TestRehashFailureIsLoggedAndLoginSucceeds(t *testing.T) {
	r := newRig(t, false)
	r.u.PasswordHash = "old:old password 1"
	var buf bytes.Buffer
	r.svc.users = failingSetPassword{r.users}
	r.svc.log = slog.New(slog.NewTextHandler(&buf, nil))
	if _, _, err := r.svc.Login(ctx, r.u.Email, "old password 1", ClientInfo{}); err != nil {
		t.Fatalf("login must survive a rehash failure: %v", err)
	}
	if !strings.Contains(buf.String(), "password rehash failed") {
		t.Fatalf("failure not logged: %q", buf.String())
	}
}

type busyHasher struct{ plainHasher }

func (busyHasher) Verify(context.Context, string, string) (bool, error) {
	return false, errs.New(errs.RateLimited, "server is busy, retry shortly")
}

func TestLoginSurfacesBusyHasher(t *testing.T) {
	r := newRig(t, false)
	r.svc.hasher = busyHasher{}
	_, _, err := r.svc.Login(ctx, r.u.Email, "old password 1", ClientInfo{})
	if errs.CodeOf(err) != errs.RateLimited {
		t.Fatalf("want RATE_LIMITED so the client retries, got %v", err)
	}
}
