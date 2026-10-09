package auth

import (
	"context"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

func unauthenticated() error { return errs.New(errs.Unauthenticated, "invalid or expired credentials") }

// AuthenticateSession resolves a session cookie. It returns the actor and the
// session's CSRF token (for double-submit verification by the transport).
func (s *Service) AuthenticateSession(ctx context.Context, rawToken string, ci ClientInfo) (actor.Actor, string, error) {
	if rawToken == "" || len(rawToken) > 128 {
		return actor.Actor{}, "", unauthenticated()
	}
	sess, err := s.sessions.GetByTokenHash(ctx, crypto.SHA256Hex(rawToken))
	if errs.Is(err, errs.NotFound) {
		return actor.Actor{}, "", unauthenticated()
	}
	if err != nil {
		return actor.Actor{}, "", err
	}
	if !s.clock.Now().Before(sess.ExpiresAt) {
		return actor.Actor{}, "", unauthenticated()
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return actor.Actor{}, "", unauthenticated()
	}
	if u.Status != user.StatusActive {
		return actor.Actor{}, "", unauthenticated()
	}
	a := actor.Actor{
		UserID: u.ID, Type: actor.TypeUser, ID: u.ID.String(), Label: u.Email,
		SessionID: sess.ID, RequestID: ci.RequestID, IP: ci.IP, EmailVerified: s.verified(u),
	}
	return a, sess.CSRFToken, nil
}

// AuthenticateAPIKey resolves a bearer API key (SHA-256 lookup, constant format check).
func (s *Service) AuthenticateAPIKey(ctx context.Context, raw string, ci ClientInfo) (actor.Actor, error) {
	if !crypto.LooksLikeAPIKey(raw) {
		return actor.Actor{}, unauthenticated()
	}
	k, err := s.keys.GetByHash(ctx, crypto.SHA256Hex(raw))
	if errs.Is(err, errs.NotFound) {
		return actor.Actor{}, unauthenticated()
	}
	if err != nil {
		return actor.Actor{}, err
	}
	now := s.clock.Now()
	if !k.Usable(now) {
		return actor.Actor{}, unauthenticated()
	}
	u, err := s.users.GetByID(ctx, k.UserID)
	if err != nil || u.Status != user.StatusActive {
		return actor.Actor{}, unauthenticated()
	}
	if err := s.keys.TouchLastUsed(ctx, k.ID, now); err != nil {
		return actor.Actor{}, err
	}
	return actor.Actor{
		UserID: k.UserID, Type: actor.TypeAPIKey, ID: k.ID.String(), Label: k.Name,
		Scopes: k.Scopes, APIKeyID: k.ID, RequestID: ci.RequestID, IP: ci.IP, EmailVerified: s.verified(u),
		DangerousPolicy: k.DangerousPolicy,
	}, nil
}

// verified is the actor's EmailVerified: true when the owner verified the
// address or when this server does not enforce verification.
func (s *Service) verified(u *user.User) bool { return !s.requireVerified || u.EmailVerified() }

// VerificationEnforced reports whether unverified owners are restricted.
func (s *Service) VerificationEnforced() bool { return s.requireVerified }
