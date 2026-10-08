package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// Service is the auth use-case facade.
type Service struct {
	users      Users
	sessions   Sessions
	keys       APIKeys
	hasher     PasswordHasher
	tx         port.TxRunner
	audit      port.AuditRecorder
	clock      port.Clock
	sessionTTL time.Duration
	dummyHash  string

	tokens EmailTokens
	mail   port.MailQueue
	forgot ForgotQueue
	log    *slog.Logger
	webURL string
	// requireVerified makes unverified owners fail RequireVerified guards.
	requireVerified bool
}

// Deps bundles Service dependencies.
type Deps struct {
	Users      Users
	Sessions   Sessions
	APIKeys    APIKeys
	Hasher     PasswordHasher
	Tx         port.TxRunner
	Audit      port.AuditRecorder
	Clock      port.Clock
	SessionTTL time.Duration

	Tokens EmailTokens
	Mail   port.MailQueue
	Forgot ForgotQueue
	Log    *slog.Logger
	// WebBaseURL is the frontend origin that mailed links point to.
	WebBaseURL string
	// RequireVerification enforces email verification (set when mail can really be delivered).
	RequireVerification bool
}

// NewService creates the auth service.
func NewService(d Deps) (*Service, error) {
	if d.SessionTTL == 0 {
		d.SessionTTL = 7 * 24 * time.Hour
	}
	if d.Tokens == nil || d.Mail == nil || d.Forgot == nil {
		return nil, errors.New("auth: Tokens, Mail and Forgot are required")
	}
	dummy, err := d.Hasher.Hash("timing-equalizer-password")
	if err != nil {
		return nil, err
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{users: d.Users, sessions: d.Sessions, keys: d.APIKeys, hasher: d.Hasher, tx: d.Tx,
		audit: d.Audit, clock: d.Clock, sessionTTL: d.SessionTTL, dummyHash: dummy,
		tokens: d.Tokens, mail: d.Mail, forgot: d.Forgot, log: d.Log, webURL: strings.TrimRight(d.WebBaseURL, "/"),
		requireVerified: d.RequireVerification}, nil
}

// RegisterInput is the registration payload.
type RegisterInput struct {
	Email, Password, DisplayName string
}

// Register creates a user and logs them in.
func (s *Service) Register(ctx context.Context, in RegisterInput, ci ClientInfo) (*user.User, IssuedSession, error) {
	email, err := user.NormalizeEmail(in.Email)
	if err != nil {
		return nil, IssuedSession{}, err
	}
	if err := user.ValidatePassword(in.Password); err != nil {
		return nil, IssuedSession{}, err
	}
	name, err := user.ValidateDisplayName(in.DisplayName)
	if err != nil {
		return nil, IssuedSession{}, err
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, IssuedSession{}, err
	}
	u := &user.User{Email: email, PasswordHash: hash, DisplayName: name, Status: user.StatusActive, Plan: user.DefaultPlan}
	var issued IssuedSession
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.users.Create(ctx, u); err != nil {
			return err
		}
		var err error
		if issued, err = s.newSession(ctx, u.ID, ci); err != nil {
			return err
		}
		return s.audit.Record(ctx, userActor(u, ci), audit.ActionUserRegistered, "user", u.ID.String(), nil)
	})
	if err != nil {
		return nil, IssuedSession{}, err
	}
	s.IssueVerification(ctx, u)
	return u, issued, nil
}

// Login verifies credentials and issues a session. Errors never reveal whether the email exists.
func (s *Service) Login(ctx context.Context, email, password string, ci ClientInfo) (*user.User, IssuedSession, error) {
	invalid := errs.New(errs.Unauthenticated, "invalid email or password")
	norm, err := user.NormalizeEmail(email)
	if err != nil {
		_, _ = s.hasher.Verify(password, s.dummyHash)
		return nil, IssuedSession{}, invalid
	}
	u, err := s.users.GetByEmail(ctx, norm)
	if errs.Is(err, errs.NotFound) {
		_, _ = s.hasher.Verify(password, s.dummyHash)
		return nil, IssuedSession{}, invalid
	}
	if err != nil {
		return nil, IssuedSession{}, err
	}
	ok, err := s.hasher.Verify(password, u.PasswordHash)
	if err != nil || !ok || u.Status != user.StatusActive {
		return nil, IssuedSession{}, invalid
	}
	var issued IssuedSession
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if issued, err = s.newSession(ctx, u.ID, ci); err != nil {
			return err
		}
		return s.audit.Record(ctx, userActor(u, ci), audit.ActionUserLogin, "user", u.ID.String(), nil)
	})
	return u, issued, err
}

func (s *Service) newSession(ctx context.Context, userID uuid.UUID, ci ClientInfo) (IssuedSession, error) {
	token, err := crypto.RandomToken(32)
	if err != nil {
		return IssuedSession{}, err
	}
	csrf, err := crypto.RandomToken(32)
	if err != nil {
		return IssuedSession{}, err
	}
	sess := &Session{
		UserID: userID, TokenHash: crypto.SHA256Hex(token), CSRFToken: csrf,
		ExpiresAt: s.clock.Now().Add(s.sessionTTL), UserAgent: truncate(ci.UserAgent, 256), IP: ci.IP,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return IssuedSession{}, err
	}
	return IssuedSession{Token: token, CSRFToken: csrf, ExpiresAt: sess.ExpiresAt}, nil
}

// Logout deletes the actor's session.
func (s *Service) Logout(ctx context.Context, a actor.Actor) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.sessions.Delete(ctx, a.UserID, a.SessionID); err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionUserLogout, "user", a.UserID.String(), nil)
	})
}

// Me returns the current user.
func (s *Service) Me(ctx context.Context, a actor.Actor) (*user.User, error) {
	if a.UserID == uuid.Nil {
		return nil, errs.New(errs.Unauthenticated, "authentication required")
	}
	return s.users.GetByID(ctx, a.UserID)
}

// PurgeExpiredSessions removes expired sessions (called periodically).
func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessions.DeleteExpired(ctx, s.clock.Now())
}

func userActor(u *user.User, ci ClientInfo) actor.Actor {
	return actor.Actor{UserID: u.ID, Type: actor.TypeUser, ID: u.ID.String(), Label: u.Email, RequestID: ci.RequestID, IP: ci.IP}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
