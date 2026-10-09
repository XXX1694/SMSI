package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/socialos/backend/internal/domain/post"
)

// Jobs implements posts.Jobs and scheduler.Jobs.
type Jobs struct{ db *DB }

// NewJobs creates the repo.
func NewJobs(db *DB) *Jobs { return &Jobs{db: db} }

const jobCols = `j.id, j.post_target_id, j.run_at, j.asynq_task_id, j.status, j.updated_at`

func scanJob(row interface{ Scan(...any) error }) (*post.Job, error) {
	var j post.Job
	if err := row.Scan(&j.ID, &j.PostTargetID, &j.RunAt, &j.AsynqTaskID, &j.Status, &j.UpdatedAt); err != nil {
		return nil, err
	}
	return &j, nil
}

func collectJobs(rows pgx.Rows) ([]post.Job, error) {
	defer rows.Close()
	var out []post.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, mapErr(rows.Err(), "scheduled job")
}

// Create inserts a job; the partial unique index allows one active job per target.
func (r *Jobs) Create(ctx context.Context, j *post.Job) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO scheduled_jobs (id, post_target_id, run_at, status) VALUES ($1,$2,$3,$4) RETURNING updated_at`,
		j.ID, j.PostTargetID, j.RunAt, j.Status).Scan(&j.UpdatedAt), "scheduled job")
}

// Get returns a job by id (system: worker only).
func (r *Jobs) Get(ctx context.Context, id uuid.UUID) (*post.Job, error) {
	j, err := scanJob(r.db.q(ctx).QueryRow(ctx, `SELECT `+jobCols+` FROM scheduled_jobs j WHERE j.id = $1`, id))
	return j, mapErr(err, "scheduled job")
}

// CancelActiveForPost cancels active jobs of the user's post and returns them.
func (r *Jobs) CancelActiveForPost(ctx context.Context, userID, postID uuid.UUID) ([]post.Job, error) {
	rows, err := r.db.q(ctx).Query(ctx, `UPDATE scheduled_jobs j SET status = 'cancelled'
		FROM post_targets t WHERE t.id = j.post_target_id AND t.post_id = $1 AND t.user_id = $2 AND j.status IN ('pending','enqueued')
		RETURNING `+jobCols, postID, userID)
	if err != nil {
		return nil, mapErr(err, "scheduled job")
	}
	return collectJobs(rows)
}

// MarkEnqueued stores the queue task id (only while the job is still active).
func (r *Jobs) MarkEnqueued(ctx context.Context, jobID uuid.UUID, taskID string) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE scheduled_jobs SET status = 'enqueued', asynq_task_id = $2
		WHERE id = $1 AND status IN ('pending','enqueued')`, jobID, taskID)
	return mapErr(err, "scheduled job")
}

// MarkDoneForTarget finishes all active jobs of a target.
func (r *Jobs) MarkDoneForTarget(ctx context.Context, targetID uuid.UUID) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE scheduled_jobs SET status = 'done' WHERE post_target_id = $1 AND status IN ('pending','enqueued')`, targetID)
	return mapErr(err, "scheduled job")
}

// ActiveForTarget returns the active job of a target, or nil.
func (r *Jobs) ActiveForTarget(ctx context.Context, targetID uuid.UUID) (*post.Job, error) {
	j, err := scanJob(r.db.q(ctx).QueryRow(ctx, `SELECT `+jobCols+` FROM scheduled_jobs j
		WHERE j.post_target_id = $1 AND j.status IN ('pending','enqueued')`, targetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return j, err
}

// NeedingReconcile lists overdue active jobs and never-enqueued pending jobs (system).
func (r *Jobs) NeedingReconcile(ctx context.Context, before time.Time, limit int) ([]post.Job, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+jobCols+` FROM scheduled_jobs j
		WHERE (j.status IN ('pending','enqueued') AND j.run_at < $1)
		   OR (j.status = 'pending' AND j.asynq_task_id = '' AND j.updated_at < now() - interval '30 seconds')
		ORDER BY j.run_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return collectJobs(rows)
}
