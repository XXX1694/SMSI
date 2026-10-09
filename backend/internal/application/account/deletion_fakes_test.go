package account

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/user"
)

// world is a tiny in-memory stand-in for everything a purge touches, with the same ordering rules as the schema.
type world struct {
	mu        sync.Mutex
	clock     *clockAt
	users     map[uuid.UUID]*user.User
	scheduled map[uuid.UUID]time.Time
	records   map[uuid.UUID]*record
	posts     map[uuid.UUID][]uuid.UUID // user -> post ids
	audits    map[uuid.UUID]int
	media     map[uuid.UUID][]MediaObject
	exports   map[uuid.UUID][]string
	publish   map[uuid.UUID]bool // user has a post being published
	sessions  map[uuid.UUID]int
	keysLeft  map[uuid.UUID]int
	order     []string // calls that matter for ordering
}

type record struct {
	requested time.Time
	purged    *time.Time
	counts    Counts
	saved     bool
}

func newWorld(c *clockAt) *world {
	return &world{clock: c, users: map[uuid.UUID]*user.User{}, scheduled: map[uuid.UUID]time.Time{}, records: map[uuid.UUID]*record{},
		posts: map[uuid.UUID][]uuid.UUID{}, audits: map[uuid.UUID]int{}, media: map[uuid.UUID][]MediaObject{},
		exports: map[uuid.UUID][]string{}, publish: map[uuid.UUID]bool{}, sessions: map[uuid.UUID]int{}, keysLeft: map[uuid.UUID]int{}}
}

func (w *world) addUser(email string) *user.User {
	u := &user.User{ID: uuid.New(), Email: email, PasswordHash: "hash:secret", Status: user.StatusActive}
	w.users[u.ID] = u
	w.sessions[u.ID], w.keysLeft[u.ID] = 2, 3
	return u
}

func (w *world) GetByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	u, ok := w.users[id]
	if !ok {
		return nil, errs.NotFoundf("user")
	}
	c := *u
	if t, ok := w.scheduled[id]; ok {
		c.DeletionScheduledAt = &t
	}
	return &c, nil
}

func (w *world) Verify(_ context.Context, password, encoded string) (bool, error) {
	return "hash:"+password == encoded, nil
}

func (w *world) DeleteAllForUser(_ context.Context, id, _ uuid.UUID) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.sessions[id]
	w.sessions[id] = 0
	return int64(n), nil
}

func (w *world) RevokeAllForUser(_ context.Context, id uuid.UUID, _ time.Time) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.keysLeft[id]
	w.keysLeft[id] = 0
	return int64(n), nil
}

func (w *world) UnscheduleAll(_ context.Context, id uuid.UUID) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.order = append(w.order, "unschedule")
	return 1, nil
}

func (w *world) Schedule(_ context.Context, id uuid.UUID, requestedAt, purgeAt time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.scheduled[id]; ok {
		return errs.New(errs.Conflict, "already scheduled")
	}
	w.scheduled[id] = purgeAt
	w.records[id] = &record{requested: requestedAt}
	w.order = append(w.order, "schedule")
	return nil
}

func (w *world) Cancel(_ context.Context, id uuid.UUID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.scheduled[id]; !ok {
		return errs.New(errs.Conflict, "no deletion is scheduled for this account")
	}
	delete(w.scheduled, id)
	delete(w.records, id)
	return nil
}

func (w *world) Due(_ context.Context, now time.Time, _ int) ([]uuid.UUID, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []uuid.UUID
	for id, u := range w.users {
		if at, ok := w.scheduled[id]; (u.Status == user.StatusActive && ok && !at.After(now)) || u.Status == user.StatusDeleted {
			out = append(out, id)
		}
	}
	return out, nil
}

func (w *world) Claim(_ context.Context, id uuid.UUID, now time.Time) (string, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	u, ok := w.users[id]
	if !ok {
		return "", false, nil
	}
	at, sched := w.scheduled[id]
	if u.Status == user.StatusDeleted || (u.Status == user.StatusActive && sched && !at.After(now)) {
		u.Status = user.StatusDeleted
		return u.Email, true, nil
	}
	return "", false, nil
}

func (w *world) Busy(_ context.Context, id uuid.UUID) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.publish[id] {
		return "a post is being published", nil
	}
	return "", nil
}

func (w *world) Counts(_ context.Context, id uuid.UUID) (Counts, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Counts{Posts: int64(len(w.posts[id])), Media: int64(len(w.media[id]))}, nil
}

func (w *world) SaveCounts(_ context.Context, id uuid.UUID, c Counts) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r := w.records[id]; r != nil && !r.saved {
		r.counts, r.saved = c, true
	}
	return nil
}

func (w *world) DeleteBatch(_ context.Context, id uuid.UUID, t Table, limit int) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.order = append(w.order, "batch:"+string(t))
	switch t {
	case TablePosts:
		n := min(limit, len(w.posts[id]))
		w.posts[id] = w.posts[id][n:]
		return int64(n), nil
	case TableAuditLogs:
		n := min(limit, w.audits[id])
		w.audits[id] -= n
		return int64(n), nil
	}
	return 0, nil
}

func (w *world) MediaBatch(_ context.Context, id uuid.UUID, limit int) ([]MediaObject, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.media[id][:min(limit, len(w.media[id]))], nil
}

func (w *world) DeleteMedia(_ context.Context, id uuid.UUID, ids []uuid.UUID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.posts[id]) > 0 {
		return errs.New(errs.Conflict, "post_media still references media (RESTRICT)")
	}
	w.order = append(w.order, "media")
	keep := w.media[id][:0:0]
	for _, m := range w.media[id] {
		gone := false
		for _, x := range ids {
			gone = gone || x == m.ID
		}
		if !gone {
			keep = append(keep, m)
		}
	}
	w.media[id] = keep
	return nil
}

func (w *world) ExportKeys(_ context.Context, id uuid.UUID) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.exports[id], nil
}

func (w *world) DeleteUser(_ context.Context, id uuid.UUID, at time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.order = append(w.order, "user")
	delete(w.users, id)
	r := w.records[id]
	if r == nil {
		r = &record{requested: at}
		w.records[id] = r
	}
	r.purged = &at
	return nil
}

// mailSink records queued mail.
type mailSink struct {
	mu   sync.Mutex
	sent []port.Message
}

func (m *mailSink) Enqueue(_ context.Context, msg port.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mailSink) templates() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var t []string
	for _, s := range m.sent {
		t = append(t, s.Template)
	}
	return strings.Join(t, ",")
}

type purgeQueued struct {
	mu  sync.Mutex
	ids []uuid.UUID
}

func (q *purgeQueued) EnqueuePurge(_ context.Context, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, id)
	return nil
}
