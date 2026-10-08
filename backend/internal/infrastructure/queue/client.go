// Package queue is the Asynq transport for publish jobs. The DB table
// scheduled_jobs is the source of truth; Redis only carries tasks.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hibiken/asynq"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/domain/post"
)

// TypePublishTarget is the task type name.
const TypePublishTarget = "publish:target"

// Task options.
const (
	taskTimeout   = 5 * time.Minute
	taskRetention = 24 * time.Hour
)

// Client enqueues, inspects and deletes publish tasks.
type Client struct {
	client    *asynq.Client
	inspector *asynq.Inspector
	queue     string
}

// NewClient creates a queue client for the given Redis and queue name.
func NewClient(redis asynq.RedisConnOpt, queue string) *Client {
	return &Client{client: asynq.NewClient(redis), inspector: asynq.NewInspector(redis), queue: queue}
}

// Close releases Redis connections.
func (c *Client) Close() error {
	err := c.client.Close()
	if ierr := c.inspector.Close(); err == nil {
		err = ierr
	}
	return err
}

// TaskID is deterministic per (target, run_at, job) so duplicate enqueues of
// the same job collapse while a later job for the same target never collides
// with a retained completed task.
func TaskID(j post.Job) string {
	return j.PostTargetID.String() + ":" + strconv.FormatInt(j.RunAt.Unix(), 10) + ":" + j.ID.String()[:8]
}

// Enqueue schedules the job at RunAt. An existing task with the same id is success.
func (c *Client) Enqueue(ctx context.Context, j post.Job) (string, error) {
	return c.enqueue(ctx, j, TaskID(j))
}

// Reenqueue enqueues with a fresh task id (used when the old task is finished/lost).
func (c *Client) Reenqueue(ctx context.Context, j post.Job) (string, error) {
	return c.enqueue(ctx, j, TaskID(j)+":r"+strconv.FormatInt(time.Now().UnixNano(), 36))
}

// ProcessAtFor rounds t up to the next whole second. Asynq scores delayed tasks
// by Unix seconds (floor), so a sub-second run_at would otherwise fire early.
func ProcessAtFor(t time.Time) time.Time {
	if t.Nanosecond() == 0 {
		return t
	}
	return time.Unix(t.Unix()+1, 0)
}

func (c *Client) enqueue(ctx context.Context, j post.Job, id string) (string, error) {
	payload, err := json.Marshal(scheduler.Payload{TargetID: j.PostTargetID, JobID: j.ID})
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(TypePublishTarget, payload)
	_, err = c.client.EnqueueContext(ctx, task,
		asynq.TaskID(id), asynq.Queue(c.queue), asynq.ProcessAt(ProcessAtFor(j.RunAt)), asynq.MaxRetry(scheduler.MaxRetry),
		asynq.Timeout(taskTimeout), asynq.Retention(taskRetention))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("queue: enqueue: %w", err)
	}
	return id, nil
}

// Delete removes a not-yet-running task (missing tasks are fine).
func (c *Client) Delete(_ context.Context, taskID string) error {
	err := c.inspector.DeleteTask(c.queue, taskID)
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return nil
	}
	return err
}

// TaskState maps Asynq task state to the scheduler's view.
func (c *Client) TaskState(_ context.Context, taskID string) (scheduler.TaskState, error) {
	info, err := c.inspector.GetTaskInfo(c.queue, taskID)
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return scheduler.TaskMissing, nil
	}
	if err != nil {
		return scheduler.TaskMissing, err
	}
	switch info.State {
	case asynq.TaskStateCompleted:
		return scheduler.TaskCompleted, nil
	case asynq.TaskStateArchived:
		return scheduler.TaskArchived, nil
	default:
		return scheduler.TaskLive, nil
	}
}
