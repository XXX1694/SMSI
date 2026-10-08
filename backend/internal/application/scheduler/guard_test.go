package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/post"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeJobs struct {
	Jobs
	job      post.Job
	enqueued string
}

func (f *fakeJobs) Get(context.Context, uuid.UUID) (*post.Job, error) { j := f.job; return &j, nil }
func (f *fakeJobs) MarkEnqueued(_ context.Context, _ uuid.UUID, id string) error {
	f.enqueued = id
	return nil
}

// fakeTx records the clock at the moment the publish transaction begins.
type fakeTx struct {
	clock *fakeClock
	calls int
	at    time.Time
}

func (f *fakeTx) InTx(context.Context, func(context.Context) error) error {
	f.calls++
	f.at = f.clock.Now()
	return nil // begin sees no run: finished
}

type fakeQueue struct {
	Queue
	reenqueued []post.Job
}

func (q *fakeQueue) Reenqueue(_ context.Context, j post.Job) (string, error) {
	q.reenqueued = append(q.reenqueued, j)
	return "task-2", nil
}

func (c *fakeClock) sleep(_ context.Context, d time.Duration) error { c.now = c.now.Add(d); return nil }

var base = time.Date(2026, 10, 8, 21, 26, 10, 751_000_000, time.UTC)

func TestRunNeverStartsBeforeRunAt(t *testing.T) {
	clk := &fakeClock{now: base}
	tx := &fakeTx{clock: clk}
	runAt := base.Add(207 * time.Millisecond) // woken ~200 ms early by whole-second scoring
	jobs := &fakeJobs{job: post.Job{ID: uuid.New(), RunAt: runAt, Status: post.JobEnqueued}}
	p := NewPublisher(Deps{Jobs: jobs, Tx: tx, Clock: clk, Sleep: clk.sleep})
	if err := p.Run(context.Background(), Payload{JobID: jobs.job.ID}, RetryInfo{}); err != nil {
		t.Fatal(err)
	}
	if tx.calls == 0 || tx.at.Before(runAt) {
		t.Fatalf("publish began at %v, before scheduled %v (calls=%d)", tx.at, runAt, tx.calls)
	}
}

func TestRunReenqueuesWhenFarTooEarly(t *testing.T) {
	clk := &fakeClock{now: base}
	tx := &fakeTx{clock: clk}
	jobs := &fakeJobs{job: post.Job{ID: uuid.New(), RunAt: base.Add(time.Minute), Status: post.JobEnqueued}}
	q := &fakeQueue{}
	p := NewPublisher(Deps{Jobs: jobs, Tx: tx, Clock: clk, Queue: q, Sleep: clk.sleep})
	if err := p.Run(context.Background(), Payload{JobID: jobs.job.ID}, RetryInfo{}); err != nil {
		t.Fatal(err)
	}
	if tx.calls != 0 || len(q.reenqueued) != 1 || jobs.enqueued != "task-2" || !clk.now.Equal(base) {
		t.Fatalf("calls=%d reenqueued=%d task=%q now=%v", tx.calls, len(q.reenqueued), jobs.enqueued, clk.now)
	}
}

func TestRunFarTooEarlyWithoutQueueIsRetryable(t *testing.T) {
	clk := &fakeClock{now: base}
	tx := &fakeTx{clock: clk}
	jobs := &fakeJobs{job: post.Job{ID: uuid.New(), RunAt: base.Add(time.Minute), Status: post.JobEnqueued}}
	p := NewPublisher(Deps{Jobs: jobs, Tx: tx, Clock: clk})
	err := p.Run(context.Background(), Payload{JobID: jobs.job.ID}, RetryInfo{})
	if !IsRetryable(err) || tx.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, tx.calls)
	}
}

func TestRunSleepRespectsCancellation(t *testing.T) {
	clk := &fakeClock{now: base}
	tx := &fakeTx{clock: clk}
	jobs := &fakeJobs{job: post.Job{ID: uuid.New(), RunAt: base.Add(time.Second), Status: post.JobEnqueued}}
	p := NewPublisher(Deps{Jobs: jobs, Tx: tx, Clock: clk,
		Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Run(ctx, Payload{JobID: jobs.job.ID}, RetryInfo{})
	if !IsRetryable(err) || !errors.Is(err, context.Canceled) || tx.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, tx.calls)
	}
}

func TestRunDueJobDoesNotWait(t *testing.T) {
	clk := &fakeClock{now: base}
	tx := &fakeTx{clock: clk}
	jobs := &fakeJobs{job: post.Job{ID: uuid.New(), RunAt: base, Status: post.JobEnqueued}} // "publish now"
	p := NewPublisher(Deps{Jobs: jobs, Tx: tx, Clock: clk, Sleep: func(context.Context, time.Duration) error {
		t.Fatal("must not sleep")
		return nil
	}})
	if err := p.Run(context.Background(), Payload{JobID: jobs.job.ID}, RetryInfo{}); err != nil || tx.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, tx.calls)
	}
}
