package auth

import (
	"context"

	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/domain/terms"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// PendingView is what the sign-up form shows before the account exists.
type PendingView struct {
	Provider    identity.Provider
	Email       string
	DisplayName string
	// Next is the in-app path to continue to once the account is created.
	Next string
}

func errTicketGone() error {
	return errs.New(errs.NotFound, "this sign-up has expired; start again from the sign-in page")
}

func ticketHash(raw string) (string, bool) {
	if raw == "" || len(raw) > 128 {
		return "", false
	}
	return crypto.SHA256Hex(raw), true
}

// PendingSignup reads a live sign-up ticket without using it. NOT_FOUND when the ticket is unknown, used or expired.
func (s *Service) PendingSignup(ctx context.Context, rawTicket string) (*PendingView, error) {
	h, ok := ticketHash(rawTicket)
	if !ok || s.social == nil {
		return nil, errTicketGone()
	}
	fl, err := s.social.flows.GetByTicket(ctx, h, s.clock.Now())
	if errs.Is(err, errs.NotFound) || (err == nil && fl.Pending == nil) {
		return nil, errTicketGone()
	}
	if err != nil {
		return nil, err
	}
	return &PendingView{Provider: fl.Provider, Email: fl.Pending.Email, DisplayName: fl.Pending.DisplayName, Next: fl.RedirectAfter}, nil
}

// CompleteSignupInput is the sign-up form.
type CompleteSignupInput struct {
	Ticket      string
	DisplayName string
	// AcceptTerms must be true: the caller confirms the current Terms and Privacy Policy (D-016).
	AcceptTerms bool
}

// CompleteSignup creates the account a callback found no owner for, and signs it in. The Terms are checked before the
// ticket is touched, and the ticket is redeemed in the same transaction as the user: a refused or failed completion
// rolls the redemption back, so the person can fix the form and try again. Two completions racing for one ticket, or
// for an email or identity someone else just took, are stopped by the ticket's delete and by the unique constraints.
func (s *Service) CompleteSignup(ctx context.Context, in CompleteSignupInput, ci ClientInfo) (*user.User, IssuedSession, string, error) {
	h, ok := ticketHash(in.Ticket)
	if !ok || s.social == nil {
		return nil, IssuedSession{}, "", errTicketGone()
	}
	if !in.AcceptTerms {
		return nil, IssuedSession{}, "", errs.Validationf("you must accept the Terms and the Privacy Policy").
			WithField("accept_terms", "must be accepted")
	}
	name, err := user.ValidateDisplayName(in.DisplayName)
	if err != nil {
		return nil, IssuedSession{}, "", err
	}
	now := s.clock.Now()
	var u *user.User
	var issued IssuedSession
	var next string
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		fl, err := s.social.flows.ConsumeTicket(ctx, h, now)
		if errs.Is(err, errs.NotFound) || (err == nil && fl.Pending == nil) {
			return errTicketGone()
		}
		if err != nil {
			return err
		}
		if err := s.refuseTakenIdentity(ctx, fl); err != nil {
			return err
		}
		u = &user.User{Email: fl.Pending.Email, DisplayName: name, Status: user.StatusActive, Plan: user.DefaultPlan,
			TermsAcceptedAt: &now, TermsVersion: terms.CurrentVersion}
		if err := s.users.Create(ctx, u); err != nil {
			return conflictFor(err, "an account with this email already exists; sign in and connect the provider in Settings")
		}
		// The provider vouched for this address (only authoritative ones reach a ticket), so it counts as verified.
		if err := s.users.MarkEmailVerified(ctx, u.ID, now); err != nil {
			return err
		}
		u.EmailVerifiedAt = &now
		i := &identity.Identity{UserID: u.ID, Provider: fl.Provider, Subject: fl.Pending.Subject, Email: fl.Pending.Email, EmailVerified: true, LinkedAt: now}
		if err := s.social.identities.Create(ctx, i); err != nil {
			return conflictFor(err, "this provider account is already linked to another account")
		}
		next = fl.RedirectAfter
		issued, err = s.startSession(ctx, u, ci, audit.ActionUserRegistered, map[string]any{"method": string(fl.Provider)})
		return err
	})
	if err != nil {
		return nil, IssuedSession{}, "", err
	}
	return u, issued, next, nil
}

// refuseTakenIdentity stops a completion when the provider account got an owner after the ticket was issued (a second
// browser finished first). Anything but a usable owner is refused as unavailable, never signed in.
func (s *Service) refuseTakenIdentity(ctx context.Context, fl *identity.Flow) error {
	i, err := s.social.identities.GetBySubject(ctx, fl.Provider, fl.Pending.Subject)
	if errs.Is(err, errs.NotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	owner, err := s.users.GetByID(ctx, i.UserID)
	if err != nil {
		return err
	}
	if !usable(owner) {
		return errs.New(errs.Forbidden, "this account is not available")
	}
	return errs.New(errs.Conflict, "this provider account already has an account; sign in instead")
}

// conflictFor turns a unique-constraint CONFLICT into a message the person can act on and keeps every other error.
func conflictFor(err error, msg string) error {
	if errs.Is(err, errs.Conflict) {
		return errs.New(errs.Conflict, msg)
	}
	return err
}
