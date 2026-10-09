package approvals

import (
	"context"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
)

// List returns the owner's approvals, newest first: only the open ones, or all when includeDecided. Sessions only.
func (s *Service) List(ctx context.Context, a actor.Actor, includeDecided bool, page port.Page) (port.Result[approval.Approval], error) {
	if err := a.RequireSession(); err != nil {
		return port.Result[approval.Approval]{}, err
	}
	items, err := s.repo.List(ctx, a.UserID, !includeDecided, s.clock.Now(), port.Page{Limit: page.Limit + 1, Cursor: page.Cursor})
	if err != nil {
		return port.Result[approval.Approval]{}, err
	}
	now := s.clock.Now()
	for i := range items {
		items[i].Status = items[i].EffectiveStatus(now)
	}
	return port.Paginate(items, page.Limit, func(x approval.Approval) port.Cursor {
		return port.Cursor{At: x.CreatedAt, ID: x.ID}
	}), nil
}

// Get returns one approval of the owner. Sessions only.
func (s *Service) Get(ctx context.Context, a actor.Actor, id uuid.UUID) (*approval.Approval, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	return s.view(ctx, a, id)
}

// view reads one approval of the owner with its status as of now: a pending or approved one past its deadline is
// reported as expired, whatever the row says.
func (s *Service) view(ctx context.Context, a actor.Actor, id uuid.UUID) (*approval.Approval, error) {
	ap, err := s.repo.Get(ctx, a.UserID, id)
	if err != nil {
		return nil, err
	}
	ap.Status = ap.EffectiveStatus(s.clock.Now())
	return ap, nil
}

// Approve lets the agent repeat the call once. Sessions only: a key can never approve itself.
func (s *Service) Approve(ctx context.Context, a actor.Actor, id uuid.UUID) (*approval.Approval, error) {
	return s.decide(ctx, a, id, approval.StatusApproved, audit.ActionApprovalApproved)
}

// Deny refuses the action; the agent is told not to ask again. Sessions only.
func (s *Service) Deny(ctx context.Context, a actor.Actor, id uuid.UUID) (*approval.Approval, error) {
	return s.decide(ctx, a, id, approval.StatusDenied, audit.ActionApprovalDenied)
}

func (s *Service) decide(ctx context.Context, a actor.Actor, id uuid.UUID, to approval.Status, action string) (*approval.Approval, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		ok, err := s.repo.Decide(ctx, a.UserID, id, to, s.clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			if _, err := s.repo.Get(ctx, a.UserID, id); err != nil {
				return err
			}
			return errs.New(errs.Conflict, "this approval is no longer pending")
		}
		return s.audit.Record(ctx, a, action, "approval", id.String(), nil)
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, a, id)
}
