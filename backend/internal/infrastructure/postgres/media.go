package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/media"
)

// Media implements media.Repo.
type Media struct{ db *DB }

// NewMedia creates the repo.
func NewMedia(db *DB) *Media { return &Media{db: db} }

const mediaCols = `id, user_id, kind, mime_type, size_bytes, storage_key, original_name, width, height, status, sha256, created_at`

func scanMedia(row interface{ Scan(...any) error }) (*media.Media, error) {
	var m media.Media
	if err := row.Scan(&m.ID, &m.UserID, &m.Kind, &m.MimeType, &m.SizeBytes, &m.StorageKey, &m.OriginalName,
		&m.Width, &m.Height, &m.Status, &m.SHA256, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

// Create inserts media metadata.
func (r *Media) Create(ctx context.Context, m *media.Media) error {
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO media (id, user_id, kind, mime_type, size_bytes, storage_key, original_name, width, height, status, sha256)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING created_at`,
		m.ID, m.UserID, m.Kind, m.MimeType, m.SizeBytes, m.StorageKey, m.OriginalName, m.Width, m.Height, m.Status, m.SHA256).Scan(&m.CreatedAt), "media")
}

// Get returns one media of the user.
func (r *Media) Get(ctx context.Context, userID, id uuid.UUID) (*media.Media, error) {
	m, err := scanMedia(r.db.q(ctx).QueryRow(ctx, `SELECT `+mediaCols+` FROM media WHERE id = $1 AND user_id = $2`, id, userID))
	return m, mapErr(err, "media")
}

// GetMany returns the user's media among ids (missing ids are omitted).
func (r *Media) GetMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]media.Media, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+mediaCols+` FROM media WHERE user_id = $1 AND id = ANY($2)`, userID, ids)
	if err != nil {
		return nil, mapErr(err, "media")
	}
	return collectMedia(rows)
}

func collectMedia(rows interface {
	Next() bool
	Scan(...any) error
	Close()
	Err() error
}) ([]media.Media, error) {
	defer rows.Close()
	out := []media.Media{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// List pages the user's media newest first.
func (r *Media) List(ctx context.Context, userID uuid.UUID, page port.Page) ([]media.Media, error) {
	if page.Cursor != nil {
		rows, err := r.db.q(ctx).Query(ctx, `SELECT `+mediaCols+` FROM media WHERE user_id = $1 AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC LIMIT $4`, userID, page.Cursor.At, page.Cursor.ID, page.Limit)
		if err != nil {
			return nil, err
		}
		return collectMedia(rows)
	}
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+mediaCols+` FROM media WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, page.Limit)
	if err != nil {
		return nil, err
	}
	return collectMedia(rows)
}

// Delete removes the user's media row. Links from soft-deleted posts are
// dropped first (the FK is RESTRICT); callers must have checked IsReferenced.
func (r *Media) Delete(ctx context.Context, userID, id uuid.UUID) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		if _, err := r.db.q(ctx).Exec(ctx, `DELETE FROM post_media pm USING posts p
			WHERE pm.post_id = p.id AND pm.media_id = $1 AND p.user_id = $2 AND p.deleted_at IS NOT NULL`, id, userID); err != nil {
			return mapErr(err, "media")
		}
		tag, err := r.db.q(ctx).Exec(ctx, `DELETE FROM media WHERE id = $1 AND user_id = $2`, id, userID)
		return mustAffect(tag, err, "media")
	})
}

// IsReferenced reports whether any non-deleted post uses the media.
func (r *Media) IsReferenced(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	var used bool
	err := r.db.q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM post_media pm JOIN posts p ON p.id = pm.post_id
		WHERE pm.media_id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL)`, id, userID).Scan(&used)
	return used, err
}
