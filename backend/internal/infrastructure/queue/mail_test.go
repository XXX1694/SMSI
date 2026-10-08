package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/socialos/backend/internal/application/port"
)

type fakeMailer struct {
	got []port.Message
	err error
}

func (f *fakeMailer) Send(_ context.Context, m port.Message) error {
	f.got = append(f.got, m)
	return f.err
}

func TestMailTaskRoundTripsThroughHandler(t *testing.T) {
	in := port.Message{To: "a@example.com", Subject: "S", Text: "t", HTML: "<p>t</p>", Template: "verify_email"}
	task, opts, err := NewMailTask("q", in)
	if err != nil {
		t.Fatal(err)
	}
	if task.Type() != TypeMailSend || TypeMailSend != "mail:send" {
		t.Fatalf("type %q", task.Type())
	}
	var maxRetry, retention = -1, -1
	for _, o := range opts {
		switch o.Type() {
		case asynq.MaxRetryOpt:
			maxRetry = o.Value().(int)
		case asynq.RetentionOpt:
			retention = int(o.Value().(interface{ Seconds() float64 }).Seconds())
		}
	}
	if maxRetry != 5 || retention != 0 {
		t.Fatalf("MaxRetry=%d Retention=%ds, want 5 and 0", maxRetry, retention)
	}
	m := &fakeMailer{}
	if err := MailHandler(m, nil)(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(m.got) != 1 || m.got[0] != in {
		t.Fatalf("handler delivered %+v", m.got)
	}
}

func TestMailHandlerErrors(t *testing.T) {
	m := &fakeMailer{err: errors.New("smtp down")}
	task, _, _ := NewMailTask("q", port.Message{To: "a@example.com"})
	if err := MailHandler(m, nil)(context.Background(), task); err == nil || errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("send failures must be retried, got %v", err)
	}
	bad := asynq.NewTask(TypeMailSend, []byte("{"))
	if err := MailHandler(m, nil)(context.Background(), bad); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("a corrupt payload must not be retried, got %v", err)
	}
}
