package scheduler_test

import (
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/application/scheduler/schedulertest"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

func TestRunSuccess(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	wantNil(t, runJob(w, scheduler.RetryInfo{}))

	tg := w.Target()
	if tg.Status != post.TargetPublished || tg.ExternalPostID != "ext-1" || tg.PublishedAt == nil || tg.AttemptCount != 1 {
		t.Fatalf("target = %+v", tg)
	}
	wantPost(t, w, post.StatusPublished)
	if w.Post().PublishedAt == nil {
		t.Fatal("post PublishedAt not set")
	}
	wantLastAttempt(t, w, post.AttemptSucceeded)
	wantJob(t, w, post.JobDone)
	wantAudit(t, w, audit.ActionTargetPublished, audit.ActionPostCompleted)
	if m := w.Store.Metrics; len(m) != 1 || m[0].Metric != scheduler.MetricPublished {
		t.Fatalf("metrics = %+v", m)
	}
	if len(w.Observed) != 1 || w.Observed[0] != "fake:published" {
		t.Fatalf("observed = %v", w.Observed)
	}
	req := w.Provider.(*schedulertest.Provider).Requests[0]
	if req.IdempotencyKey != "idem-1" || req.Text != "hello" || req.Account.Username != "acme" {
		t.Fatalf("request = %+v", req)
	}
}

func TestRunProviderErrors(t *testing.T) {
	notExhausted, exhausted := scheduler.RetryInfo{Retried: 1, MaxRetry: 5}, scheduler.RetryInfo{Retried: 5, MaxRetry: 5}
	tests := []struct {
		name      string
		prov      func() provider.Provider
		err       error
		ri        scheduler.RetryInfo
		wantRetry bool
		target    post.TargetStatus
		code      string
		post      post.Status
		attempt   post.AttemptStatus
		outcome   string
		expired   bool
	}{
		{name: "retryable with retries left", err: perr(provider.KindRetryable, "HTTP_503"), ri: notExhausted, wantRetry: true,
			target: post.TargetPending, code: "HTTP_503", post: post.StatusPublishing, attempt: post.AttemptFailed, outcome: "retry"},
		{name: "retryable exhausted fails", err: perr(provider.KindRetryable, "HTTP_503"), ri: exhausted,
			target: post.TargetFailed, code: "HTTP_503", post: post.StatusFailed, attempt: post.AttemptFailed, outcome: "failed"},
		{name: "permanent fails", err: perr(provider.KindPermanent, "HTTP_400"), ri: notExhausted,
			target: post.TargetFailed, code: "HTTP_400", post: post.StatusFailed, attempt: post.AttemptFailed, outcome: "failed"},
		{name: "plain error is permanent", err: errInfra, ri: notExhausted,
			target: post.TargetFailed, code: "PROVIDER_ERROR", post: post.StatusFailed, attempt: post.AttemptFailed, outcome: "failed"},
		{name: "auth expires account", err: perr(provider.KindAuth, "HTTP_401"), ri: notExhausted, expired: true,
			target: post.TargetFailed, code: string(errs.SocialAccountExpired), post: post.StatusFailed, attempt: post.AttemptFailed, outcome: "failed"},
		{name: "unsupported", err: provider.ErrUnsupported, ri: notExhausted,
			target: post.TargetFailed, code: string(errs.ProviderNotAvailable), post: post.StatusFailed, attempt: post.AttemptFailed, outcome: "failed"},
		{name: "unknown on plain provider needs review", err: perr(provider.KindUnknown, "TIMEOUT"), ri: notExhausted,
			target: post.TargetNeedsReview, code: "OUTCOME_UNKNOWN", post: post.StatusFailed, attempt: post.AttemptUnknown, outcome: "needs_review"},
		{name: "unknown on idempotent provider retries", err: perr(provider.KindUnknown, "TIMEOUT"), ri: notExhausted, wantRetry: true,
			prov: func() provider.Provider {
				p := schedulertest.NewProvider("fake")
				p.Caps.SafeToRetryAfterUnknown = true
				return p
			},
			target: post.TargetPending, code: "OUTCOME_UNKNOWN", post: post.StatusPublishing, attempt: post.AttemptUnknown, outcome: "retry"},
		{name: "unknown on idempotent provider exhausted needs review", err: perr(provider.KindUnknown, "TIMEOUT"), ri: exhausted,
			prov: func() provider.Provider {
				p := schedulertest.NewProvider("fake")
				p.Caps.SafeToRetryAfterUnknown = true
				return p
			},
			target: post.TargetNeedsReview, code: "OUTCOME_UNKNOWN", post: post.StatusFailed, attempt: post.AttemptUnknown, outcome: "needs_review"},
		{name: "unknown on lookup provider retries", err: perr(provider.KindUnknown, "TIMEOUT"), ri: notExhausted, wantRetry: true,
			prov:   func() provider.Provider { return schedulertest.NewLookupProvider("fake") },
			target: post.TargetPending, code: "OUTCOME_UNKNOWN", post: post.StatusPublishing, attempt: post.AttemptUnknown, outcome: "retry"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var prov provider.Provider
			if tc.prov != nil {
				prov = tc.prov()
			}
			w := schedulertest.NewWorld(prov)
			switch p := w.Provider.(type) {
			case *schedulertest.Provider:
				p.PublishErrs = []error{tc.err}
			case *schedulertest.LookupProvider:
				p.PublishErrs = []error{tc.err}
			}
			err := runJob(w, tc.ri)
			if tc.wantRetry {
				wantRetry(t, err)
			} else {
				wantNil(t, err)
			}
			wantTarget(t, w, tc.target, tc.code)
			wantPost(t, w, tc.post)
			wantLastAttempt(t, w, tc.attempt)
			if got := w.Observed; len(got) != 1 || got[0] != "fake:"+tc.outcome {
				t.Fatalf("observed = %v, want %s", got, tc.outcome)
			}
			if exp := w.Store.Accounts[w.AccountID].Status == socialaccount.StatusExpired; exp != tc.expired {
				t.Fatalf("account expired = %v, want %v", exp, tc.expired)
			}
			if tc.target.Terminal() {
				wantJob(t, w, post.JobDone)
			} else {
				wantJob(t, w, post.JobEnqueued)
			}
		})
	}
}

