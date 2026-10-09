package auth

import (
	"errors"
	"testing"

	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/terms"
)

func TestRegisterRequiresAcceptedTerms(t *testing.T) {
	r := newRig(t, false)
	in := RegisterInput{Email: "new@example.com", Password: "correct horse battery"}
	_, _, err := r.svc.Register(ctx, in, ClientInfo{})
	var e *errs.Error
	if !errs.Is(err, errs.Validation) || !errors.As(err, &e) || e.Fields["accept_terms"] == "" {
		t.Fatalf("want VALIDATION_ERROR on accept_terms, got %v", err)
	}
	for _, u := range r.users.byID {
		if u.Email == "new@example.com" {
			t.Fatal("a user was created without accepting the terms")
		}
	}
}

func TestRegisterStoresTermsVersionAndTime(t *testing.T) {
	r := newRig(t, false)
	in := RegisterInput{Email: "new@example.com", Password: "correct horse battery", AcceptTerms: true}
	u, _, err := r.svc.Register(ctx, in, ClientInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if u.TermsVersion != terms.CurrentVersion || u.TermsAcceptedAt == nil || !u.TermsAcceptedAt.Equal(r.clock.now) {
		t.Fatalf("terms not recorded: %q %v", u.TermsVersion, u.TermsAcceptedAt)
	}
}
