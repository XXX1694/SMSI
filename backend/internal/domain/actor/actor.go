// Package actor describes the authenticated principal of a request or job.
package actor

import (
	"context"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/errs"
)

// Type is the kind of principal.
type Type string

const (
	TypeUser      Type = "user"
	TypeAPIKey    Type = "api_key"
	TypeScheduler Type = "scheduler"
	TypeSystem    Type = "system"
)

// Actor is who performs an action. UserID is the tenant boundary.
type Actor struct {
	UserID    uuid.UUID
	Type      Type
	ID        string // user id, api key id, or "scheduler"
	Label     string // display name / key name (never a secret)
	Scopes    []apikey.Scope
	SessionID uuid.UUID
	APIKeyID  uuid.UUID
	RequestID string
	IP        string
}

// IsSession reports whether the actor authenticated with a browser session.
func (a Actor) IsSession() bool { return a.Type == TypeUser && a.SessionID != uuid.Nil }

// Has reports whether the actor holds a scope. Sessions hold all scopes.
func (a Actor) Has(s apikey.Scope) bool {
	if a.Type == TypeUser || a.Type == TypeScheduler || a.Type == TypeSystem {
		return true
	}
	for _, have := range a.Scopes {
		if have == s {
			return true
		}
	}
	return false
}

// Require returns INSUFFICIENT_SCOPE when the scope is missing.
func (a Actor) Require(s apikey.Scope) error {
	if a.UserID == uuid.Nil {
		return errs.New(errs.Unauthenticated, "authentication required")
	}
	if !a.Has(s) {
		return errs.Newf(errs.InsufficientScope, "missing required scope %s", s)
	}
	return nil
}

// RequireSession allows only browser sessions (e.g. API key management).
func (a Actor) RequireSession() error {
	if a.UserID == uuid.Nil {
		return errs.New(errs.Unauthenticated, "authentication required")
	}
	if !a.IsSession() {
		return errs.New(errs.Forbidden, "this action requires a browser session")
	}
	return nil
}

// EffectiveScopes returns the scopes the actor holds.
func (a Actor) EffectiveScopes() []apikey.Scope {
	if a.Type == TypeAPIKey {
		return a.Scopes
	}
	return apikey.AllScopes()
}

type ctxKey struct{}

// With stores the actor in ctx.
func With(ctx context.Context, a Actor) context.Context { return context.WithValue(ctx, ctxKey{}, a) }

// From returns the actor in ctx (zero value if absent).
func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(ctxKey{}).(Actor)
	return a, ok
}

// Scheduler returns the actor used by background jobs for a tenant.
func Scheduler(userID uuid.UUID) Actor {
	return Actor{UserID: userID, Type: TypeScheduler, ID: "scheduler", Label: "scheduler"}
}

// System returns the actor for automated actions on behalf of a tenant that no
// user or key performed (e.g. a chat platform delivering proof of ownership).
// source names the originator ("telegram").
func System(userID uuid.UUID, source string) Actor {
	return Actor{UserID: userID, Type: TypeSystem, ID: source, Label: source}
}
