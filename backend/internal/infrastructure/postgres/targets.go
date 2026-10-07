package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/socialos/backend/internal/domain/post"
)

const targetCols = `t.id, t.post_id, t.user_id, t.social_account_id, t.platform, t.content, t.status, t.external_post_id,
	t.external_url, t.published_at, t.error_code, t.error_message, t.idempotency_key, t.attempt_count, t.created_at, t.updated_at`

func scanTarget(row interface{ Scan(...any) error }) (*post.Target, error) {
	var t post.Target
	if err := row.Scan(&t.ID, &t.PostID, &t.UserID, &t.SocialAccountID, &t.Platform, &t.Content, &t.Status, &t.ExternalPostID,
		&t.ExternalURL, &t.PublishedAt, &t.ErrorCode, &t.ErrorMessage, &t.IdempotencyKey, &t.AttemptCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// hydrate loads targets and media ids for posts (all of the same user).
func (r *Posts) hydrate(ctx context.Context, userID uuid.UUID, ps []*post.Post) error {
	if len(ps) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(ps))
	byID := map[uuid.UUID]*post.Post{}
	for i, p := range ps {
		ids[i] = p.ID
		byID[p.ID] = p
		p.Targets, p.MediaIDs = []post.Target{}, []uuid.UUID{}
	}
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+targetCols+` FROM post_targets t
		WHERE t.post_id = ANY($1) AND t.user_id = $2 ORDER BY t.created_at, t.id`, ids, userID)
	if err != nil {
		return mapErr(err, "post target")
	}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			rows.Close()
			return err
		}
		byID[t.PostID].Targets = append(byID[t.PostID].Targets, *t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	mrows, err := r.db.q(ctx).Query(ctx, `SELECT pm.post_id, pm.media_id FROM post_media pm JOIN media m ON m.id = pm.media_id
		WHERE pm.post_id = ANY($1) AND m.user_id = $2 ORDER BY pm.position`, ids, userID)
	if err != nil {
		return mapErr(err, "post media")
	}
	defer mrows.Close()
	for mrows.Next() {
		var pid, mid uuid.UUID
		if err := mrows.Scan(&pid, &mid); err != nil {
			return err
		}
		byID[pid].MediaIDs = append(byID[pid].MediaIDs, mid)
	}
	return mrows.Err()
}

