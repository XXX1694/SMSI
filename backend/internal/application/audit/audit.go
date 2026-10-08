// Package audit records and lists audit log entries.
package audit

import (
	"context"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	domain "github.com/socialos/backend/internal/domain/audit"
)

// Repo persists audit entries. All reads are tenant-scoped.
type Repo interface {
	Insert(ctx context.Context, e *domain.Entry) error
	List(ctx context.Context, userID uuid.UUID, action string, page port.Page) ([]domain.Entry, error)
}

// Service implements port.AuditRecorder and the audit-log listing.
type Service struct {
	repo  Repo
	clock port.Clock
}

// NewService creates the audit service.
func NewService(repo Repo, clock port.Clock) *Service { return &Service{repo: repo, clock: clock} }

// Record writes an entry attributed to the actor.
func (s *Service) Record(ctx context.Context, a actor.Actor, action, resourceType, resourceID string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	e := &domain.Entry{
		UserID:       a.UserID,
		ActorType:    string(a.Type),
		ActorID:      a.ID,
		ActorLabel:   a.Label,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     meta,
		RequestID:    a.RequestID,
		IP:           a.IP,
		CreatedAt:    s.clock.Now(),
	}
	return s.repo.Insert(ctx, e)
}

// List returns the tenant's audit log, newest first, optionally only entries of one action. Browser sessions only.
func (s *Service) List(ctx context.Context, a actor.Actor, action string, page port.Page) (port.Result[domain.Entry], error) {
	if err := a.RequireSession(); err != nil {
		return port.Result[domain.Entry]{}, err
	}
	items, err := s.repo.List(ctx, a.UserID, action, port.Page{Limit: page.Limit + 1, Cursor: page.Cursor})
	if err != nil {
		return port.Result[domain.Entry]{}, err
	}
	return port.Paginate(items, page.Limit, func(e domain.Entry) port.Cursor {
		return port.Cursor{At: e.CreatedAt, ID: e.ID}
	}), nil
}
