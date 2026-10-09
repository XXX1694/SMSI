package schedulertest

import (
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/scheduler"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Epoch is the default fake "now".
var Epoch = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// World is a ready-to-use scheduler environment: all fakes wired to one Store
// and seeded with one user, one active account on Provider, one scheduled post
// with a single pending target and an enqueued job due at Epoch.
type World struct {
	Store    *Store
	Clock    *Clock
	Tx       *Tx
	Queue    *Queue
	Registry *provider.Registry
	Provider provider.Provider

	UserID                      uuid.UUID
	AccountID, PostID, TargetID uuid.UUID
	JobID                       uuid.UUID
	// Observed collects OnOutcome callbacks as "provider:outcome".
	Observed []string
}

// NewWorld builds and seeds a World for prov (default: NewProvider("fake")).
func NewWorld(prov provider.Provider) *World {
	if prov == nil {
		prov = NewProvider("fake")
	}
	clk := &Clock{T: Epoch}
	s := NewStore(clk)
	w := &World{Store: s, Clock: clk, Tx: &Tx{S: s}, Queue: NewQueue(s), Registry: provider.NewRegistry(prov),
		Provider: prov, UserID: uuid.New(), AccountID: uuid.New(), PostID: uuid.New(), TargetID: uuid.New(), JobID: uuid.New()}
	now := clk.Now()
	s.Accounts[w.AccountID] = socialaccount.Account{ID: w.AccountID, UserID: w.UserID, Provider: prov.Name(),
		ProviderAccountID: "pa-1", Username: "acme", Status: socialaccount.StatusActive}
	s.Posts[w.PostID] = post.Post{ID: w.PostID, UserID: w.UserID, Status: post.StatusScheduled, ScheduledAt: &now}
	s.Targets[w.TargetID] = post.Target{ID: w.TargetID, PostID: w.PostID, UserID: w.UserID, SocialAccountID: w.AccountID,
		Platform: prov.Name(), Content: "hello", Status: post.TargetPending, IdempotencyKey: "idem-1", UpdatedAt: now}
	s.Jobs[w.JobID] = post.Job{ID: w.JobID, PostTargetID: w.TargetID, RunAt: now, Status: post.JobEnqueued, AsynqTaskID: "task-0"}
	return w
}

// Deps returns scheduler.Deps wired to the fakes. Queue and Sleep are set.
func (w *World) Deps() scheduler.Deps {
	s := w.Store
	return scheduler.Deps{
		Targets: Targets{s}, Posts: Posts{s}, Jobs: Jobs{s}, Accounts: Accounts{s}, Vault: Vault{s},
		Media: MediaStore{s}, Metrics: Metrics{s}, Registry: w.Registry, Tx: w.Tx, Audit: Audit{s}, Clock: w.Clock,
		Queue: w.Queue, Sleep: w.Clock.Sleep, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnOutcome: func(p, o string) { w.Observed = append(w.Observed, p+":"+o) },
	}
}

// Publisher builds a scheduler.Publisher over the fakes.
func (w *World) Publisher() *scheduler.Publisher { return scheduler.NewPublisher(w.Deps()) }

// Reconciler builds a scheduler.Reconciler over the fakes.
func (w *World) Reconciler() *scheduler.Reconciler {
	return scheduler.NewReconciler(w.Publisher(), w.Queue)
}

// Payload is the queue payload of the seeded job.
func (w *World) Payload() scheduler.Payload {
	return scheduler.Payload{TargetID: w.TargetID, JobID: w.JobID}
}

// Target returns the seeded target as stored now.
func (w *World) Target() post.Target { return w.Store.Target(w.TargetID) }

// Post returns the seeded post as stored now.
func (w *World) Post() post.Post { return w.Store.Post(w.PostID) }

// SetTarget mutates the stored seeded target.
func (w *World) SetTarget(fn func(*post.Target)) {
	t := w.Store.Target(w.TargetID)
	fn(&t)
	w.Store.Targets[w.TargetID] = t
}

// SetPost mutates the stored seeded post.
func (w *World) SetPost(fn func(*post.Post)) {
	p := w.Store.Post(w.PostID)
	fn(&p)
	w.Store.Posts[w.PostID] = p
}

// SetAccount mutates the stored seeded account.
func (w *World) SetAccount(fn func(*socialaccount.Account)) {
	a := w.Store.Accounts[w.AccountID]
	fn(&a)
	w.Store.Accounts[w.AccountID] = a
}

// SetJob mutates the stored seeded job.
func (w *World) SetJob(fn func(*post.Job)) {
	j := w.Store.Jobs[w.JobID]
	fn(&j)
	w.Store.Jobs[w.JobID] = j
}

// AddAttempt inserts an attempt for the seeded target.
func (w *World) AddAttempt(no int, st post.AttemptStatus, startedAt time.Time) post.Attempt {
	a := post.Attempt{ID: uuid.New(), PostTargetID: w.TargetID, AttemptNo: no, StartedAt: startedAt, Status: st}
	w.Store.Attempts = append(w.Store.Attempts, a)
	return a
}

// AddSiblingTarget adds a second target to the seeded post with the given status.
func (w *World) AddSiblingTarget(st post.TargetStatus) uuid.UUID {
	id := uuid.New()
	w.Store.Targets[id] = post.Target{ID: id, PostID: w.PostID, UserID: w.UserID, SocialAccountID: w.AccountID,
		Platform: w.Provider.Name(), Status: st, UpdatedAt: w.Clock.Now()}
	return id
}
