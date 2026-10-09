// Package approvals lets the owner approve or deny dangerous actions that an
// API key attempted, and lets use cases consume such an approval once (D-013).
package approvals

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
)

// Defaults for Config.
const (
	DefaultTTL        = 10 * time.Minute
	DefaultMaxPending = 10
)

// Config tunes the service.
type Config struct {
	TTL        time.Duration
	MaxPending int
	// WebBaseURL builds the approve link given to the agent.
	WebBaseURL string
}

// Deps bundles dependencies.
type Deps struct {
	Repo  Repo
	Tx    port.TxRunner
	Audit port.AuditRecorder
	Clock port.Clock
	// Detach returns ctx without the caller's database transaction, so a new pending approval survives the rollback
	// of the 428 response. Nil means ctx is used as is (fakes).
	Detach func(context.Context) context.Context
	Config Config
}

// Service implements port.ApprovalGate and the owner-facing approval use cases.
type Service struct {
	Deps
}

var _ port.ApprovalGate = (*Service)(nil)

// NewService creates the service.
func NewService(d Deps) *Service {
	if d.Config.TTL <= 0 {
		d.Config.TTL = DefaultTTL
	}
	if d.Config.MaxPending <= 0 {
		d.Config.MaxPending = DefaultMaxPending
	}
	if d.Detach == nil {
		d.Detach = func(ctx context.Context) context.Context { return ctx }
	}
	return &Service{Deps: d}
}

func binding(a actor.Actor, req approval.Request) Binding {
	return Binding{ActorType: string(a.Type), ActorID: a.ID, Action: req.Action, ResourceType: req.ResourceType,
		ResourceID: req.ResourceID, Fingerprint: req.Fingerprint}
}

// Require implements port.ApprovalGate.
func (s *Service) Require(ctx context.Context, a actor.Actor, req approval.Request) error {
	if !a.NeedsApproval() {
		return nil
	}
	now := s.Clock.Now()
	b := binding(a, req)
	if id, ok := approval.IDFrom(ctx); ok {
		used, err := s.Repo.Consume(ctx, a.UserID, id, b, now)
		if err != nil {
			return err
		}
		if used {
			return s.Audit.Record(ctx, a, audit.ActionApprovalUsed, "approval", id.String(), auditMeta(req))
		}
		if err := s.refuseIfDenied(ctx, a, id, b); err != nil {
			return err
		}
	}
	return s.request(ctx, a, req, b, now)
}

// refuseIfDenied tells an agent that the owner said no, so it stops asking. Any other mismatch (wrong target, reuse,
// expiry, another tenant's id) gets a fresh request and no hint about why.
func (s *Service) refuseIfDenied(ctx context.Context, a actor.Actor, id uuid.UUID, b Binding) error {
	ap, err := s.Repo.Get(ctx, a.UserID, id)
	if err != nil || ap.Status != approval.StatusDenied || binding(a, approval.Request{Action: ap.Action,
		ResourceType: ap.ResourceType, ResourceID: ap.ResourceID, Fingerprint: ap.Fingerprint}) != b || ap.ActorID != b.ActorID {
		return nil
	}
	return errs.New(errs.Forbidden, "the owner denied this action")
}

func (s *Service) request(ctx context.Context, a actor.Actor, req approval.Request, b Binding, now time.Time) error {
	dctx := s.Detach(ctx)
	ap, err := s.Repo.FindPending(dctx, a.UserID, b, now)
	if errs.Is(err, errs.NotFound) {
		ap, err = s.create(dctx, a, req, now)
	}
	if err != nil {
		return err
	}
	return errs.New(errs.ApprovalRequired, "this action needs the owner's approval").
		WithField("approval_id", ap.ID.String()).
		WithField("approve_url", s.Config.WebBaseURL+"/approvals").
		WithField("expires_at", ap.ExpiresAt.UTC().Format(time.RFC3339)).
		WithField("action", string(ap.Action))
}

func (s *Service) create(ctx context.Context, a actor.Actor, req approval.Request, now time.Time) (*approval.Approval, error) {
	n, err := s.Repo.CountPending(ctx, a.UserID, now)
	if err != nil {
		return nil, err
	}
	if n >= s.Config.MaxPending {
		return nil, errs.New(errs.RateLimited, "too many pending approvals; ask the owner to decide on them first")
	}
	ap := &approval.Approval{ID: uuid.New(), UserID: a.UserID, ActorType: string(a.Type), ActorID: a.ID, ActorLabel: a.Label,
		Action: req.Action, ResourceType: req.ResourceType, ResourceID: req.ResourceID, Fingerprint: req.Fingerprint,
		Summary: req.Summary, Status: approval.StatusPending, ExpiresAt: now.Add(s.Config.TTL), CreatedAt: now}
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.Create(ctx, ap); err != nil {
			return err
		}
		return s.Audit.Record(ctx, a, audit.ActionApprovalRequested, "approval", ap.ID.String(), auditMeta(req))
	})
	return ap, err
}

func auditMeta(req approval.Request) map[string]any {
	return map[string]any{"action": string(req.Action), "resource_type": req.ResourceType, "resource_id": req.ResourceID}
}
