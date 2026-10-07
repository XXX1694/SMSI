// Package posts implements post authoring and lifecycle use cases
// (draft → schedule → publish/cancel/retry) on top of the domain state machine.
package posts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// ListFilter narrows GET /posts.
type ListFilter struct {
	Status post.Status
	From   *time.Time
	To     *time.Time
}

// Repo persists posts and targets. Every method is tenant-scoped by userID.
type Repo interface {
	Create(ctx context.Context, p *post.Post) error
	Get(ctx context.Context, userID, id uuid.UUID) (*post.Post, error)
	GetForUpdate(ctx context.Context, userID, id uuid.UUID) (*post.Post, error)
	List(ctx context.Context, userID uuid.UUID, f ListFilter, page port.Page) ([]post.Post, error)
	Update(ctx context.Context, p *post.Post) error
	ReplaceTargets(ctx context.Context, p *post.Post) error
	ReplaceMedia(ctx context.Context, userID, postID uuid.UUID, mediaIDs []uuid.UUID) error
	UpdateTarget(ctx context.Context, t *post.Target) error
	SoftDelete(ctx context.Context, userID, id uuid.UUID, at time.Time) error
	Attempts(ctx context.Context, userID, postID uuid.UUID) ([]post.Attempt, error)
}

// Jobs persists scheduled_jobs (the source of truth for scheduling).
type Jobs interface {
	Create(ctx context.Context, j *post.Job) error
	CancelActiveForPost(ctx context.Context, userID, postID uuid.UUID) ([]post.Job, error)
	MarkEnqueued(ctx context.Context, jobID uuid.UUID, taskID string) error
}

// Enqueuer is the queue transport (Asynq).
type Enqueuer interface {
	Enqueue(ctx context.Context, j post.Job) (taskID string, err error)
	Delete(ctx context.Context, taskID string) error
}

// Accounts is the tenant-scoped account reader.
type Accounts interface {
	Get(ctx context.Context, userID, id uuid.UUID) (*socialaccount.Account, error)
}

// Media is the tenant-scoped media reader.
type Media interface {
	GetMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]media.Media, error)
}
