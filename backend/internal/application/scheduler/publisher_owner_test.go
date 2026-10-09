package scheduler_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/post"
)

type owners struct{ reason string }

func (o owners) BlockReason(context.Context, uuid.UUID) (string, error) { return o.reason, nil }

// storeUnscheduler stands in for posts.Service.UnscheduleForOwner (covered end to end against Postgres in the e2e suite).
type storeUnscheduler struct {
	w       *schedulertest.World
	reasons []string
}

func (u *storeUnscheduler) UnscheduleForOwner(_ context.Context, _, postID uuid.UUID, reason string) error {
	u.reasons = append(u.reasons, reason)
	p := u.w.Store.Posts[postID]
	p.Status, p.ScheduledAt = post.StatusDraft, nil
	u.w.Store.Posts[postID] = p
	for id, j := range u.w.Store.Jobs {
		if j.Status.Active() {
			j.Status = post.JobCancelled
			u.w.Store.Jobs[id] = j
		}
	}
	return nil
}

func runWithOwners(w *schedulertest.World, o scheduler.Owners) (*storeUnscheduler, error) {
	d := w.Deps()
	d.Owners = o
	u := &storeUnscheduler{w: w}
	d.Unscheduler = u
	return u, scheduler.NewPublisher(d).Run(context.Background(), w.Payload(), retryInfo())
}

// A job that comes due while its owner's account is scheduled for deletion (or deleted) sends nothing: the post goes
// back to a draft, the job is finished and no attempt is made (D-019).
func TestDueJobOfAnAccountScheduledForDeletionIsNotPublished(t *testing.T) {
	prov := schedulertest.NewProvider("fake")
	w := schedulertest.NewWorld(prov)
	u, err := runWithOwners(w, owners{reason: scheduler.SkipAccountDeletion})
	wantNil(t, err)
	if len(u.reasons) != 1 || u.reasons[0] != scheduler.SkipAccountDeletion {
		t.Fatalf("unscheduled with reasons %v, want [account_deletion]", u.reasons)
	}
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
	_, err := runWithOwners(w, owners{})
	wantNil(t, err)
	if prov.PublishCalls() != 1 {
		t.Fatalf("provider called %d times, want 1", prov.PublishCalls())
	}
}

// A disabled owner is skipped with its own reason, not reported as a deletion.
func TestDueJobOfADisabledOwnerIsSkippedWithItsOwnReason(t *testing.T) {
	prov := schedulertest.NewProvider("fake")
	w := schedulertest.NewWorld(prov)
	u, err := runWithOwners(w, owners{reason: scheduler.SkipOwnerDisabled})
	wantNil(t, err)
	if len(u.reasons) != 1 || u.reasons[0] != scheduler.SkipOwnerDisabled || prov.PublishCalls() != 0 {
		t.Fatalf("reasons %v, publish calls %d", u.reasons, prov.PublishCalls())
	}
}

// A post that is already publishing cannot go back to a draft: the target fails with ACCOUNT_DELETION_SCHEDULED, the job
// is finished and the post settles to failed, so it can be retried once the deletion is cancelled.
func TestDueJobOfAPublishingPostOfAnAccountBeingDeletedFailsTheTarget(t *testing.T) {
	prov := schedulertest.NewProvider("fake")
	w := schedulertest.NewWorld(prov)
	p := w.Store.Posts[w.PostID]
	p.Status = post.StatusPublishing
	w.Store.Posts[w.PostID] = p

	u, err := runWithOwners(w, owners{reason: scheduler.SkipAccountDeletion})
	wantNil(t, err)
	if len(u.reasons) != 0 || prov.PublishCalls() != 0 {
		t.Fatalf("a publishing post must not be unscheduled (%v) or sent (%d)", u.reasons, prov.PublishCalls())
	}
	if tg := w.Target(); tg.Status != post.TargetFailed || tg.ErrorCode != "ACCOUNT_DELETION_SCHEDULED" {
		t.Fatalf("target = %s/%s, want failed/ACCOUNT_DELETION_SCHEDULED", tg.Status, tg.ErrorCode)
	}
	wantPost(t, w, post.StatusFailed)
	if j := w.Store.Jobs[w.JobID]; j.Status.Active() {
		t.Fatalf("job still active: %s", j.Status)
	}
}

// A target that is already published (or whose post is no longer in flight) is finished as usual; the owner check only
// concerns a job that would really publish.
func TestOwnerCheckComesAfterTheTerminalChecks(t *testing.T) {
	prov := schedulertest.NewProvider("fake")
	w := schedulertest.NewWorld(prov)
	tg := w.Store.Targets[w.TargetID]
	tg.Status = post.TargetPublished
	w.Store.Targets[w.TargetID] = tg

	u, err := runWithOwners(w, owners{reason: scheduler.SkipAccountDeletion})
	wantNil(t, err)
	if len(u.reasons) != 0 {
		t.Fatalf("the owner check ran for a published target: %v", u.reasons)
	}
	if j := w.Store.Jobs[w.JobID]; j.Status.Active() {
		t.Fatalf("job still active: %s", j.Status)
	}
}
