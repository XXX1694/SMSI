package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Tunables.
const (
	// InFlightWindow: a `started` attempt younger than this belongs to a live worker.
	InFlightWindow = 5 * time.Minute
	// PublishTimeout bounds a single provider call (must be < InFlightWindow).
	PublishTimeout = 3 * time.Minute
	// RefreshWindow: refresh tokens expiring within this window.
	RefreshWindow = 5 * time.Minute
)

var errInFlight = errors.New("target is being published by another worker")
var errTooEarly = errors.New("task fired before the job run_at")

// Publisher executes publish:target jobs.
type Publisher struct {
	targets  Targets
	posts    Posts
	jobs     Jobs
	accounts Accounts
	vault    Vault
	media    MediaStore
	metrics  Metrics
	registry *provider.Registry
	tx       port.TxRunner
	audit    port.AuditRecorder
	clock    port.Clock
	log      *slog.Logger
	observe  func(provider, outcome string)
	queue    Queue
	sleep    func(ctx context.Context, d time.Duration) error
}

// Deps bundles Publisher dependencies.
type Deps struct {
	Targets  Targets
	Posts    Posts
	Jobs     Jobs
	Accounts Accounts
	Vault    Vault
	Media    MediaStore
	Metrics  Metrics
	Registry *provider.Registry
	Tx       port.TxRunner
	Audit    port.AuditRecorder
	Clock    port.Clock
	Log      *slog.Logger
	// OnOutcome (optional) is called after each settled attempt with
	// outcome published | failed | retry | needs_review (metrics hook).
	OnOutcome func(provider, outcome string)
	// Queue (optional) re-enqueues a task that fires more than EarlyWaitMax before its job's run_at.
	Queue Queue
	// Sleep (optional) waits d or until ctx ends; tests inject a fake-clock version.
	Sleep func(ctx context.Context, d time.Duration) error
}

// NewPublisher creates the publisher.
func NewPublisher(d Deps) *Publisher {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Sleep == nil {
		d.Sleep = sleepCtx
	}
	return &Publisher{queue: d.Queue, sleep: d.Sleep, targets: d.Targets, posts: d.Posts, jobs: d.Jobs, accounts: d.Accounts, vault: d.Vault,
		media: d.Media, metrics: d.Metrics, registry: d.Registry, tx: d.Tx, audit: d.Audit, clock: d.Clock, log: d.Log, observe: d.OnOutcome}
}

func (p *Publisher) outcome(r *run, outcome string) {
	if p.observe != nil && r.account != nil {
		p.observe(r.account.Provider, outcome)
	}
}

// run is the state of one execution after the begin transaction.
type run struct {
	target    *post.Target
	post      *post.Post
	account   *socialaccount.Account
	attempt   *post.Attempt
	lookup    bool // previous outcome unknown: look up before re-posting
	actor     actor.Actor
	payload   Payload
	retryInfo RetryInfo
}

// Run executes one publish job. It returns nil when the job is finished
// (success, permanent failure, or no-op) and a *RetryableError to retry.
func (p *Publisher) Run(ctx context.Context, pl Payload, ri RetryInfo) error {
	if proceed, err := p.holdUntilDue(ctx, pl); err != nil || !proceed {
		return err
	}
	r, err := p.begin(ctx, pl)
	if errors.Is(err, errInFlight) {
		return &RetryableError{Err: err}
	}
	if err != nil {
		return &RetryableError{Err: err} // DB/infra failure before any provider call: safe to retry
	}
	if r == nil {
		return nil
	}
	r.retryInfo = ri
	return p.execute(ctx, r)
}

// begin locks the target, applies idempotency checks and records a `started` attempt.
func (p *Publisher) begin(ctx context.Context, pl Payload) (*run, error) {
	var out *run
	err := p.tx.InTx(ctx, func(ctx context.Context) error {
		job, err := p.jobs.Get(ctx, pl.JobID)
		if errs.Is(err, errs.NotFound) || (err == nil && !job.Status.Active()) {
			return nil // cancelled/rescheduled/finished: late task is a no-op
		}
		if err != nil {
			return err
		}
		t, ok, err := p.targets.LockTarget(ctx, pl.TargetID)
		if errs.Is(err, errs.NotFound) {
			return nil // target removed (post edited/deleted)
		}
		if err != nil {
			return err
		}
		if !ok {
			return errInFlight
		}
		r, err := p.prepare(ctx, pl, t)
		out = r
		return err
	})
	return out, err
}

