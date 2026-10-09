package auth

import (
	"context"
	"log/slog"
	"net/url"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/mail"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/domain/user"
)

// Sign-in method names in /me's login_methods; the providers use their own ids.
const methodPassword = "password"

// SignInMethods is what the settings page shows: the linked providers and whether a password exists.
type SignInMethods struct {
	HasPassword bool
	Identities  []identity.Identity
}

func linkFailed(code string) CallbackResult {
	return CallbackResult{Redirect: SettingsPath + "?error=" + url.QueryEscape(code)}
}

func (s *Service) identitiesOf(ctx context.Context, userID uuid.UUID) ([]identity.Identity, error) {
	if s.social == nil {
		return nil, nil
	}
	return s.social.identities.ListByUser(ctx, userID)
}

// SignInMethods lists the session user's linked providers.
func (s *Service) SignInMethods(ctx context.Context, a actor.Actor) (SignInMethods, error) {
	if err := a.RequireSession(); err != nil {
		return SignInMethods{}, err
	}
	u, err := s.users.GetByID(ctx, a.UserID)
	if err != nil {
		return SignInMethods{}, err
	}
	list, err := s.identitiesOf(ctx, u.ID)
	return SignInMethods{HasPassword: u.HasPassword(), Identities: list}, err
}

// LoginMethods names the ways the user can sign in: "password" if there is one, then each linked provider.
func (s *Service) LoginMethods(ctx context.Context, u *user.User) ([]string, error) {
	out := []string{}
	if u.HasPassword() {
		out = append(out, methodPassword)
	}
	list, err := s.identitiesOf(ctx, u.ID)
	for _, i := range list {
		out = append(out, string(i.Provider))
	}
	return out, err
}

// StartLink begins connecting a provider to the session user. The flow is bound to that user; the callback is the one
// sign-in uses (SocialCallback) and finishes in completeLink.
func (s *Service) StartLink(ctx context.Context, a actor.Actor, id identity.Provider) (StartResult, error) {
	if err := a.RequireSession(); err != nil {
		return StartResult{}, err
	}
	if err := a.RequireNotDeleting(); err != nil {
		return StartResult{}, err
	}
	if _, err := s.socialProvider(id); err != nil {
		return StartResult{}, err
	}
	list, err := s.identitiesOf(ctx, a.UserID)
	if err != nil {
		return StartResult{}, err
	}
	for _, i := range list {
		if i.Provider == id {
			return StartResult{}, errs.Newf(errs.Conflict, "%s is already connected; disconnect it first to connect another account", providerNames[id])
		}
	}
	uid := a.UserID
	return s.startFlow(ctx, id, identity.IntentLink, &uid, SettingsPath)
}

// completeLink attaches the verified provider account to the user the flow was started for. The email the provider
// reports plays no part: the user is already signed in. An account that belongs to someone else is never moved.
func (s *Service) completeLink(ctx context.Context, fl *identity.Flow, c identity.Claims, ci ClientInfo) (CallbackResult, error) {
	u, err := s.users.GetByID(ctx, *fl.LinkUserID)
	if err != nil {
		return CallbackResult{}, err
	}
	if !usable(u) {
		return linkFailed(ErrAccountGone), nil
	}
	owner, err := s.social.identities.GetBySubject(ctx, c.Provider, c.Subject)
	if err == nil {
		if owner.UserID == u.ID {
			return CallbackResult{Redirect: fl.RedirectAfter}, nil // already connected: nothing to do
		}
		return linkFailed(ErrIdentityInUse), nil
	}
	if !errs.Is(err, errs.NotFound) {
		return CallbackResult{}, err
	}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		i := &identity.Identity{UserID: u.ID, Provider: c.Provider, Subject: c.Subject, Email: c.Email, EmailVerified: c.EmailVerified, LinkedAt: s.clock.Now()}
		if err := s.social.identities.Create(ctx, i); err != nil {
			return err
		}
		return s.audit.Record(ctx, userActor(u, ci), audit.ActionIdentityLinked, "user", u.ID.String(), map[string]any{"provider": string(c.Provider), "via": "settings"})
	})
	if errs.Is(err, errs.Conflict) { // lost a race: the account was taken, or the user connected the provider meanwhile
		return linkFailed(ErrIdentityInUse), nil
	}
	if err != nil {
		return CallbackResult{}, err
	}
	s.notifyIdentityLinked(ctx, u, c.Provider)
	return CallbackResult{Redirect: fl.RedirectAfter}, nil
}

// Unlink disconnects a provider from the session user. The last way to sign in cannot be removed: without a password
// and another provider the account would be locked out.
func (s *Service) Unlink(ctx context.Context, a actor.Actor, id identity.Provider) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	if s.social == nil || !id.Valid() {
		return errs.NotFoundf("sign-in method")
	}
	var u *user.User
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		// Two unlinks racing for the last two providers must not both pass the check below.
		if err := s.social.identities.LockUser(ctx, a.UserID); err != nil {
			return err
		}
		var err error
		if u, err = s.users.GetByID(ctx, a.UserID); err != nil {
			return err
		}
		list, err := s.social.identities.ListByUser(ctx, u.ID)
		if err != nil {
			return err
		}
		found, others := false, 0
		for _, i := range list {
			if i.Provider == id {
				found = true
			} else {
				others++
			}
		}
		if !found {
			return errs.NotFoundf("sign-in method")
		}
		if !u.HasPassword() && others == 0 {
			return errs.New(errs.Conflict, "keep at least one way to sign in: set a password or connect another provider first")
		}
		if err := s.social.identities.Delete(ctx, u.ID, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionIdentityUnlinked, "user", u.ID.String(), map[string]any{"provider": string(id)})
	})
	if err != nil {
		return err
	}
	s.notify(ctx, u, mail.IdentityUnlinked, mail.Data{Provider: providerNames[id], Link: s.webURL + "/settings"})
	return nil
}

// SetInitialPassword gives a user without a password their first one, so a social sign-up can also sign in with email.
// Having no password to confirm with, the user must have signed in within actor.FreshSessionWindow; otherwise
// REAUTH_REQUIRED. Changing an existing password is ChangePassword.
func (s *Service) SetInitialPassword(ctx context.Context, a actor.Actor, password string) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	if err := user.ValidatePassword(password); err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, a.UserID)
	if err != nil {
		return err
	}
	if u.HasPassword() {
		return errs.New(errs.Conflict, "this account already has a password; change it instead")
	}
	if !a.SessionIsFresh(s.clock.Now()) {
		return errs.New(errs.ReauthRequired, "sign in again to set a password")
	}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		revoked, keys, err := s.replacePassword(ctx, u.ID, password, a.SessionID, false)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionPasswordChanged, "user", u.ID.String(),
			map[string]any{"initial": true, "sessions_revoked": revoked, "keys_revoked": keys, "revoke_keys": false})
	})
	if err != nil {
		return err
	}
	s.notifyPasswordChanged(ctx, u, false)
	return nil
}

// notify queues a notice mail after the commit. A failure is logged and never fails the action that caused it.
func (s *Service) notify(ctx context.Context, u *user.User, template string, d mail.Data) {
	msg, err := mail.Render(template, u.Email, d)
	if err == nil {
		err = s.mail.Enqueue(ctx, msg)
	}
	if err != nil {
		s.log.WarnContext(ctx, template+" mail not queued", slog.String("user_id", u.ID.String()), slog.Any("error", err))
	}
}
