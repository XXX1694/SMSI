package scheduler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/post"
)

var errInfra = errors.New("db is down")

func perr(kind provider.Kind, code string) *provider.Error {
	return &provider.Error{Kind: kind, Provider: "fake", Code: code, Message: "msg " + code}
}

// runJob runs the seeded job once and returns Run's error.
func runJob(w *schedulertest.World, ri scheduler.RetryInfo) error {
	return w.Publisher().Run(context.Background(), w.Payload(), ri)
}

func wantNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
}

func wantRetry(t *testing.T, err error) {
	t.Helper()
	if !scheduler.IsRetryable(err) {
		t.Fatalf("Run error = %v, want RetryableError", err)
	}
}

func wantTarget(t *testing.T, w *schedulertest.World, st post.TargetStatus, code string) {
	t.Helper()
	tg := w.Target()
	if tg.Status != st || tg.ErrorCode != code {
		t.Fatalf("target = %s/%q, want %s/%q", tg.Status, tg.ErrorCode, st, code)
	}
}

func wantPost(t *testing.T, w *schedulertest.World, st post.Status) {
	t.Helper()
	if got := w.Post().Status; got != st {
		t.Fatalf("post status = %s, want %s", got, st)
	}
}

func wantLastAttempt(t *testing.T, w *schedulertest.World, st post.AttemptStatus) {
	t.Helper()
	as := w.Store.AttemptsFor(w.TargetID)
	if len(as) == 0 || as[len(as)-1].Status != st {
		t.Fatalf("attempts = %+v, want last %s", as, st)
	}
}

func wantJob(t *testing.T, w *schedulertest.World, st post.JobStatus) {
	t.Helper()
	if got := w.Store.Jobs[w.JobID].Status; got != st {
		t.Fatalf("job status = %s, want %s", got, st)
	}
}

func wantAudit(t *testing.T, w *schedulertest.World, actions ...string) {
	t.Helper()
	got := w.Store.AuditActions()
	if len(got) != len(actions) {
		t.Fatalf("audit = %v, want %v", got, actions)
	}
	for i := range got {
		if got[i] != actions[i] {
			t.Fatalf("audit = %v, want %v", got, actions)
		}
	}
}
