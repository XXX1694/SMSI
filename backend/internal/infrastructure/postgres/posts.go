package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/application/posts"
	"github.com/socialos/backend/internal/domain/post"
)

// Posts implements posts.Repo and the post side of scheduler.Posts.
type Posts struct{ db *DB }

// NewPosts creates the repo.
func NewPosts(db *DB) *Posts { return &Posts{db: db} }

const postCols = `p.id, p.user_id, p.title, p.content, p.status, p.scheduled_at, p.published_at, p.quota_counted_at, p.created_by, p.created_by_ref, p.deleted_at, p.created_at, p.updated_at`

func scanPost(row interface{ Scan(...any) error }) (*post.Post, error) {
	var p post.Post
	if err := row.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.Status, &p.ScheduledAt, &p.PublishedAt, &p.QuotaCountedAt,
		&p.CreatedBy, &p.CreatedByRef, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// Create inserts a post with its targets and media links.
func (r *Posts) Create(ctx context.Context, p *post.Post) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		err := r.db.q(ctx).QueryRow(ctx, `INSERT INTO posts (id, user_id, title, content, status, scheduled_at, created_by, created_by_ref)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at, updated_at`,
			p.ID, p.UserID, p.Title, p.Content, p.Status, p.ScheduledAt, p.CreatedBy, p.CreatedByRef).Scan(&p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return mapErr(err, "post")
		}
		for i := range p.Targets {
			if err := r.insertTarget(ctx, &p.Targets[i]); err != nil {
				return err
			}
		}
		return r.ReplaceMedia(ctx, p.UserID, p.ID, p.MediaIDs)
	})
}

func (r *Posts) insertTarget(ctx context.Context, t *post.Target) error {
	err := r.db.q(ctx).QueryRow(ctx, `INSERT INTO post_targets (id, post_id, user_id, social_account_id, platform, content, status, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at, updated_at`,
		t.ID, t.PostID, t.UserID, t.SocialAccountID, t.Platform, t.Content, t.Status, t.IdempotencyKey).Scan(&t.CreatedAt, &t.UpdatedAt)
	return mapErr(err, "post target")
}

// Get returns a non-deleted post of the user with targets and media ids.
func (r *Posts) Get(ctx context.Context, userID, id uuid.UUID) (*post.Post, error) {
	return r.get(ctx, userID, id, "")
}

// GetForUpdate is Get with a row lock on the post.
func (r *Posts) GetForUpdate(ctx context.Context, userID, id uuid.UUID) (*post.Post, error) {
	return r.get(ctx, userID, id, " FOR UPDATE")
}

func (r *Posts) get(ctx context.Context, userID, id uuid.UUID, lock string) (*post.Post, error) {
	p, err := scanPost(r.db.q(ctx).QueryRow(ctx, `SELECT `+postCols+` FROM posts p
		WHERE p.id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL`+lock, id, userID))
	if err != nil {
		return nil, mapErr(err, "post")
	}
	if err := r.hydrate(ctx, userID, []*post.Post{p}); err != nil {
		return nil, err
	}
	return p, nil
}

// List returns posts newest first (keyset on created_at, id).
func (r *Posts) List(ctx context.Context, userID uuid.UUID, f posts.ListFilter, page port.Page) ([]post.Post, error) {
	where := []string{"p.user_id = $1", "p.deleted_at IS NULL"}
	args := []any{userID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Status != "" {
		add("p.status = $%d", f.Status)
	}
	if f.From != nil {
		add("COALESCE(p.scheduled_at, p.published_at, p.created_at) >= $%d", *f.From)
	}
	if f.To != nil {
		add("COALESCE(p.scheduled_at, p.published_at, p.created_at) < $%d", *f.To)
	}
	if page.Cursor != nil {
		args = append(args, page.Cursor.At, page.Cursor.ID)
		where = append(where, fmt.Sprintf("(p.created_at, p.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, page.Limit)
	sql := `SELECT ` + postCols + ` FROM posts p WHERE ` + strings.Join(where, " AND ") +
		fmt.Sprintf(` ORDER BY p.created_at DESC, p.id DESC LIMIT $%d`, len(args))
	return r.queryPosts(ctx, userID, sql, args...)
}

func (r *Posts) queryPosts(ctx context.Context, userID uuid.UUID, sql string, args ...any) ([]post.Post, error) {
	rows, err := r.db.q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, mapErr(err, "post")
	}
	var ptrs []*post.Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		ptrs = append(ptrs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.hydrate(ctx, userID, ptrs); err != nil {
		return nil, err
	}
	out := make([]post.Post, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out, nil
}

// Update writes mutable post fields.
func (r *Posts) Update(ctx context.Context, p *post.Post) error {
	err := r.db.q(ctx).QueryRow(ctx, `UPDATE posts SET title = $3, content = $4, status = $5, scheduled_at = $6, published_at = $7, quota_counted_at = COALESCE($8, quota_counted_at)
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL RETURNING updated_at`,
		p.ID, p.UserID, p.Title, p.Content, p.Status, p.ScheduledAt, p.PublishedAt, p.QuotaCountedAt).Scan(&p.UpdatedAt)
	return mapErr(err, "post")
}

// SoftDelete marks a post deleted.
func (r *Posts) SoftDelete(ctx context.Context, userID, id uuid.UUID, at time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE posts SET deleted_at = $3 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID, at)
	return mustAffect(tag, err, "post")
}

// ReplaceMedia rewrites post_media in order, verifying media ownership.
func (r *Posts) ReplaceMedia(ctx context.Context, userID, postID uuid.UUID, mediaIDs []uuid.UUID) error {
	if _, err := r.db.q(ctx).Exec(ctx, `DELETE FROM post_media pm USING posts p
		WHERE pm.post_id = p.id AND p.id = $1 AND p.user_id = $2`, postID, userID); err != nil {
		return mapErr(err, "post media")
	}
	for i, mid := range mediaIDs {
		tag, err := r.db.q(ctx).Exec(ctx, `INSERT INTO post_media (post_id, media_id, position)
			SELECT $1, m.id, $4 FROM media m WHERE m.id = $2 AND m.user_id = $3`, postID, mid, userID, i)
		if err := mustAffect(tag, err, "media"); err != nil {
			return err
		}
	}
	return nil
}
