package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/mail"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/emailtoken"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// maxRawToken bounds what is hashed and looked up; real tokens are 43 characters.
const maxRawToken = 128

func invalidLink() error { return errs.Validationf("link is invalid or has expired") }

// link builds a mailed link. The token goes in the URL fragment, which browsers
// never send to servers or in the Referer header, so it stays out of access logs.
func (s *Service) link(path, rawToken string) string {
	return s.webURL + path + "#token=" + rawToken
}

func humanTTL(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()))
}

// issueToken stores a fresh token (retiring older ones) and queues the mail.
// The raw token only ever lives in the rendered message.
func (s *Service) issueToken(ctx context.Context, u *user.User, p emailtoken.Purpose, tmpl, path string) error {
	raw, err := crypto.RandomToken(32)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	t := &emailtoken.Token{UserID: u.ID, Purpose: p, Hash: crypto.SHA256Hex(raw), Email: u.Email,
		ExpiresAt: now.Add(p.TTL()), CreatedAt: now}
	if err := s.tokens.Create(ctx, t); err != nil {
		return err
	}
	msg, err := mail.Render(tmpl, u.Email, mail.Data{Link: s.link(path, raw), ExpiresIn: humanTTL(p.TTL())})
	if err != nil {
		return err
	}
	return s.mail.Enqueue(ctx, msg)
}

// IssueVerification mails a verification link. It never fails the caller: a
// down queue is logged and the user can use "resend".
func (s *Service) IssueVerification(ctx context.Context, u *user.User) {
	if u.EmailVerified() {
		return
	}
	if err := s.issueToken(ctx, u, emailtoken.VerifyEmail, mail.VerifyEmail, "/verify-email"); err != nil {
		s.log.WarnContext(ctx, "verification mail not queued", slog.String("user_id", u.ID.String()), slog.Any("error", err))
	}
}

// ResendVerification mails a new link to the session user. At most one per cooldown.
func (s *Service) ResendVerification(ctx context.Context, a actor.Actor) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, a.UserID)
	if err != nil {
		return err
	}
	if u.EmailVerified() {
		return errs.New(errs.Conflict, "email is already verified")
	}
	if s.coolingDown(ctx, u.ID, emailtoken.VerifyEmail) {
		return errs.New(errs.RateLimited, "a verification email was sent a moment ago; wait a minute and try again")
	}
	return s.issueToken(ctx, u, emailtoken.VerifyEmail, mail.VerifyEmail, "/verify-email")
}

func (s *Service) coolingDown(ctx context.Context, uid uuid.UUID, p emailtoken.Purpose) bool {
	last, err := s.tokens.LatestCreatedAt(ctx, uid, p)
	return err == nil && !last.IsZero() && s.clock.Now().Sub(last) < emailtoken.Cooldown
}

// VerifyEmail redeems a verification token once.
func (s *Service) VerifyEmail(ctx context.Context, rawToken string, ci ClientInfo) error {
	if rawToken == "" || len(rawToken) > maxRawToken {
		return invalidLink()
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		t, err := s.tokens.Consume(ctx, emailtoken.VerifyEmail, crypto.SHA256Hex(rawToken), now)
		if errs.Is(err, errs.NotFound) {
			return invalidLink()
		}
		if err != nil {
			return err
		}
		u, err := s.users.GetByID(ctx, t.UserID)
		if err != nil {
			return err
		}
		if !strings.EqualFold(u.Email, t.Email) {
			return invalidLink() // the address changed after the mail went out
		}
		if err := s.users.MarkEmailVerified(ctx, u.ID, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, userActor(u, ci), audit.ActionEmailVerified, "user", u.ID.String(), nil)
	})
}

// ForgotPassword mails a reset link when the address belongs to an active
// account. It returns nil in every case the caller could otherwise use to
// learn whether the address is registered: unknown address, cooldown, or a
// failed enqueue (logged).
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	norm, err := user.NormalizeEmail(email)
	if err != nil {
		return nil
	}
	u, err := s.users.GetByEmail(ctx, norm)
	if errs.Is(err, errs.NotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if u.Status != user.StatusActive || s.coolingDown(ctx, u.ID, emailtoken.ResetPassword) {
		return nil
	}
	if err := s.issueToken(ctx, u, emailtoken.ResetPassword, mail.ResetPassword, "/reset-password"); err != nil {
		s.log.WarnContext(ctx, "reset mail not queued", slog.String("user_id", u.ID.String()), slog.Any("error", err))
	}
	return nil
}

// ResetPassword redeems a reset token, sets the password and signs the user out everywhere.
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string, ci ClientInfo) error {
	if rawToken == "" || len(rawToken) > maxRawToken {
		return invalidLink()
	}
	if err := user.ValidatePassword(newPassword); err != nil {
		return err // before consuming: a weak password must not burn the link
	}
	var u *user.User
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		t, err := s.tokens.Consume(ctx, emailtoken.ResetPassword, crypto.SHA256Hex(rawToken), now)
		if errs.Is(err, errs.NotFound) {
			return invalidLink()
		}
		if err != nil {
			return err
		}
		if u, err = s.users.GetByID(ctx, t.UserID); err != nil {
			return err
		}
		if u.Status != user.StatusActive || !strings.EqualFold(u.Email, t.Email) {
			return invalidLink()
		}
		revoked, err := s.replacePassword(ctx, u.ID, newPassword, uuid.Nil)
		if err != nil {
			return err
		}
		// Opening the mailed link proves control of the address.
		if err := s.users.MarkEmailVerified(ctx, u.ID, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, userActor(u, ci), audit.ActionPasswordReset, "user", u.ID.String(),
			map[string]any{"sessions_revoked": revoked})
	})
	if err != nil {
		return err
	}
	s.notifyPasswordChanged(ctx, u)
	return nil
}

// ChangePassword changes the session user's password after re-checking the current one.
// Every other session is signed out; the caller's stays.
func (s *Service) ChangePassword(ctx context.Context, a actor.Actor, current, next string) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	if err := user.ValidatePassword(next); err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, a.UserID)
	if err != nil {
		return err
	}
	if ok, err := s.hasher.Verify(current, u.PasswordHash); err != nil || !ok {
		return errs.Validationf("current password is incorrect").WithField("current_password", "incorrect")
	}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		revoked, err := s.replacePassword(ctx, u.ID, next, a.SessionID)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionPasswordChanged, "user", u.ID.String(),
			map[string]any{"sessions_revoked": revoked})
	})
	if err != nil {
		return err
	}
	s.notifyPasswordChanged(ctx, u)
	return nil
}

// replacePassword stores the new hash, retires pending reset links and deletes
// every session of the user except `keep` (uuid.Nil keeps none). It returns the
// number of sessions removed.
func (s *Service) replacePassword(ctx context.Context, uid uuid.UUID, password string, keep uuid.UUID) (int64, error) {
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return 0, err
	}
	if err := s.users.SetPassword(ctx, uid, hash); err != nil {
		return 0, err
	}
	if err := s.tokens.RetireAll(ctx, uid, emailtoken.ResetPassword, s.clock.Now()); err != nil {
		return 0, err
	}
	return s.sessions.DeleteAllForUser(ctx, uid, keep)
}

func (s *Service) notifyPasswordChanged(ctx context.Context, u *user.User) {
	msg, err := mail.Render(mail.PasswordChanged, u.Email, mail.Data{})
	if err == nil {
		err = s.mail.Enqueue(ctx, msg)
	}
	if err != nil {
		s.log.WarnContext(ctx, "password-changed mail not queued", slog.String("user_id", u.ID.String()), slog.Any("error", err))
	}
}
