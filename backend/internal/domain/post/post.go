package post

import (
	"time"

	"github.com/google/uuid"
)

// CreatedBy identifies who created a post.
type CreatedBy string

const (
	CreatedByUser   CreatedBy = "user"
	CreatedByAPIKey CreatedBy = "api_key"
)

// Post is the aggregate root.
type Post struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Title       string
	Content     string
	Status      Status
	ScheduledAt *time.Time
	PublishedAt *time.Time
	// QuotaCountedAt is set once, when the post is first scheduled or published; it is what the monthly quota counts.
	QuotaCountedAt *time.Time
	CreatedBy      CreatedBy
	CreatedByRef   string
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time

	Targets  []Target
	MediaIDs []uuid.UUID
}

// Target is one post on one social account.
type Target struct {
	ID              uuid.UUID
	PostID          uuid.UUID
	UserID          uuid.UUID
	SocialAccountID uuid.UUID
	Platform        string
	Content         string
	Status          TargetStatus
	ExternalPostID  string
	ExternalURL     string
	PublishedAt     *time.Time
	ErrorCode       string
	ErrorMessage    string
	IdempotencyKey  string
	AttemptCount    int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Attempt records a single provider call.
type Attempt struct {
	ID               uuid.UUID
	PostTargetID     uuid.UUID
	AttemptNo        int
	StartedAt        time.Time
	FinishedAt       *time.Time
	Status           AttemptStatus
	ErrorCode        string
	ErrorMessage     string
	ResponseMetadata map[string]any
}

// Job is a scheduled_jobs row.
type Job struct {
	ID           uuid.UUID
	PostTargetID uuid.UUID
	RunAt        time.Time
	AsynqTaskID  string
	Status       JobStatus
	UpdatedAt    time.Time
}

// TargetStatuses returns the statuses of all targets.
func (p *Post) TargetStatuses() []TargetStatus {
	out := make([]TargetStatus, len(p.Targets))
	for i, t := range p.Targets {
		out[i] = t.Status
	}
	return out
}
