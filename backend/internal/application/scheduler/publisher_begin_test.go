package scheduler_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

func providerCalls(w *schedulertest.World) int {
	switch p := w.Provider.(type) {
	case *schedulertest.Provider:
		return p.PublishCalls()
	case *schedulertest.LookupProvider:
		return p.PublishCalls()
	}
	return -1
}

func TestRunNoOpWhenNothingToPublish(t *testing.T) {
	tests := []struct {
		name  string
		setup func(w *schedulertest.World)
	}{
		{"job cancelled", func(w *schedulertest.World) { w.SetJob(func(j *post.Job) { j.Status = post.JobCancelled }) }},
		{"job already done", func(w *schedulertest.World) { w.SetJob(func(j *post.Job) { j.Status = post.JobDone }) }},
		{"job gone", func(w *schedulertest.World) { delete(w.Store.Jobs, w.JobID) }},
		{"target removed", func(w *schedulertest.World) { delete(w.Store.Targets, w.TargetID) }},
		{"target already published", func(w *schedulertest.World) {
			w.SetTarget(func(tg *post.Target) { tg.Status = post.TargetPublished })
		}},
		{"target cancelled", func(w *schedulertest.World) {
			w.SetTarget(func(tg *post.Target) { tg.Status = post.TargetCancelled })
		}},
		{"target needs review", func(w *schedulertest.World) {
			w.SetTarget(func(tg *post.Target) { tg.Status = post.TargetNeedsReview })
		}},
		{"post cancelled", func(w *schedulertest.World) { w.SetPost(func(p *post.Post) { p.Status = post.StatusCancelled }) }},
		{"post back to draft", func(w *schedulertest.World) { w.SetPost(func(p *post.Post) { p.Status = post.StatusDraft }) }},
		{"post deleted", func(w *schedulertest.World) { delete(w.Store.Posts, w.PostID) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := schedulertest.NewWorld(nil)
			tc.setup(w)
			wantNil(t, runJob(w, scheduler.RetryInfo{}))
			if providerCalls(w) != 0 || len(w.Store.Attempts) != 0 || len(w.Observed) != 0 {
				t.Fatalf("expected no-op, calls=%d attempts=%d", providerCalls(w), len(w.Store.Attempts))
			}
		})
	}
}

func TestRunNoOpFinishesTheJob(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.SetTarget(func(tg *post.Target) { tg.Status = post.TargetPublished })
	wantNil(t, runJob(w, scheduler.RetryInfo{}))
	wantJob(t, w, post.JobDone)
}

func TestRunIdempotentWhenExternalIDAlreadyStored(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.SetTarget(func(tg *post.Target) { tg.ExternalPostID, tg.ExternalURL = "ext-old", "https://x.test/old" })
	wantNil(t, runJob(w, scheduler.RetryInfo{}))
	if providerCalls(w) != 0 {
		t.Fatal("provider must not be called again")
	}
	if tg := w.Target(); tg.Status != post.TargetPublished || tg.ExternalPostID != "ext-old" {
		t.Fatalf("target = %+v", tg)
	}
	wantPost(t, w, post.StatusPublished)
	wantJob(t, w, post.JobDone)
}

func TestRunLockedTargetIsRetryable(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.Store.Locked[w.TargetID] = true
	wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	if providerCalls(w) != 0 {
		t.Fatal("provider called while target locked")
	}
}

func TestRunInfraFailuresBeforeProviderAreRetryable(t *testing.T) {
	for _, op := range []string{"Jobs.Get", "Targets.LockTarget", "Posts.GetForUpdate", "Accounts.Get",
		"Targets.LatestAttempt", "Posts.Update", "Targets.UpdateTarget", "Targets.InsertAttempt", "Tx.InTx"} {
		t.Run(op, func(t *testing.T) {
			w := schedulertest.NewWorld(nil)
			w.Store.Errors[op] = errInfra
			wantRetry(t, runJob(w, scheduler.RetryInfo{}))
			if providerCalls(w) != 0 {
				t.Fatal("provider called after infra failure")
			}
			// the begin tx rolled back: nothing half-written
			if tg := w.Target(); tg.Status != post.TargetPending || tg.AttemptCount != 0 || len(w.Store.Attempts) != 0 {
				t.Fatalf("not rolled back: %+v attempts=%d", tg, len(w.Store.Attempts))
			}
		})
	}
}

func TestRunAccountProblems(t *testing.T) {
	t.Run("expired account fails without calling provider", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetAccount(func(a *socialaccount.Account) { a.Status = socialaccount.StatusExpired })
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		wantTarget(t, w, post.TargetFailed, string(errs.SocialAccountExpired))
		wantPost(t, w, post.StatusFailed)
		if providerCalls(w) != 0 {
			t.Fatal("provider called for expired account")
		}
	})
	t.Run("unregistered provider fails", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetAccount(func(a *socialaccount.Account) { a.Provider = "nope" })
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		wantTarget(t, w, post.TargetFailed, string(errs.ProviderNotAvailable))
	})
	t.Run("stub provider fails", func(t *testing.T) {
		p := schedulertest.NewProvider("fake")
		p.Stub = true
		w := schedulertest.NewWorld(p)
		wantNil(t, runJob(w, scheduler.RetryInfo{}))
		wantTarget(t, w, post.TargetFailed, string(errs.ProviderNotAvailable))
	})
	t.Run("account of another user is not found", func(t *testing.T) {
		w := schedulertest.NewWorld(nil)
		w.SetAccount(func(a *socialaccount.Account) { a.UserID = uuid.New() })
		wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	})
}

