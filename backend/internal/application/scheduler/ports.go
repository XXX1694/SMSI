// Package scheduler executes publish jobs (worker side) with exactly-once
// intent: a post_target is published at most once (ARCHITECTURE.md §6), and
// reconciles the DB job table (source of truth) with the queue.
package scheduler

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Payload is the queue task body of publish:target.
type Payload struct {
	TargetID uuid.UUID `json:"target_id"`
	JobID    uuid.UUID `json:"job_id"`
}

// RetryInfo is the queue's retry bookkeeping for the current execution.
type RetryInfo struct {
	Retried  int // number of previous retries
	MaxRetry int
}

// Exhausted reports whether this is the last allowed execution.
func (r RetryInfo) Exhausted() bool { return r.Retried >= r.MaxRetry }

// Targets is the worker's view of post_targets and attempts.
type Targets interface {
	// LockTarget locks a target row with FOR UPDATE SKIP LOCKED.
	// It returns (nil, false, nil) when another transaction holds the lock.
	LockTarget(ctx context.Context, id uuid.UUID) (*post.Target, bool, error)
	// LockTargetWait locks a target row of the given owner, waiting for other holders (finalize path).
	LockTargetWait(ctx context.Context, userID, id uuid.UUID) error
	UpdateTarget(ctx context.Context, t *post.Target) error
	ListTargets(ctx context.Context, userID, postID uuid.UUID) ([]post.Target, error)
	LatestAttempt(ctx context.Context, targetID uuid.UUID) (*post.Attempt, error)
	InsertAttempt(ctx context.Context, a *post.Attempt) error
	UpdateAttempt(ctx context.Context, a *post.Attempt) error
	StuckPublishing(ctx context.Context, before time.Time, limit int) ([]post.Target, error)
}

// Posts is the subset of the post repository the worker needs.
type Posts interface {
	GetForUpdate(ctx context.Context, userID, id uuid.UUID) (*post.Post, error)
	Update(ctx context.Context, p *post.Post) error
}

// Jobs is the worker's view of scheduled_jobs.
type Jobs interface {
	Get(ctx context.Context, id uuid.UUID) (*post.Job, error)
	Create(ctx context.Context, j *post.Job) error
	MarkDoneForTarget(ctx context.Context, targetID uuid.UUID) error
	MarkEnqueued(ctx context.Context, jobID uuid.UUID, taskID string) error
	ActiveForTarget(ctx context.Context, targetID uuid.UUID) (*post.Job, error)
	// NeedingReconcile returns active jobs overdue before `before`, plus pending jobs never enqueued.
	NeedingReconcile(ctx context.Context, before time.Time, limit int) ([]post.Job, error)
}

// TaskState is the queue-side state of a task.
type TaskState int

const (
	TaskMissing TaskState = iota
	TaskLive              // pending, scheduled, active or retrying
	TaskCompleted
	TaskArchived // retries exhausted
)

// Queue is the transport port used by the worker side.
type Queue interface {
	Enqueue(ctx context.Context, j post.Job) (string, error)
	Reenqueue(ctx context.Context, j post.Job) (string, error)
	TaskState(ctx context.Context, taskID string) (TaskState, error)
}

// Accounts loads accounts and marks them expired.
type Accounts interface {
	Get(ctx context.Context, userID, id uuid.UUID) (*socialaccount.Account, error)
	MarkExpired(ctx context.Context, a actor.Actor, acc *socialaccount.Account, reason string) error
}

// Vault loads and saves decrypted credentials.
type Vault interface {
	Load(ctx context.Context, accountID uuid.UUID) (socialaccount.Credentials, error)
	Save(ctx context.Context, accountID uuid.UUID, tok provider.Token) error
}

// MediaStore resolves post media and opens their bytes.
type MediaStore interface {
	GetMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]media.Media, error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, error)
}

// Metrics records internal analytics counters.
type Metrics interface {
	Increment(ctx context.Context, userID, accountID, targetID uuid.UUID, metric string, value int64) error
}
