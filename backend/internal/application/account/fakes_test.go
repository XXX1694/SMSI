package account

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/dataexport"
	"github.com/socialos/backend/internal/domain/errs"
)

type clockAt struct{ t time.Time }

func (c *clockAt) Now() time.Time { return c.t }

type passTx struct{}

func (passTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type auditEntry struct {
	Action string
	Actor  actor.Actor
	Meta   map[string]any
}

type auditLog struct {
	mu      sync.Mutex
	entries []auditEntry
}

func (l *auditLog) Record(_ context.Context, a actor.Actor, action, _, _ string, meta map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, auditEntry{action, a, meta})
	return nil
}

func (l *auditLog) actions() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []string{}
	for _, e := range l.entries {
		out = append(out, e.Action)
	}
	return out
}

// memExports is an in-memory ExportRepo with the same guarantees as the SQL one (one active export per user).
type memExports struct {
	mu    sync.Mutex
	clock *clockAt
	rows  map[uuid.UUID]*dataexport.Export
}

func newMemExports(c *clockAt) *memExports {
	return &memExports{clock: c, rows: map[uuid.UUID]*dataexport.Export{}}
}

func (m *memExports) Create(_ context.Context, userID uuid.UUID) (*dataexport.Export, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.rows {
		if e.UserID == userID && e.Active() {
			return nil, errs.New(errs.Conflict, "export already exists")
		}
	}
	e := &dataexport.Export{ID: uuid.New(), UserID: userID, Status: dataexport.StatusPending, CreatedAt: m.clock.t, UpdatedAt: m.clock.t}
	m.rows[e.ID] = e
	c := *e
	return &c, nil
}

func (m *memExports) Get(_ context.Context, userID, id uuid.UUID) (*dataexport.Export, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.rows[id]
	if !ok || e.UserID != userID {
		return nil, errs.NotFoundf("export")
	}
	c := *e
	return &c, nil
}

func (m *memExports) List(_ context.Context, userID uuid.UUID, limit int) ([]dataexport.Export, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []dataexport.Export{}
	for _, e := range m.rows {
		if e.UserID == userID {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memExports) ExpireReady(_ context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.rows {
		if e.UserID == userID && e.Status == dataexport.StatusReady {
			e.Status = dataexport.StatusExpired
		}
	}
	return nil
}

func (m *memExports) Claim(_ context.Context, id uuid.UUID) (*dataexport.Export, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.rows[id]
	if !ok || e.Status != dataexport.StatusPending {
		return nil, errs.NotFoundf("export")
	}
	e.Status, e.UpdatedAt = dataexport.StatusRunning, m.clock.t
	c := *e
	return &c, nil
}

func (m *memExports) MarkReady(_ context.Context, id uuid.UUID, key string, size int64, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.rows[id]
	e.Status, e.StorageKey, e.SizeBytes, e.ExpiresAt = dataexport.StatusReady, key, size, &expiresAt
	return nil
}

func (m *memExports) MarkFailed(_ context.Context, id uuid.UUID, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.rows[id]; e.Active() {
		e.Status, e.ErrorCode = dataexport.StatusFailed, code
	}
	return nil
}

func (m *memExports) Sweepable(_ context.Context, now, pendingBefore, runningBefore time.Time, limit int) ([]dataexport.Export, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []dataexport.Export{}
	for _, e := range m.rows {
		due := (e.Status == dataexport.StatusReady && !e.ExpiresAt.After(now)) ||
			(e.Status == dataexport.StatusExpired && e.StorageKey != "") ||
			(e.Status == dataexport.StatusPending && e.UpdatedAt.Before(pendingBefore)) ||
			(e.Status == dataexport.StatusRunning && e.UpdatedAt.Before(runningBefore))
		if due && len(out) < limit {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (m *memExports) ClearObject(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.rows[id]
	e.StorageKey, e.Status = "", dataexport.StatusExpired
	return nil
}

type queued struct {
	mu  sync.Mutex
	ids []uuid.UUID
	err error
}

func (q *queued) EnqueueExport(_ context.Context, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.ids = append(q.ids, id)
	return nil
}

// memData serves fixed rows per dataset, paged by id like the SQL repo.
type memData struct {
	rows  map[Dataset][]Row
	media []MediaFile
	// asked records the user of every read, so a test can prove no other tenant was queried.
	asked []uuid.UUID
	// onRows runs at the start of every Rows call (to cancel the context mid-build).
	onRows func()
}

func (d *memData) Rows(_ context.Context, ds Dataset, userID, after uuid.UUID, limit int) ([]Row, error) {
	d.asked = append(d.asked, userID)
	if d.onRows != nil {
		d.onRows()
	}
	out := []Row{}
	for _, r := range d.rows[ds] {
		if string(r.ID[:]) > string(after[:]) && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d *memData) MediaFiles(_ context.Context, _, after uuid.UUID, limit int) ([]MediaFile, error) {
	out := []MediaFile{}
	for _, m := range d.media {
		if string(m.ID[:]) > string(after[:]) && len(out) < limit {
			out = append(out, m)
		}
	}
	return out, nil
}

func row(id uuid.UUID, v map[string]any) Row {
	b, _ := json.Marshal(v)
	return Row{ID: id, JSON: b}
}

var errBoom = errors.New("boom")
