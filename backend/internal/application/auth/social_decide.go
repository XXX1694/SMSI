package auth

import (
	"context"
	"log/slog"
	"strings"

	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// decide looks up the two possible owners, applies the linking rules (identity.Decide) and carries out the result.
func (s *Service) decide(ctx context.Context, fl *identity.Flow, c identity.Claims, ci ClientInfo) (CallbackResult, error) {
	byIdentity, identityUser, err := s.ownerByIdentity(ctx, c)
	if err != nil {
		return CallbackResult{}, err
	}
	byEmail, emailUser, err := s.ownerByEmail(ctx, c)
	if err != nil {
		return CallbackResult{}, err
	}
	d := identity.Decide(c, byIdentity, byEmail)
	switch d.Action {
	case identity.SignIn:
		return s.signInKnown(ctx, fl, c, identityUser, ci)
	case identity.LinkAndSignIn:
		return s.linkAndSignIn(ctx, fl, c, emailUser, ci)
	case identity.SignUp:
		return s.startSignup(ctx, fl, c)
	default:
		return failed(string(d.Reason)), nil
	}
}

// usable is false for disabled, deleted and deletion-scheduled accounts: a pending deletion is a decision to leave,
// so a provider sign-in must not quietly reopen the account (D-019).
func usable(u *user.User) bool { return u.Status == user.StatusActive && u.DeletionScheduledAt == nil }

func (s *Service) ownerByIdentity(ctx context.Context, c identity.Claims) (*identity.Owner, *user.User, error) {
	i, err := s.social.identities.GetBySubject(ctx, c.Provider, c.Subject)
	if errs.Is(err, errs.NotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	u, err := s.users.GetByID(ctx, i.UserID)
	if err != nil {
		return nil, nil, err
	}
	return &identity.Owner{UserID: u.ID, Active: usable(u), EmailVerified: u.EmailVerified()}, u, nil
}

func (s *Service) ownerByEmail(ctx context.Context, c identity.Claims) (*identity.Owner, *user.User, error) {
	if c.Email == "" {
		return nil, nil, nil
	}
	u, err := s.users.GetByEmail(ctx, c.Email)
	if errs.Is(err, errs.NotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &identity.Owner{UserID: u.ID, Active: usable(u), EmailVerified: u.EmailVerified()}, u, nil
}

func (s *Service) signInKnown(ctx context.Context, fl *identity.Flow, c identity.Claims, u *user.User, ci ClientInfo) (CallbackResult, error) {
	var issued IssuedSession
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.social.identities.TouchLogin(ctx, u.ID, c.Provider, c.Email, c.EmailVerified, s.clock.Now()); err != nil {
			return err
		}
		var err error
		issued, err = s.startSession(ctx, u, ci, audit.ActionUserLogin, map[string]any{"method": string(c.Provider)})
		return err
	})
	if errs.Is(err, errs.NotFound) {
		return CallbackResult{}, errs.New(errs.Conflict, "the identity was unlinked meanwhile") // retried once by the caller
	}
	if err != nil {
		return CallbackResult{}, err
	}
	return CallbackResult{Redirect: fl.RedirectAfter, Session: &issued}, nil
}

func (s *Service) linkAndSignIn(ctx context.Context, fl *identity.Flow, c identity.Claims, u *user.User, ci ClientInfo) (CallbackResult, error) {
	var issued IssuedSession
	now := s.clock.Now()
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		i := &identity.Identity{UserID: u.ID, Provider: c.Provider, Subject: c.Subject, Email: c.Email, EmailVerified: c.EmailVerified, LinkedAt: now}
		if err := s.social.identities.Create(ctx, i); err != nil {
			return err
		}
		meta := map[string]any{"method": string(c.Provider)}
		if err := s.audit.Record(ctx, userActor(u, ci), audit.ActionIdentityLinked, "user", u.ID.String(), map[string]any{"provider": string(c.Provider), "via": "email_match"}); err != nil {
			return err
		}
		var err error
		issued, err = s.startSession(ctx, u, ci, audit.ActionUserLogin, meta)
		return err
	})
	if err != nil {
		return CallbackResult{}, err
	}
	return CallbackResult{Redirect: fl.RedirectAfter, Session: &issued}, nil
}

// startSignup keeps what the provider vouched for behind a one-time ticket. No user exists yet: the account is created
// only when the person accepts the Terms (D-016).
func (s *Service) startSignup(ctx context.Context, fl *identity.Flow, c identity.Claims) (CallbackResult, error) {
	ticket, err := crypto.RandomToken(32)
	if err != nil {
		return CallbackResult{}, err
	}
	name, _ := user.ValidateDisplayName(truncateRunes(c.DisplayName, 100))
	pending := identity.PendingSignup{Subject: c.Subject, Email: c.Email, DisplayName: name}
	if err := s.social.flows.SetPending(ctx, fl.ID, crypto.SHA256Hex(ticket), s.clock.Now().Add(TicketTTL), pending); err != nil {
		return CallbackResult{}, err
	}
	return CallbackResult{Redirect: SignupCompletePath, Ticket: ticket}, nil
}

func truncateRunes(s string, n int) string {
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n])
	}
	return strings.TrimSpace(s)
}

// logSocial logs a failed step. The error is the adapter's own text, which carries no token or address.
func (s *Service) logSocial(ctx context.Context, id identity.Provider, msg string, err error) {
	s.log.WarnContext(ctx, "social sign-in: "+msg, slog.String("provider", string(id)), slog.Any("error", err))
}
