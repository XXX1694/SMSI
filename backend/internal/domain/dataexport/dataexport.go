// Package dataexport defines the account data export (a ZIP built by the worker) and its lifecycle.
package dataexport

import (
	"time"

	"github.com/google/uuid"
)

// Status of an export. pending -> running -> ready | failed; a ready export becomes expired.
type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusReady   Status = "ready"
	StatusFailed  Status = "failed"
	StatusExpired Status = "expired"
)

// Error codes stored with a failed export. They are short and carry no internals.
const (
	ErrBuildFailed  = "build_failed"
	ErrInterrupted  = "interrupted"
	ErrTimedOut     = "timed_out"
	ErrQueueFailed  = "queue_unavailable"
	ErrStoreFailure = "storage_failed"
)

// Export is one export request of a user.
type Export struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Status     Status
	StorageKey string
	SizeBytes  int64
	ErrorCode  string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Active reports whether the worker still has to build or is building the export.
func (e *Export) Active() bool { return e.Status == StatusPending || e.Status == StatusRunning }

// Downloadable reports whether the ZIP can be fetched at now: ready and not past its expiry.
func (e *Export) Downloadable(now time.Time) bool {
	return e.Status == StatusReady && e.ExpiresAt != nil && now.Before(*e.ExpiresAt)
}

// EffectiveStatus is the status to show at now: a ready export past its expiry is reported as expired even before
// the sweep has deleted its object.
func (e *Export) EffectiveStatus(now time.Time) Status {
	if e.Status == StatusReady && !e.Downloadable(now) {
		return StatusExpired
	}
	return e.Status
}

// ObjectKey is where the ZIP of an export lives; it sits under the owner's prefix like the user's media.
func ObjectKey(userID, exportID uuid.UUID) string {
	return "users/" + userID.String() + "/exports/" + exportID.String() + ".zip"
}
