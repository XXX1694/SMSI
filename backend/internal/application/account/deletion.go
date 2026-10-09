package account

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/adapters/mail"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
)

// DefaultGrace is how long an account stays recoverable after deletion was requested (ACCOUNT_DELETION_GRACE_DAYS).
const DefaultGrace = 7 * 24 * time.Hour

// DeletionDeps bundles the deletion service's dependencies.
type DeletionDeps struct {
	Repo      DeletionRepo
	Users     UserReader
	Passwords Passwords
	Sessions  Revoker
	Keys      KeyRevoker
	Posts     PostStopper
	Queue     PurgeQueue
	Store     ObjectStore
	// Prefixes deletes everything under a key prefix; nil only in tests whose storage cannot list.
	Prefixes PrefixDeleter
	Mail     port.MailQueue
	Tx       port.TxRunner
	Audit    port.AuditRecorder
	Clock    port.Clock
	Log      *slog.Logger
	// WebBaseURL is where the mails link to.
	WebBaseURL string
	// Grace is the period between the request and the purge. Zero means DefaultGrace.
	Grace time.Duration
}

// DeletionService schedules, cancels and carries out account deletion.
type DeletionService struct{ d DeletionDeps }

// NewDeletionService creates the service.
func NewDeletionService(d DeletionDeps) *DeletionService {
	if d.Grace <= 0 {
		d.Grace = DefaultGrace
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	d.WebBaseURL = strings.TrimRight(d.WebBaseURL, "/")
	return &DeletionService{d: d}
}

// Schedule is what the owner gets back from Request.
type Schedule struct {
	PurgeAt time.Time
}

// Request schedules the deletion of the session user's account. It needs a browser session (an API key can never
// delete an account), the current password and the account's email typed as confirmation. Sessions and API keys are
// revoked at once, scheduled posts go back to drafts, and the data is purged after the grace period unless the owner
// signs in and cancels.
func (s *DeletionService) Request(ctx context.Context, a actor.Actor, password, confirmEmail string) (*Schedule, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	u, err := s.d.Users.GetByID(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	if u.DeletionScheduledAt != nil {
		return nil, errs.New(errs.Conflict, "deletion of this account is already scheduled")
	}
	if err := s.reauthenticate(ctx, u, password, confirmEmail); err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	purgeAt := now.Add(s.d.Grace)
	// Access ends first, in one transaction: once it commits no session or key can schedule anything, and the account
	// refuses scheduling and publishing while the deletion is pending (RequireNotDeleting, the publisher's owner check).
	err = s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.d.Repo.Schedule(ctx, u.ID, now, purgeAt); err != nil {
			return err
		}
		sessions, err := s.d.Sessions.DeleteAllForUser(ctx, u.ID, uuid.Nil)
		if err != nil {
			return err
		}
		keys, err := s.d.Keys.RevokeAllForUser(ctx, u.ID, now)
		if err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, a, audit.ActionDeletionScheduled, "user", u.ID.String(),
			map[string]any{"purge_at": purgeAt.UTC().Format(time.RFC3339), "sessions_revoked": sessions, "keys_revoked": keys})
	})
	if err != nil {
		return nil, err
	}
	// Then the scheduled posts go back to drafts. A failure is not fatal: the publisher skips them anyway and the purge
	// stops them again; it is logged so it can be looked at.
	if _, err := s.d.Posts.UnscheduleAll(ctx, u.ID); err != nil {
		s.d.Log.WarnContext(ctx, "scheduled posts not stopped at deletion request", slog.String("user_id", u.ID.String()), slog.Any("error", err))
	}
	s.notify(ctx, u.Email, mail.AccountDeletionScheduled, mail.Data{Link: s.d.WebBaseURL + "/login", Date: purgeAt.UTC().Format("2 January 2006, 15:04 UTC")})
	return &Schedule{PurgeAt: purgeAt}, nil
}

// reauthenticate checks the password and the typed confirmation. Both failures are field errors, so the form can say
// which one is wrong; a rate-limited hasher passes through unchanged.
func (s *DeletionService) reauthenticate(ctx context.Context, u *user.User, password, confirmEmail string) error {
	ok, err := s.d.Passwords.Verify(ctx, password, u.PasswordHash)
	if errs.CodeOf(err) == errs.RateLimited {
		return err
	}
	if err != nil {
		// Not a wrong password: a broken hash or a hasher failure must not look like one.
		s.d.Log.ErrorContext(ctx, "password check failed during account deletion", slog.String("user_id", u.ID.String()), slog.Any("error", err))
		return errs.Wrap(errs.Internal, "could not check the password", err)
	}
	if !ok {
		return errs.Validationf("the password is incorrect").WithField("password", "incorrect")
	}
	if !strings.EqualFold(strings.TrimSpace(confirmEmail), u.Email) {
		return errs.Validationf("type your email address exactly to confirm").WithField("confirm", "does not match your email")
	}
	return nil
}

// Cancel stops a scheduled deletion. The account has been intact the whole time, so nothing needs restoring (scheduled
// posts stay drafts; API keys and sessions stay revoked).
func (s *DeletionService) Cancel(ctx context.Context, a actor.Actor) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	return s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.d.Repo.Cancel(ctx, a.UserID); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, a, audit.ActionDeletionCancelled, "user", a.UserID.String(), nil)
	})
}

// Sweep queues a purge for every account whose grace period is over, and for those a purge left half done. It is
// idempotent and runs hourly in the worker.
func (s *DeletionService) Sweep(ctx context.Context) (int, error) {
	ids, err := s.d.Repo.Due(ctx, s.d.Clock.Now(), 100)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.d.Queue.EnqueuePurge(ctx, id); err != nil {
			s.d.Log.WarnContext(ctx, "purge not queued", slog.String("user_id", id.String()), slog.Any("error", err))
		}
	}
	return len(ids), nil
}

func (s *DeletionService) notify(ctx context.Context, to, template string, d mail.Data) {
	msg, err := mail.Render(template, to, d)
	if err == nil {
		err = s.d.Mail.Enqueue(ctx, msg)
	}
	if err != nil {
		s.d.Log.WarnContext(ctx, "account mail not queued", slog.String("template", template), slog.Any("error", err))
	}
}