func TestRunRecordingFailureAfterPublishIsRetryableAndLeavesAttemptStarted(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.Store.Errors["Audit.Record"] = errInfra
	wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	if providerCalls(w) != 1 {
		t.Fatalf("provider calls = %d", providerCalls(w))
	}
	// success tx rolled back; the begin tx stays committed: attempt 'started', target 'publishing'
	wantTarget(t, w, post.TargetPublishing, "")
	wantLastAttempt(t, w, post.AttemptStarted)
}

func TestRunFailRecordingErrorIsRetryable(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	w.Provider.(*schedulertest.Provider).PublishErrs = []error{perr(provider.KindPermanent, "HTTP_400")}
	w.Store.Errors["Metrics.Increment"] = errInfra
	wantRetry(t, runJob(w, scheduler.RetryInfo{}))
	wantTarget(t, w, post.TargetPublishing, "")
}

func TestRunAfterCrashedWorker(t *testing.T) {
	old := schedulertest.Epoch.Add(-InFlight - time.Second)
	tests := []struct {
		name       string
		prov       func() provider.Provider
		status     post.AttemptStatus
		started    time.Time
		wantRetry  bool
		wantTarget post.TargetStatus
		publishes  int
		lookups    int
	}{
		{name: "fresh started attempt means another worker is live", status: post.AttemptStarted, started: schedulertest.Epoch,
			wantRetry: true, wantTarget: post.TargetPublishing},
		{name: "stale started on plain provider needs review", status: post.AttemptStarted, started: old,
			wantTarget: post.TargetNeedsReview},
		{name: "stale started on idempotent provider republishes", status: post.AttemptStarted, started: old,
			prov: func() provider.Provider {
				p := schedulertest.NewProvider("fake")
				p.Caps.SafeToRetryAfterUnknown = true
				return p
			}, wantTarget: post.TargetPublished, publishes: 1},
		{name: "stale started on lookup provider looks up then publishes", status: post.AttemptStarted, started: old,
			prov:       func() provider.Provider { return schedulertest.NewLookupProvider("fake") },
			wantTarget: post.TargetPublished, publishes: 1, lookups: 1},
		{name: "unknown attempt on lookup provider looks up first", status: post.AttemptUnknown, started: old,
			prov:       func() provider.Provider { return schedulertest.NewLookupProvider("fake") },
			wantTarget: post.TargetPublished, publishes: 1, lookups: 1},
		{name: "unknown attempt on plain provider just republishes", status: post.AttemptUnknown, started: old,
			wantTarget: post.TargetPublished, publishes: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var prov provider.Provider
			if tc.prov != nil {
				prov = tc.prov()
			}
			w := schedulertest.NewWorld(prov)
			w.SetTarget(func(tg *post.Target) { tg.Status, tg.AttemptCount = post.TargetPublishing, 1 })
			w.AddAttempt(1, tc.status, tc.started)
			err := runJob(w, scheduler.RetryInfo{})
			if tc.wantRetry {
				wantRetry(t, err)
			} else {
				wantNil(t, err)
			}
			wantTarget(t, w, tc.wantTarget, map[bool]string{true: "OUTCOME_UNKNOWN"}[tc.wantTarget == post.TargetNeedsReview])
			if providerCalls(w) != tc.publishes {
				t.Fatalf("publishes = %d, want %d", providerCalls(w), tc.publishes)
			}
			if lp, ok := w.Provider.(*schedulertest.LookupProvider); ok && lp.Lookups != tc.lookups {
				t.Fatalf("lookups = %d, want %d", lp.Lookups, tc.lookups)
			}
		})
	}
}

func TestRunLookupResolvesWithoutRepublishing(t *testing.T) {
	lp := schedulertest.NewLookupProvider("fake")
	lp.Found = &provider.PublishResult{ExternalID: "ext-found", URL: "https://x.test/found"}
	w := schedulertest.NewWorld(lp)
	w.SetTarget(func(tg *post.Target) { tg.Status, tg.AttemptCount = post.TargetPublishing, 1 })
	w.AddAttempt(1, post.AttemptUnknown, schedulertest.Epoch.Add(-time.Hour))
	wantNil(t, runJob(w, scheduler.RetryInfo{}))
	if lp.PublishCalls() != 0 {
		t.Fatal("must not republish a post found by lookup")
	}
	if tg := w.Target(); tg.Status != post.TargetPublished || tg.ExternalPostID != "ext-found" {
		t.Fatalf("target = %+v", tg)
	}
	as := w.Store.AttemptsFor(w.TargetID)
	if as[len(as)-1].ResponseMetadata["resolved_by"] != "lookup" {
		t.Fatalf("attempt metadata = %v", as[len(as)-1].ResponseMetadata)
	}
	wantAudit(t, w, audit.ActionTargetPublished, audit.ActionPostCompleted)
}

func TestRunLookupErrorIsClassified(t *testing.T) {
	lp := schedulertest.NewLookupProvider("fake")
	lp.LookupErr = perr(provider.KindRetryable, "HTTP_503")
	w := schedulertest.NewWorld(lp)
	w.AddAttempt(1, post.AttemptUnknown, schedulertest.Epoch.Add(-time.Hour))
	w.SetTarget(func(tg *post.Target) { tg.AttemptCount = 1 })
	wantRetry(t, runJob(w, scheduler.RetryInfo{MaxRetry: 5}))
	if lp.PublishCalls() != 0 {
		t.Fatal("publish after failed lookup")
	}
}

// InFlight mirrors scheduler.InFlightWindow for readability.
const InFlight = scheduler.InFlightWindow