// prepare runs inside the begin tx with the target row locked.
func (p *Publisher) prepare(ctx context.Context, pl Payload, t *post.Target) (*run, error) {
	ps, err := p.posts.GetForUpdate(ctx, t.UserID, t.PostID)
	if errs.Is(err, errs.NotFound) {
		return nil, p.jobs.MarkDoneForTarget(ctx, t.ID) // post deleted
	}
	if err != nil {
		return nil, err
	}
	r := &run{target: t, post: ps, actor: actor.Scheduler(t.UserID), payload: pl}
	if t.Status == post.TargetPublished || t.Status == post.TargetCancelled || t.Status == post.TargetNeedsReview ||
		(ps.Status != post.StatusScheduled && ps.Status != post.StatusPublishing) {
		return nil, p.jobs.MarkDoneForTarget(ctx, t.ID)
	}
	if t.ExternalPostID != "" { // published earlier but status not committed
		return nil, p.markPublishedLocked(ctx, r, provider.PublishResult{ExternalID: t.ExternalPostID, URL: t.ExternalURL}, nil)
	}
	acc, err := p.accounts.Get(ctx, t.UserID, t.SocialAccountID)
	if err != nil {
		return nil, err
	}
	r.account = acc
	latest, err := p.targets.LatestAttempt(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	if proceed, err := p.resolvePrevious(ctx, r, latest); err != nil || !proceed {
		return nil, err
	}
	if ps.Status == post.StatusScheduled {
		ps.Status = post.StatusPublishing
		if err := p.posts.Update(ctx, ps); err != nil {
			return nil, err
		}
	}
	return r, p.startAttempt(ctx, r)
}

// resolvePrevious handles a previous attempt whose outcome is not known.
// It returns proceed=false when the run must stop (in flight / needs review).
func (p *Publisher) resolvePrevious(ctx context.Context, r *run, latest *post.Attempt) (bool, error) {
	if latest == nil {
		return true, nil
	}
	switch latest.Status {
	case post.AttemptStarted:
		if latest.StartedAt.After(p.clock.Now().Add(-InFlightWindow)) {
			return false, errInFlight
		}
		// The worker that started this attempt died: the provider may or may not have posted.
		latest.Status = post.AttemptUnknown
		latest.ErrorCode, latest.ErrorMessage = "OUTCOME_UNKNOWN", "worker stopped before recording the provider response"
		now := p.clock.Now()
		latest.FinishedAt = &now
		if err := p.targets.UpdateAttempt(ctx, latest); err != nil {
			return false, err
		}
		return p.decideUnknown(ctx, r)
	case post.AttemptUnknown:
		r.lookup = p.canLookup(r.account.Provider)
	}
	return true, nil
}

// decideUnknown: look up if possible, re-send only if the provider is idempotent, else needs_review.
func (p *Publisher) decideUnknown(ctx context.Context, r *run) (bool, error) {
	prov, err := p.registry.Get(r.account.Provider)
	if err == nil && prov.Capabilities().SafeToRetryAfterUnknown {
		return true, nil
	}
	if p.canLookup(r.account.Provider) {
		r.lookup = true
		return true, nil
	}
	return false, p.needsReviewLocked(ctx, r, nil)
}

func (p *Publisher) canLookup(name string) bool {
	prov, err := p.registry.Get(name)
	if err != nil {
		return false
	}
	_, ok := prov.(provider.Lookuper)
	return ok
}

func (p *Publisher) startAttempt(ctx context.Context, r *run) error {
	r.target.Status = post.TargetPublishing
	r.target.AttemptCount++
	if err := p.targets.UpdateTarget(ctx, r.target); err != nil {
		return err
	}
	r.attempt = &post.Attempt{
		PostTargetID: r.target.ID, AttemptNo: r.target.AttemptCount, StartedAt: p.clock.Now(),
		Status: post.AttemptStarted, ResponseMetadata: map[string]any{},
	}
	return p.targets.InsertAttempt(ctx, r.attempt)
}