func TestRetryableErrorCarriesProviderCause(t *testing.T) {
	w := schedulertest.NewWorld(nil)
	cause := perr(provider.KindRetryable, "HTTP_429")
	cause.RetryAfter = 90e9
	w.Provider.(*schedulertest.Provider).PublishErrs = []error{cause}
	err := runJob(w, scheduler.RetryInfo{MaxRetry: 5})
	wantRetry(t, err)
	if d := scheduler.RetryDelay(0, err); d < 90e9 {
		t.Fatalf("RetryDelay = %s, want >= Retry-After 90s", d)
	}
}

func TestRetryInfoExhausted(t *testing.T) {
	if (scheduler.RetryInfo{Retried: 4, MaxRetry: 5}).Exhausted() || !(scheduler.RetryInfo{Retried: 5, MaxRetry: 5}).Exhausted() {
		t.Fatal("Exhausted boundary wrong")
	}
}

func TestSiblingTargetsDecidePostStatus(t *testing.T) {
	tests := []struct {
		name    string
		sibling post.TargetStatus
		want    post.Status
	}{
		{"sibling failed gives partial", post.TargetFailed, post.StatusPartiallyPublished},
		{"sibling pending keeps publishing", post.TargetPending, post.StatusPublishing},
		{"sibling cancelled ignored", post.TargetCancelled, post.StatusPublished},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := schedulertest.NewWorld(nil)
			w.AddSiblingTarget(tc.sibling)
			wantNil(t, runJob(w, scheduler.RetryInfo{}))
			wantPost(t, w, tc.want)
		})
	}
}
