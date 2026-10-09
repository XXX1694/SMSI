package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/dataexport"
)

// Task types.
const (
	TypeMailSend      = "mail:send"
	TypeAuthForgot    = "auth:forgot"
	TypeAccountPurge  = "account:purge"
	TypeAccountExport = "account:export"
)

// Mail task options. Retention is 0 on purpose: the payload holds one-time
// links, so a finished task must not linger in Redis.
const (
	mailMaxRetry = 5
	mailTimeout  = 2 * time.Minute
)

type mailPayload struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Text     string `json:"text"`
	HTML     string `json:"html"`
	Template string `json:"template"`
	// TokenID is the stored token row behind the link (never the raw token).
	TokenID string `json:"token_id,omitempty"`
}

// NewMailTask builds the mail:send task and its options for a queue.
func NewMailTask(queue string, m port.Message) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(mailPayload{To: m.To, Subject: m.Subject, Text: m.Text, HTML: m.HTML, Template: m.Template, TokenID: m.TokenID})
	if err != nil {
		return nil, nil, err
	}
	return asynq.NewTask(TypeMailSend, payload),
		[]asynq.Option{asynq.Queue(queue), asynq.MaxRetry(mailMaxRetry), asynq.Retention(0), asynq.Timeout(mailTimeout)}, nil
}

// MailQueue implements port.MailQueue on top of the Asynq client.
type MailQueue struct{ c *Client }

// MailQueue returns the port.MailQueue backed by this client.
func (c *Client) MailQueue() *MailQueue { return &MailQueue{c: c} }

// Enqueue schedules m for delivery by the worker.
func (q *MailQueue) Enqueue(ctx context.Context, m port.Message) error {
	task, opts, err := NewMailTask(q.c.queue, m)
	if err != nil {
		return err
	}
	if _, err := q.c.client.EnqueueContext(ctx, task, opts...); err != nil {
		return fmt.Errorf("queue: enqueue mail: %w", err)
	}
	return nil
}

// MailHandler adapts a Mailer to an Asynq handler. When the final attempt fails, the task is archived with its
// rendered body (and so its link); retire, if set, invalidates the token behind that mail first, so the archive
// never holds a live link. The task's TokenID is the stored token's row id, not the raw token.
func MailHandler(m port.Mailer, retire func(ctx context.Context, tokenID string) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var pl mailPayload
		if err := json.Unmarshal(t.Payload(), &pl); err != nil {
			return fmt.Errorf("bad payload: %v: %w", err, asynq.SkipRetry)
		}
		err := m.Send(ctx, port.Message{To: pl.To, Subject: pl.Subject, Text: pl.Text, HTML: pl.HTML, Template: pl.Template, TokenID: pl.TokenID})
		if err == nil || pl.TokenID == "" || retire == nil {
			return err
		}
		retried, _ := asynq.GetRetryCount(ctx)
		if max, ok := asynq.GetMaxRetry(ctx); ok && retried >= max {
			if rerr := retire(ctx, pl.TokenID); rerr != nil {
				return fmt.Errorf("%w (retiring the undelivered link also failed: %v)", err, rerr)
			}
		}
		return err
	}
}

type forgotPayload struct {
	Email string `json:"email"`
}

// ForgotQueue implements auth.ForgotQueue on top of the Asynq client.
type ForgotQueue struct{ c *Client }

// ForgotQueue returns the auth.ForgotQueue backed by this client.
func (c *Client) ForgotQueue() *ForgotQueue { return &ForgotQueue{c: c} }

// EnqueueForgot schedules the password-reset lookup for the worker. Retention is 0: the payload holds an address.
func (q *ForgotQueue) EnqueueForgot(ctx context.Context, email string) error {
	payload, err := json.Marshal(forgotPayload{Email: email})
	if err != nil {
		return err
	}
	_, err = q.c.client.EnqueueContext(ctx, asynq.NewTask(TypeAuthForgot, payload),
		asynq.Queue(q.c.queue), asynq.MaxRetry(mailMaxRetry), asynq.Retention(0), asynq.Timeout(mailTimeout))
	if err != nil {
		return fmt.Errorf("queue: enqueue forgot: %w", err)
	}
	return nil
}

