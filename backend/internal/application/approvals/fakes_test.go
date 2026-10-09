package approvals

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/errs"
)

// memRepo mirrors the SQL semantics of the Postgres repo, tenant filter included.
type memRepo struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*approval.Approval
}

func newMemRepo() *memRepo { return &memRepo{rows: map[uuid.UUID]*approval.Approval{}} }

func bindingOf(a *approval.Approval) Binding {
	return Binding{ActorType: a.ActorType, ActorID: a.ActorID, Action: a.Action, ResourceType: a.ResourceType,
		ResourceID: a.ResourceID, Fingerprint: a.Fingerprint}
}

func (r *memRepo) Create(_ context.Context, a *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *a
	r.rows[a.ID] = &c
	return nil
}

func (r *memRepo) FindPending(_ context.Context, u uuid.UUID, b Binding, now time.Time) (*approval.Approval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.rows {
		if a.UserID == u && a.Status == approval.StatusPending && a.ExpiresAt.After(now) && bindingOf(a) == b {
			c := *a
			return &c, nil
		}
	}
	return nil, errs.NotFoundf("approval")
}

func (r *memRepo) CountPending(_ context.Context, u uuid.UUID, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, a := range r.rows {
		if a.UserID == u && a.Status == approval.StatusPending && a.ExpiresAt.After(now) {
			n++
		}
	}
	return n, nil
}

func (r *memRepo) Consume(_ context.Context, u, id uuid.UUID, b Binding, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	if !ok || a.UserID != u || a.Status != approval.StatusApproved || !a.ExpiresAt.After(now) || bindingOf(a) != b {
		return false, nil
	}
	a.Status, a.ConsumedAt = approval.StatusConsumed, &now
	return true, nil
}

func (r *memRepo) Get(_ context.Context, u, id uuid.UUID) (*approval.Approval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	if !ok || a.UserID != u {
		return nil, errs.NotFoundf("approval")
	}
	c := *a
	return &c, nil
}

func (r *memRepo) List(_ context.Context, u uuid.UUID, pendingOnly bool, now time.Time, page port.Page) ([]approval.Approval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []approval.Approval
	for _, a := range r.rows {
		if a.UserID == u && (!pendingOnly || (a.Status == approval.StatusPending && a.ExpiresAt.After(now))) {
			out = append(out, *a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

func (r *memRepo) Decide(_ context.Context, u, id uuid.UUID, to approval.Status, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	if !ok || a.UserID != u || a.Status != approval.StatusPending || !a.ExpiresAt.After(now) {
		return false, nil
	}
	a.Status, a.DecidedAt = to, &now
	return true, nil
}

type audited struct{ action, resourceID string }

type auditLog struct {
	mu      sync.Mutex
	entries []audited
}

func (l *auditLog) Record(_ context.Context, _ actor.Actor, action, _, resourceID string, _ map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, audited{action, resourceID})
	return nil
}

func (l *auditLog) actions() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.entries))
	for i, e := range l.entries {
		out[i] = e.action
	}
	return out
}

type passTx struct{}

func (passTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type clockAt struct{ t time.Time }

func (c *clockAt) Now() time.Time { return c.t }
