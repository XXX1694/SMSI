package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/port"
)

type captureMailer struct {
	mu  sync.Mutex
	got []port.Message
}

func (c *captureMailer) Send(_ context.Context, m port.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, m)
	return nil
}

func (c *captureMailer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.got)
}

// The API process enqueues; the worker delivers through the Mailer.
func TestMailQueuedByAPIIsSentByWorker(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm})
	want := port.Message{To: "u@example.com", Subject: "Hi", Text: "t", HTML: "<p>t</p>", Template: "verify_email"}
	if err := e.app.MailQueue.Enqueue(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for cm.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if cm.count() != 1 || cm.got[0] != want {
		t.Fatalf("delivered: %+v", cm.got)
	}
}