// ForgotHandler adapts the worker side of a password-reset request to an Asynq handler.
func ForgotHandler(process func(ctx context.Context, email string) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var pl forgotPayload
		if err := json.Unmarshal(t.Payload(), &pl); err != nil || pl.Email == "" {
			return fmt.Errorf("bad payload: %w", asynq.SkipRetry)
		}
		return process(ctx, pl.Email)
	}
}

// Export task options. The payload is only an id. MaxRetry is 0: a failed build is recorded on the export and the
// user asks again, a blind retry would redo hours of work (D-018). Retention is 0 so a finished id can be queued again.
const exportTimeout = dataexport.BuildTimeout

type exportPayload struct {
	ExportID uuid.UUID `json:"export_id"`
}

// ExportQueue implements account.ExportQueue on top of the Asynq client.
type ExportQueue struct{ c *Client }

// ExportQueue returns the account.ExportQueue backed by this client.
func (c *Client) ExportQueue() *ExportQueue { return &ExportQueue{c: c} }

// ExportQueueName is the queue of a main queue's export builds. It has its own Asynq server with concurrency 1, so a
// build that runs for an hour never occupies a slot of the publishing workers (D-018).
func ExportQueueName(main string) string { return main + "-exports" }

// EnqueueExport schedules the build. No task id or uniqueness key: an archived task would keep its id and block the
// sweep from ever queueing that export again, and a duplicate task is harmless because the build claims the row.
func (q *ExportQueue) EnqueueExport(ctx context.Context, exportID uuid.UUID) error {
	payload, err := json.Marshal(exportPayload{ExportID: exportID})
	if err != nil {
		return err
	}
	_, err = q.c.client.EnqueueContext(ctx, asynq.NewTask(TypeAccountExport, payload), asynq.Queue(ExportQueueName(q.c.queue)),
		asynq.MaxRetry(0), asynq.Retention(0), asynq.Timeout(exportTimeout))
	if err != nil {
		return fmt.Errorf("queue: enqueue export: %w", err)
	}
	return nil
}

// ExportHandler adapts the worker side of an export to an Asynq handler.
func ExportHandler(build func(ctx context.Context, exportID uuid.UUID) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var pl exportPayload
		if err := json.Unmarshal(t.Payload(), &pl); err != nil || pl.ExportID == uuid.Nil {
			return fmt.Errorf("bad payload: %w", asynq.SkipRetry)
		}
		return build(ctx, pl.ExportID)
	}
}

// Purge task options. A purge runs on the maintenance queue (see ExportQueueName). It retries only twice: the hourly
// sweep re-queues every account that is still due or half purged, so it is the outer retry loop (D-019). Unique is a
// little shorter than the sweep period plus the retries' backoff, so a waiting or running purge is not queued twice.
const (
	purgeMaxRetry = 2
	purgeTimeout  = 30 * time.Minute
	purgeUnique   = 55 * time.Minute
)

type purgePayload struct {
	UserID uuid.UUID `json:"user_id"`
}

// PurgeQueue implements account.PurgeQueue on top of the Asynq client.
type PurgeQueue struct{ c *Client }

// PurgeQueue returns the account.PurgeQueue backed by this client.
func (c *Client) PurgeQueue() *PurgeQueue { return &PurgeQueue{c: c} }

// EnqueuePurge schedules the purge of one account. A purge already queued for the same user is not queued twice.
func (q *PurgeQueue) EnqueuePurge(ctx context.Context, userID uuid.UUID) error {
	payload, err := json.Marshal(purgePayload{UserID: userID})
	if err != nil {
		return err
	}
	_, err = q.c.client.EnqueueContext(ctx, asynq.NewTask(TypeAccountPurge, payload), asynq.Queue(ExportQueueName(q.c.queue)),
		asynq.MaxRetry(purgeMaxRetry), asynq.Retention(0), asynq.Timeout(purgeTimeout), asynq.Unique(purgeUnique))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("queue: enqueue purge: %w", err)
	}
	return nil
}

// PurgeHandler adapts the worker side of an account purge to an Asynq handler.
func PurgeHandler(purge func(ctx context.Context, userID uuid.UUID) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var pl purgePayload
		if err := json.Unmarshal(t.Payload(), &pl); err != nil || pl.UserID == uuid.Nil {
			return fmt.Errorf("bad payload: %w", asynq.SkipRetry)
		}
		return purge(ctx, pl.UserID)
	}
}