// ReplaceTargets upserts the post's targets and removes targets no longer present.
func (r *Posts) ReplaceTargets(ctx context.Context, p *post.Post) error {
	keep := make([]uuid.UUID, len(p.Targets))
	for i, t := range p.Targets {
		keep[i] = t.ID
	}
	if _, err := r.db.q(ctx).Exec(ctx, `DELETE FROM post_targets WHERE post_id = $1 AND user_id = $2 AND NOT (id = ANY($3))`,
		p.ID, p.UserID, keep); err != nil {
		return mapErr(err, "post target")
	}
	for i := range p.Targets {
		t := &p.Targets[i]
		tag, err := r.db.q(ctx).Exec(ctx, `UPDATE post_targets SET content = $3 WHERE id = $1 AND user_id = $2`, t.ID, t.UserID, t.Content)
		if err != nil {
			return mapErr(err, "post target")
		}
		if tag.RowsAffected() == 0 {
			if err := r.insertTarget(ctx, t); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateTarget writes mutable target fields.
func (r *Posts) UpdateTarget(ctx context.Context, t *post.Target) error {
	err := r.db.q(ctx).QueryRow(ctx, `UPDATE post_targets SET content = $3, status = $4, external_post_id = $5, external_url = $6,
		published_at = $7, error_code = $8, error_message = $9, attempt_count = $10
		WHERE id = $1 AND user_id = $2 RETURNING updated_at`,
		t.ID, t.UserID, t.Content, t.Status, t.ExternalPostID, t.ExternalURL, t.PublishedAt, t.ErrorCode, t.ErrorMessage, t.AttemptCount).
		Scan(&t.UpdatedAt)
	return mapErr(err, "post target")
}

// ListTargets returns a post's targets.
func (r *Posts) ListTargets(ctx context.Context, userID, postID uuid.UUID) ([]post.Target, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+targetCols+` FROM post_targets t WHERE t.post_id = $1 AND t.user_id = $2 ORDER BY t.created_at, t.id`, postID, userID)
	if err != nil {
		return nil, mapErr(err, "post target")
	}
	defer rows.Close()
	var out []post.Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// LockTarget locks a target with SKIP LOCKED (system: worker only).
// ok=false means the row is locked by another transaction or does not exist.
func (r *Posts) LockTarget(ctx context.Context, id uuid.UUID) (*post.Target, bool, error) {
	t, err := scanTarget(r.db.q(ctx).QueryRow(ctx, `SELECT `+targetCols+` FROM post_targets t WHERE t.id = $1 FOR UPDATE SKIP LOCKED`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if err := r.db.q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM post_targets WHERE id = $1)`, id).Scan(&exists); err != nil {
			return nil, false, err
		}
		if !exists {
			return nil, false, mapErr(pgx.ErrNoRows, "post target")
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}

// LockTargetWait takes a blocking row lock on a target (system: worker only).
func (r *Posts) LockTargetWait(ctx context.Context, id uuid.UUID) error {
	var got uuid.UUID
	return mapErr(r.db.q(ctx).QueryRow(ctx, `SELECT id FROM post_targets WHERE id = $1 FOR UPDATE`, id).Scan(&got), "post target")
}

// StuckPublishing lists targets in publishing not updated since `before` (system).
func (r *Posts) StuckPublishing(ctx context.Context, before time.Time, limit int) ([]post.Target, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+targetCols+` FROM post_targets t
		WHERE t.status = 'publishing' AND t.updated_at < $1 ORDER BY t.updated_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []post.Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

const attemptCols = `id, post_target_id, attempt_no, started_at, finished_at, status, error_code, error_message, response_metadata`

func scanAttempt(row interface{ Scan(...any) error }) (*post.Attempt, error) {
	var a post.Attempt
	var meta []byte
	if err := row.Scan(&a.ID, &a.PostTargetID, &a.AttemptNo, &a.StartedAt, &a.FinishedAt, &a.Status, &a.ErrorCode, &a.ErrorMessage, &meta); err != nil {
		return nil, err
	}
	a.ResponseMetadata = map[string]any{}
	_ = json.Unmarshal(meta, &a.ResponseMetadata)
	return &a, nil
}

// LatestAttempt returns the newest attempt of a target, or nil.
func (r *Posts) LatestAttempt(ctx context.Context, targetID uuid.UUID) (*post.Attempt, error) {
	a, err := scanAttempt(r.db.q(ctx).QueryRow(ctx, `SELECT `+attemptCols+` FROM publication_attempts
		WHERE post_target_id = $1 ORDER BY attempt_no DESC LIMIT 1`, targetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// InsertAttempt inserts an attempt; (post_target_id, attempt_no) is unique.
func (r *Posts) InsertAttempt(ctx context.Context, a *post.Attempt) error {
	meta, _ := json.Marshal(a.ResponseMetadata)
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO publication_attempts (post_target_id, attempt_no, started_at, status, response_metadata)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`, a.PostTargetID, a.AttemptNo, a.StartedAt, a.Status, meta).Scan(&a.ID), "publication attempt")
}

// UpdateAttempt writes the attempt outcome.
func (r *Posts) UpdateAttempt(ctx context.Context, a *post.Attempt) error {
	meta, _ := json.Marshal(a.ResponseMetadata)
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE publication_attempts SET finished_at = $2, status = $3, error_code = $4,
		error_message = $5, response_metadata = $6 WHERE id = $1`, a.ID, a.FinishedAt, a.Status, a.ErrorCode, a.ErrorMessage, meta)
	return mustAffect(tag, err, "publication attempt")
}

// Attempts lists attempts for a post of the user.
func (r *Posts) Attempts(ctx context.Context, userID, postID uuid.UUID) ([]post.Attempt, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT a.id, a.post_target_id, a.attempt_no, a.started_at, a.finished_at, a.status,
		a.error_code, a.error_message, a.response_metadata FROM publication_attempts a
		JOIN post_targets t ON t.id = a.post_target_id WHERE t.post_id = $1 AND t.user_id = $2
		ORDER BY a.started_at, a.attempt_no`, postID, userID)
	if err != nil {
		return nil, mapErr(err, "publication attempt")
	}
	defer rows.Close()
	out := []post.Attempt{}
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
