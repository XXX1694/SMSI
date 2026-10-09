package scheduler_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/post"
)

type owners struct{ publishable bool }

func (o owners) Publishable(context.Context, uuid.UUID) (bool, error) { return o.publishable, nil }

func runWithOwners(w *schedulertest.World, o scheduler.Owners) error {
	d := w.Deps()
	d.Owners = o
	return scheduler.NewPublisher(d).Run(context.Background(), w.Payload(), retryInfo())
}

// A job that comes due while its owner's account is scheduled for deletion (or deleted) sends nothing: the post goes
// back to a draft, the job is finished and no attempt is made (D-019).
func TestDueJobOfAnAccountScheduledForDeletionIsNotPublished(t *testing.T) {
	prov := schedulertest.NewProvider("fake")
	w := schedulertest.NewWorld(prov)
	wantNil(t, runWithOwners(w, owners{publishable: false}))
	if prov.PublishCalls() != 0 {
		t.Fatalf("provider called %d times for an account being deleted", prov.PublishCalls())
	}
	wantPost(t, w, post.StatusDraft)
	if j := w.Store.Jobs[w.JobID]; j.Status.Active() {
		t.Fatalf("job still active: %s", j.Status)
	}
	if got := w.Target().Status; got != post.TargetPending {
		t.Fatalf("target = %s, want pending", got)
	}
}

func TestDueJobOfAnActiveAccountStillPublishes(t *testing.T) {
	prov := schedulertest.NewProvider("fake")
	w := schedulertest.NewWorld(prov)
	wantNil(t, runWithOwners(w, owners{publishable: true}))
	if prov.PublishCalls() != 1 {
		t.Fatalf("provider called %d times, want 1", prov.PublishCalls())
	}
}
