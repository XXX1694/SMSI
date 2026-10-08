package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/socialos/backend/internal/application/port"
)

// Task types. TypeAccountPurge and TypeAccountExport are reserved for the
// account-deletion and data-export work; only TypeMailSend has a handler yet.
const (
	TypeMailSend      = "mail:send"
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
}

// NewMailTask builds the mail:send task and its options for a queue.
func NewMailTask(queue string, m port.Message) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(mailPayload{To: m.To, Subject: m.Subject, Text: m.Text, HTML: m.HTML, Template: m.Template})
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

// MailHandler adapts a Mailer to an Asynq handler.
func MailHandler(m port.Mailer) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var pl mailPayload
		if err := json.Unmarshal(t.Payload(), &pl); err != nil {
			return fmt.Errorf("bad payload: %v: %w", err, asynq.SkipRetry)
		}
		return m.Send(ctx, port.Message{To: pl.To, Subject: pl.Subject, Text: pl.Text, HTML: pl.HTML, Template: pl.Template})
	}
}
