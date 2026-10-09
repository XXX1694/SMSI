// Package account implements the owner's rights over their own data: export (and, later, deletion).
package account

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/dataexport"
)

// ExportRepo persists data_exports rows. Reads and writes that take a user id are tenant-scoped; the worker-only
// methods (Claim and below) are keyed by the export id the queue carries and are never reachable from HTTP.
type ExportRepo interface {
	// Create inserts a pending export. A second active export of the user is CONFLICT (partial unique index).
	Create(ctx context.Context, userID uuid.UUID) (*dataexport.Export, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*dataexport.Export, error)
	// List returns the user's newest exports first.
	List(ctx context.Context, userID uuid.UUID, limit int) ([]dataexport.Export, error)
	// ExpireReady marks the user's ready exports expired (their objects are deleted by the sweep).
	ExpireReady(ctx context.Context, userID uuid.UUID) error

	// Claim moves a pending export to running and returns it; any other state is NOT_FOUND.
	Claim(ctx context.Context, id uuid.UUID) (*dataexport.Export, error)
	MarkReady(ctx context.Context, id uuid.UUID, key string, size int64, expiresAt time.Time) error
	MarkFailed(ctx context.Context, id uuid.UUID, code string) error
	// Sweepable lists exports the sweep must act on at now: ready past expiry, expired with an object left, pending
	// since before pendingBefore, running since before runningBefore.
	Sweepable(ctx context.Context, now, pendingBefore, runningBefore time.Time, limit int) ([]dataexport.Export, error)
	// ClearObject forgets the stored key after the object is deleted and marks a ready export expired.
	ClearObject(ctx context.Context, id uuid.UUID) error
}

// Dataset names one JSON file of the archive.
type Dataset string

// The datasets of the archive, in the order they are written.
const (
	DatasetProfile        Dataset = "profile"
	DatasetSocialAccounts Dataset = "social_accounts"
	DatasetPosts          Dataset = "posts"
	DatasetAPIKeys        Dataset = "api_keys"
	DatasetMCP            Dataset = "mcp_connections"
	DatasetApprovals      Dataset = "approvals"
	DatasetAuditLogs      Dataset = "audit_logs"
)

// Row is one record of a dataset, already rendered as JSON by the database from an explicit column list (so a column
// added later is not exported by accident, and no credential column can be).
type Row struct {
	ID   uuid.UUID
	JSON []byte
}

// MediaFile is one stored upload.
type MediaFile struct {
	ID           uuid.UUID `json:"id"`
	Kind         string    `json:"kind"`
	MimeType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	OriginalName string    `json:"original_name"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"created_at"`
	// File is the path inside the archive. StorageKey is internal and never written out.
	File       string `json:"file"`
	StorageKey string `json:"-"`
}

// ExportData reads a user's data in keyset batches (rows with id greater than after, ordered by id), so the archive
// is built with memory bounded by the batch size however much the user owns. Every query filters by user.
type ExportData interface {
	Rows(ctx context.Context, ds Dataset, userID, after uuid.UUID, limit int) ([]Row, error)
	MediaFiles(ctx context.Context, userID, after uuid.UUID, limit int) ([]MediaFile, error)
}

// ObjectStore is the part of the object storage the export needs.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// ExportQueue hands an export to the worker. Enqueueing the same id twice must not build twice (Claim guards that too).
type ExportQueue interface {
	EnqueueExport(ctx context.Context, exportID uuid.UUID) error
}
