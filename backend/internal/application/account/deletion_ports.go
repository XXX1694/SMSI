package account

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/user"
)

// Table names the purge deletes from in small batches (a fixed set, never built from input).
type Table string

// The tables with many rows per user. Everything else goes with the user row (ON DELETE CASCADE).
const (
	TablePosts     Table = "posts" // cascades to post_targets, post_media, scheduled_jobs, publication_attempts
	TableAuditLogs Table = "audit_logs"
	TableAnalytics Table = "analytics"
	TableApprovals Table = "action_approvals"
)

// Counts is what a purge removed, kept in account_deletions.counts. It holds numbers only: no PII.
type Counts struct {
	Posts          int64 `json:"posts"`
	Media          int64 `json:"media"`
	SocialAccounts int64 `json:"social_accounts"`
	APIKeys        int64 `json:"api_keys"`
}

// MediaObject is one uploaded file of the user being purged.
type MediaObject struct {
	ID  uuid.UUID
	Key string
}

// DeletionRepo persists the deletion schedule and performs the purge. Everything is keyed by the user id; the HTTP
// layer never passes one in, it always comes from the session actor.
type DeletionRepo interface {
	// Schedule sets users.deletion_scheduled_at and records the request; CONFLICT when one is already scheduled.
	Schedule(ctx context.Context, userID uuid.UUID, requestedAt, purgeAt time.Time) error
	// Cancel clears the schedule and the request record; CONFLICT when none is scheduled.
	Cancel(ctx context.Context, userID uuid.UUID) error

	// Due lists users to purge at now: grace over, or already claimed by a purge that did not finish.
	Due(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	// Claim marks a due account deleted (it can no longer sign in) and returns its email. ok is false when the
	// account is gone, was cancelled or is not due. Claiming an already claimed account succeeds, so a retry resumes.
	Claim(ctx context.Context, userID uuid.UUID, now time.Time) (email string, ok bool, err error)
	// Busy says why the purge must wait ("" when it can go on): a post is being published or an export is being built.
	Busy(ctx context.Context, userID uuid.UUID) (string, error)
	Counts(ctx context.Context, userID uuid.UUID) (Counts, error)
	// SaveCounts stores the counts once; later calls are ignored (a retry must not overwrite the full counts).
	SaveCounts(ctx context.Context, userID uuid.UUID, c Counts) error
	DeleteBatch(ctx context.Context, userID uuid.UUID, t Table, limit int) (int64, error)
	MediaBatch(ctx context.Context, userID uuid.UUID, limit int) ([]MediaObject, error)
	DeleteMedia(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error
	ExportKeys(ctx context.Context, userID uuid.UUID) ([]string, error)
	// DeleteUser removes the user row (and with it everything left) and stamps purged_at, in one transaction.
	DeleteUser(ctx context.Context, userID uuid.UUID, at time.Time) error
}

// UserReader loads the account for re-authentication.
type UserReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// Passwords verifies a password against a stored hash.
type Passwords interface {
	Verify(ctx context.Context, password, encoded string) (bool, error)
}

// Revoker ends the owner's access: sessions and API keys.
type Revoker interface {
	DeleteAllForUser(ctx context.Context, userID, except uuid.UUID) (int64, error)
}

// KeyRevoker revokes API keys and MCP connections.
type KeyRevoker interface {
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) (int64, error)
}

// PostStopper takes every scheduled post of the user back to draft so nothing is published during the grace period.
type PostStopper interface {
	UnscheduleAll(ctx context.Context, userID uuid.UUID) (int, error)
}

// PurgeQueue hands a purge to the worker.
type PurgeQueue interface {
	EnqueuePurge(ctx context.Context, userID uuid.UUID) error
}
