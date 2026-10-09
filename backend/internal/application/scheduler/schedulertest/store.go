// Package schedulertest provides in-memory fakes of the scheduler's ports
// (repositories, queue, clock, tx runner, audit, vault, provider) for fast,
// DB-free unit tests. It is test support only; production code must not import it.
//
// All repositories share one Store. The fakes honour the semantics the
// scheduler relies on: LockTarget returns (nil,false,nil) for a held lock and
// errs.NotFound for a missing row, rows are returned as copies, and Tx rolls
// the whole Store back when the transaction function fails.
package schedulertest

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// AuditEntry is one recorded audit event.
type AuditEntry struct {
	Action, ResourceType, ResourceID string
	Meta                             map[string]any
}

// MetricCall is one recorded analytics increment.
type MetricCall struct {
	Metric string
	Value  int64
}

// Store is the shared in-memory database behind every repository fake.
type Store struct {
	Clock *Clock

	Targets  map[uuid.UUID]post.Target
	Posts    map[uuid.UUID]post.Post
	Jobs     map[uuid.UUID]post.Job
	Accounts map[uuid.UUID]socialaccount.Account
	Attempts []post.Attempt
	Creds    map[uuid.UUID]socialaccount.Credentials
	Media    map[uuid.UUID]media.Media
	// Saved holds tokens persisted through Vault.Save, in order.
	Saved   []provider.Token
	Audit   []AuditEntry
	Metrics []MetricCall

	// Locked simulates targets whose row lock is held by another transaction.
	Locked map[uuid.UUID]bool
	// Errors injects a failure into the named operation, e.g. "Targets.UpdateTarget".
	// The error is returned on every call until removed.
	Errors map[string]error
}

// NewStore creates an empty store using clock for UpdatedAt stamps.
func NewStore(clock *Clock) *Store {
	return &Store{
		Clock:   clock,
		Targets: map[uuid.UUID]post.Target{}, Posts: map[uuid.UUID]post.Post{},
		Jobs: map[uuid.UUID]post.Job{}, Accounts: map[uuid.UUID]socialaccount.Account{},
		Creds: map[uuid.UUID]socialaccount.Credentials{}, Media: map[uuid.UUID]media.Media{},
		Locked: map[uuid.UUID]bool{}, Errors: map[string]error{},
	}
}

func (s *Store) fail(op string) error { return s.Errors[op] }

// snapshot is a rollback point for Tx.
type snapshot struct {
	targets  map[uuid.UUID]post.Target
	posts    map[uuid.UUID]post.Post
	jobs     map[uuid.UUID]post.Job
	accounts map[uuid.UUID]socialaccount.Account
	attempts []post.Attempt
	saved    []provider.Token
	audit    []AuditEntry
	metrics  []MetricCall
}

func clone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Store) snapshot() snapshot {
	return snapshot{clone(s.Targets), clone(s.Posts), clone(s.Jobs), clone(s.Accounts),
		append([]post.Attempt(nil), s.Attempts...), append([]provider.Token(nil), s.Saved...),
		append([]AuditEntry(nil), s.Audit...), append([]MetricCall(nil), s.Metrics...)}
}

func (s *Store) restore(sn snapshot) {
	s.Targets, s.Posts, s.Jobs, s.Accounts = sn.targets, sn.posts, sn.jobs, sn.accounts
	s.Attempts, s.Saved, s.Audit, s.Metrics = sn.attempts, sn.saved, sn.audit, sn.metrics
}

// Target returns a copy of a stored target; it panics when absent (test bug).
func (s *Store) Target(id uuid.UUID) post.Target {
	t, ok := s.Targets[id]
	if !ok {
		panic(fmt.Sprintf("schedulertest: target %s not found", id))
	}
	return t
}

// Post returns a copy of a stored post; it panics when absent.
func (s *Store) Post(id uuid.UUID) post.Post {
	p, ok := s.Posts[id]
	if !ok {
		panic(fmt.Sprintf("schedulertest: post %s not found", id))
	}
	return p
}

// AttemptsFor returns the attempts of a target ordered by attempt number.
func (s *Store) AttemptsFor(targetID uuid.UUID) []post.Attempt {
	var out []post.Attempt
	for _, a := range s.Attempts {
		if a.PostTargetID == targetID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AttemptNo < out[j].AttemptNo })
	return out
}

// AuditActions lists recorded audit actions in order.
func (s *Store) AuditActions() []string {
	out := make([]string, len(s.Audit))
	for i, a := range s.Audit {
		out[i] = a.Action
	}
	return out
}

// JobForTarget returns the most recently created job of a target, if any.
func (s *Store) JobForTarget(targetID uuid.UUID) (post.Job, bool) {
	var best post.Job
	var found bool
	for _, j := range s.Jobs {
		if j.PostTargetID == targetID && (!found || j.RunAt.After(best.RunAt)) {
			best, found = j, true
		}
	}
	return best, found
}

func notFound(what string) error { return errs.NotFoundf(what) }
