package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/dataexport"
)

// Exports implements account.ExportRepo. Indexes: data_exports_user_idx (user, newest first),
// data_exports_active_uniq (one pending/running per user), data_exports_sweep_idx (status, updated_at).
type Exports struct{ db *DB }

// NewExports creates the repo.
func NewExports(db *DB) *Exports { return &Exports{db: db} }

const exportCols = `id, user_id, status, storage_key, size_bytes, error_code, expires_at, created_at, updated_at`

func scanExport(row interface{ Scan(...any) error }) (*dataexport.Export, error) {
	var e dataexport.Export
	if err := row.Scan(&e.ID, &e.UserID, &e.Status, &e.StorageKey, &e.SizeBytes, &e.ErrorCode, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

// Create inserts a pending export; a second active one hits the partial unique index and is CONFLICT.
func (r *Exports) Create(ctx context.Context, userID uuid.UUID) (*dataexport.Export, error) {
	e, err := scanExport(r.db.q(ctx).QueryRow(ctx, `INSERT INTO data_exports (user_id) VALUES ($1) RETURNING `+exportCols, userID))
	return e, mapErr(err, "export")
}

// Get returns one of the user's exports.
func (r *Exports) Get(ctx context.Context, userID, id uuid.UUID) (*dataexport.Export, error) {
	e, err := scanExport(r.db.q(ctx).QueryRow(ctx, `SELECT `+exportCols+` FROM data_exports WHERE id = $1 AND user_id = $2`, id, userID))
	return e, mapErr(err, "export")
}

// List returns the user's exports, newest first.
func (r *Exports) List(ctx context.Context, userID uuid.UUID, limit int) ([]dataexport.Export, error) {
	return r.collect(ctx, `SELECT `+exportCols+` FROM data_exports WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit)
}

// ExpireReady marks the user's ready exports expired.
func (r *Exports) ExpireReady(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE data_exports SET status = 'expired' WHERE user_id = $1 AND status = 'ready'`, userID)
	return mapErr(err, "export")
}

// Claim moves a pending export to running (system: keyed by id, called by the worker).
func (r *Exports) Claim(ctx context.Context, id uuid.UUID) (*dataexport.Export, error) {
	e, err := scanExport(r.db.q(ctx).QueryRow(ctx,
		`UPDATE data_exports SET status = 'running' WHERE id = $1 AND status = 'pending' RETURNING `+exportCols, id))
	return e, mapErr(err, "export")
}

// MarkReady records the finished archive (system).
func (r *Exports) MarkReady(ctx context.Context, id uuid.UUID, key string, size int64, expiresAt time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx,
		`UPDATE data_exports SET status = 'ready', storage_key = $2, size_bytes = $3, expires_at = $4, error_code = '' WHERE id = $1`,
		id, key, size, expiresAt)
	return mustAffect(tag, err, "export")
}

// MarkFailed records a failure; only an unfinished export can fail (system).
func (r *Exports) MarkFailed(ctx context.Context, id uuid.UUID, code string) error {
	_, err := r.db.q(ctx).Exec(ctx,
		`UPDATE data_exports SET status = 'failed', error_code = $2 WHERE id = $1 AND status IN ('pending','running')`, id, code)
	return mapErr(err, "export")
}

// Sweepable lists what the hourly sweep must act on (system).
func (r *Exports) Sweepable(ctx context.Context, now, pendingBefore, runningBefore time.Time, limit int) ([]dataexport.Export, error) {
	return r.collect(ctx, `SELECT `+exportCols+` FROM data_exports
		WHERE (status = 'ready' AND expires_at <= $1)
		   OR (status = 'expired' AND storage_key <> '')
		   OR (status = 'pending' AND updated_at < $2)
		   OR (status = 'running' AND updated_at < $3)
		ORDER BY updated_at LIMIT $4`, now, pendingBefore, runningBefore, limit)
}

// ClearObject forgets the key of a deleted object and marks the export expired (system).
func (r *Exports) ClearObject(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.q(ctx).Exec(ctx, `UPDATE data_exports SET storage_key = '', status = 'expired' WHERE id = $1 AND status IN ('ready','expired')`, id)
	return mapErr(err, "export")
}

func (r *Exports) collect(ctx context.Context, sql string, args ...any) ([]dataexport.Export, error) {
	rows, err := r.db.q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, mapErr(err, "export")
	}
	defer rows.Close()
	out := []dataexport.Export{}
	for rows.Next() {
		e, err := scanExport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}
