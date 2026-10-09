package schedulertest

import (
	"context"
	"fmt"
	"time"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/post"
)

// Clock is a manual clock (port.Clock). Sleep advances it instead of waiting.
type Clock struct {
	T      time.Time
	Sleeps []time.Duration
}

// Now returns the current fake time.
func (c *Clock) Now() time.Time { return c.T }

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) { c.T = c.T.Add(d) }

// Sleep is a scheduler.Deps.Sleep: it records d, advances the clock and
// returns ctx.Err() when the context is already done.
func (c *Clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.Sleeps = append(c.Sleeps, d)
	c.Advance(d)
	return nil
}

// Tx is a port.TxRunner over the Store: nested calls join the outer
// transaction, and a failing outermost fn rolls the whole Store back.
type Tx struct {
	S     *Store
	Calls int // outermost transactions started
	depth int
}

// InTx runs fn, restoring the Store when the outermost fn fails.
func (t *Tx) InTx(ctx context.Context, fn func(context.Context) error) error {
	if t.depth > 0 {
		return fn(ctx)
	}
	if err := t.S.fail("Tx.InTx"); err != nil {
		return err
	}
	t.Calls++
	sn := t.S.snapshot()
	t.depth++
	defer func() { t.depth-- }()
	if err := fn(ctx); err != nil {
		t.S.restore(sn)
		return err
	}
	return nil
}

// Audit is a port.AuditRecorder that appends to Store.Audit (rolled back with the tx).
type Audit struct{ S *Store }

// Record stores the event.
func (a Audit) Record(_ context.Context, _ actor.Actor, action, resourceType, resourceID string, meta map[string]any) error {
	if err := a.S.fail("Audit.Record"); err != nil {
		return err
	}
	a.S.Audit = append(a.S.Audit, AuditEntry{action, resourceType, resourceID, meta})
	return nil
}

// Queue fakes scheduler.Queue and records what was (re-)enqueued.
type Queue struct {
	S          *Store
	Enqueued   []post.Job
	Reenqueued []post.Job
	// States maps a task id to its queue state; unknown ids are TaskMissing.
	States map[string]scheduler.TaskState
	n      int
}

// NewQueue creates a queue fake.
func NewQueue(s *Store) *Queue { return &Queue{S: s, States: map[string]scheduler.TaskState{}} }

func (q *Queue) next(prefix string) string { q.n++; return fmt.Sprintf("%s-%d", prefix, q.n) }

// Enqueue records the job and returns a new task id (state: live).
func (q *Queue) Enqueue(_ context.Context, j post.Job) (string, error) {
	if err := q.S.fail("Queue.Enqueue"); err != nil {
		return "", err
	}
	id := q.next("task")
	q.Enqueued = append(q.Enqueued, j)
	q.States[id] = scheduler.TaskLive
	return id, nil
}

// Reenqueue records the job and returns a fresh task id (state: live).
func (q *Queue) Reenqueue(_ context.Context, j post.Job) (string, error) {
	if err := q.S.fail("Queue.Reenqueue"); err != nil {
		return "", err
	}
	id := q.next("retask")
	q.Reenqueued = append(q.Reenqueued, j)
	q.States[id] = scheduler.TaskLive
	return id, nil
}

// TaskState returns the configured state (TaskMissing by default).
func (q *Queue) TaskState(_ context.Context, taskID string) (scheduler.TaskState, error) {
	if err := q.S.fail("Queue.TaskState"); err != nil {
		return scheduler.TaskMissing, err
	}
	return q.States[taskID], nil
}
