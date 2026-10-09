package actor

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
)

func TestScopeAuthorization(t *testing.T) {
	uid := uuid.New()
	key := Actor{UserID: uid, Type: TypeAPIKey, APIKeyID: uuid.New(), Scopes: []apikey.Scope{apikey.PostsRead, apikey.PostsWrite}}
	if err := key.Require(apikey.PostsRead); err != nil {
		t.Fatalf("expected granted: %v", err)
	}
	if err := key.Require(apikey.PostsPublish); !errs.Is(err, errs.InsufficientScope) {
		t.Fatalf("expected INSUFFICIENT_SCOPE, got %v", err)
	}
	if err := key.RequireSession(); !errs.Is(err, errs.Forbidden) {
		t.Fatalf("api key must not pass session check: %v", err)
	}
	sess := Actor{UserID: uid, Type: TypeUser, SessionID: uuid.New()}
	for _, s := range apikey.AllScopes() {
		if err := sess.Require(s); err != nil {
			t.Fatalf("session should have %s", s)
		}
	}
	if err := sess.RequireSession(); err != nil {
		t.Fatal(err)
	}
	if err := (Actor{}).Require(apikey.PostsRead); !errs.Is(err, errs.Unauthenticated) {
		t.Fatalf("anonymous must be unauthenticated: %v", err)
	}
	if len(key.EffectiveScopes()) != 2 || len(sess.EffectiveScopes()) != len(apikey.AllScopes()) {
		t.Fatal("effective scopes wrong")
	}
}

func TestContextRoundTrip(t *testing.T) {
	a := Scheduler(uuid.New())
	got, ok := From(With(context.Background(), a))
	if !ok || got.UserID != a.UserID || got.Type != TypeScheduler {
		t.Fatal("round trip failed")
	}
	if _, ok := From(context.Background()); ok {
		t.Fatal("expected no actor")
	}
}

func TestRequireNotDeleting(t *testing.T) {
	if err := (Actor{UserID: uuid.New(), Type: TypeUser, DeletionScheduled: true}).RequireNotDeleting(); !errs.Is(err, errs.Conflict) {
		t.Fatalf("a session of an account being deleted: %v", err)
	}
	if err := (Actor{UserID: uuid.New(), Type: TypeAPIKey, DeletionScheduled: true}).RequireNotDeleting(); !errs.Is(err, errs.Conflict) {
		t.Fatalf("an API key of an account being deleted: %v", err)
	}
	for _, a := range []Actor{{Type: TypeUser}, Scheduler(uuid.New()), System(uuid.New(), "account_deletion")} {
		a.DeletionScheduled = a.Type == TypeScheduler || a.Type == TypeSystem
		if err := a.RequireNotDeleting(); err != nil {
			t.Fatalf("%s must pass: %v", a.Type, err)
		}
	}
}
