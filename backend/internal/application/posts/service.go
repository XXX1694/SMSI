package posts

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/post"
)

// Service implements post use cases.
type Service struct {
	repo     Repo
	jobs     Jobs
	queue    Enqueuer
	accounts Accounts
	media    Media
	registry *provider.Registry
	tx       port.TxRunner
	audit    port.AuditRecorder
	clock    port.Clock
	gate     port.ApprovalGate
	log      *slog.Logger
	// minAgentLead is how far ahead an API key may schedule without approval.
	minAgentLead time.Duration
}

// Deps bundles dependencies.
type Deps struct {
	Repo     Repo
	Jobs     Jobs
	Queue    Enqueuer
	Accounts Accounts
	Media    Media
	Registry *provider.Registry
	Tx       port.TxRunner
	Audit    port.AuditRecorder
	Clock    port.Clock
	// Gate asks the owner to approve dangerous actions of API keys; nil refuses them (fail closed).
	Gate port.ApprovalGate
	// MinAgentLead overrides DefaultMinAgentLead; negative disables the rule.
	MinAgentLead time.Duration
	Log          *slog.Logger
}

// NewService creates the posts service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Gate == nil {
		d.Gate = port.FailClosedGate{}
	}
	lead := d.MinAgentLead
	if lead == 0 {
		lead = DefaultMinAgentLead
	}
	return &Service{repo: d.Repo, jobs: d.Jobs, queue: d.Queue, accounts: d.Accounts, media: d.Media,
		registry: d.Registry, tx: d.Tx, audit: d.Audit, clock: d.Clock, gate: d.Gate, minAgentLead: lead, log: d.Log}
}

// Detail is a post with its publication attempts.
type Detail struct {
	*post.Post
	Attempts []post.Attempt
}

// Get returns a post with targets, media ids and attempts.
func (s *Service) Get(ctx context.Context, a actor.Actor, id uuid.UUID) (*Detail, error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return nil, err
	}
	p, err := s.repo.Get(ctx, a.UserID, id)
	if err != nil {
		return nil, err
	}
	attempts, err := s.repo.Attempts(ctx, a.UserID, id)
	if err != nil {
		return nil, err
	}
	return &Detail{Post: p, Attempts: attempts}, nil
}

// Status returns a post with targets (lightweight polling endpoint).
func (s *Service) Status(ctx context.Context, a actor.Actor, id uuid.UUID) (*post.Post, error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, a.UserID, id)
}

// List returns the tenant's posts, newest first.
func (s *Service) List(ctx context.Context, a actor.Actor, f ListFilter, page port.Page) (port.Result[post.Post], error) {
	if err := a.Require(apikey.PostsRead); err != nil {
		return port.Result[post.Post]{}, err
	}
	if f.Status != "" && !f.Status.Valid() {
		return port.Result[post.Post]{}, errInvalidStatus(f.Status)
	}
	items, err := s.repo.List(ctx, a.UserID, f, port.Page{Limit: page.Limit + 1, Cursor: page.Cursor})
	if err != nil {
		return port.Result[post.Post]{}, err
	}
	return port.Paginate(items, page.Limit, func(p post.Post) port.Cursor { return port.Cursor{At: p.CreatedAt, ID: p.ID} }), nil
}

func createdBy(a actor.Actor) (post.CreatedBy, string) {
	if a.Type == actor.TypeAPIKey {
		return post.CreatedByAPIKey, a.APIKeyID.String()
	}
	return post.CreatedByUser, a.UserID.String()
}
