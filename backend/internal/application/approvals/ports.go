package approvals

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/approval"
)

// Binding is everything a consumed approval must match: who asked, what for, on which target, with which payload.
type Binding struct {
	ActorType    string
	ActorID      string
	Action       approval.Action
	ResourceType string
	ResourceID   string
	Fingerprint  string
}

// Repo persists approvals. Every method is tenant-scoped by userID.
type Repo interface {
	Create(ctx context.Context, a *approval.Approval) error
	// FindPending returns the unexpired pending approval with the same binding, or NOT_FOUND.
	FindPending(ctx context.Context, userID uuid.UUID, b Binding, now time.Time) (*approval.Approval, error)
	CountPending(ctx context.Context, userID uuid.UUID, now time.Time) (int, error)
	// Consume atomically flips an approved, unexpired approval with exactly this binding to consumed. It reports false
	// when no row matched (wrong tenant, id, binding, state or expired).
	Consume(ctx context.Context, userID, id uuid.UUID, b Binding, now time.Time) (bool, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*approval.Approval, error)
	List(ctx context.Context, userID uuid.UUID, pendingOnly bool, now time.Time, page port.Page) ([]approval.Approval, error)
	// Decide moves an unexpired pending approval to approved or denied; false when it was not pending.
	Decide(ctx context.Context, userID, id uuid.UUID, to approval.Status, now time.Time) (bool, error)
}
