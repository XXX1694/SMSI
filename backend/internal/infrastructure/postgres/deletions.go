package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/account"
	"github.com/socialos/backend/internal/domain/errs"
)

// Deletions implements account.DeletionRepo. Indexes used: users_deletion_due_idx and users_deleted_idx (sweep),
// post_targets_user_status_idx and data_exports_active_uniq (busy check), posts_user_id_idx, audit_logs_user_id_idx,
// media_user_id_idx and action_approvals_user_id_idx (batches), analytics_user_captured_idx, and the foreign-key
// indexes scheduled_jobs_target_idx / analytics_target_idx (00006) for the cascades.
type Deletions struct{ db *DB }

// NewDeletions creates the repo.
func NewDeletions(db *DB) *Deletions { return &Deletions{db: db} }

// Schedule sets the schedule and records the request (a record of a cancelled request is replaced).
func (r *Deletions) Schedule(ctx context.Context, userID uuid.UUID, requestedAt, purgeAt time.Time) error {
	q := r.db.q(ctx)
	tag, err := q.Exec(ctx, `UPDATE users SET deletion_scheduled_at = $2 WHERE id = $1 AND status = 'active' AND deletion_scheduled_at IS NULL`, userID, purgeAt)
	if err := mustAffectConflict(tag.RowsAffected(), err, "deletion of this account is already scheduled"); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO account_deletions (user_id, requested_at) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET requested_at = EXCLUDED.requested_at, purged_at = NULL, counts = '{}'`, userID, requestedAt)
	return mapErr(err, "account deletion")
}

// Cancel clears the schedule.
func (r *Deletions) Cancel(ctx context.Context, userID uuid.UUID) error {
	q := r.db.q(ctx)
	tag, err := q.Exec(ctx, `UPDATE users SET deletion_scheduled_at = NULL WHERE id = $1 AND status = 'active' AND deletion_scheduled_at IS NOT NULL`, userID)
	if err := mustAffectConflict(tag.RowsAffected(), err, "no deletion is scheduled for this account"); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `DELETE FROM account_deletions WHERE user_id = $1 AND purged_at IS NULL`, userID)
	return mapErr(err, "account deletion")
}

// Due lists the users the sweep must queue (system).
func (r *Deletions) Due(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT id FROM users
		WHERE (status = 'active' AND deletion_scheduled_at <= $1) OR status = 'deleted' ORDER BY id LIMIT $2`, now, limit)
	if err != nil {
		return nil, mapErr(err, "user")
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Claim marks a due account deleted (system). Re-claiming a deleted account is fine.
func (r *Deletions) Claim(ctx context.Context, userID uuid.UUID, now time.Time) (string, bool, error) {
	var email string
	err := r.db.q(ctx).QueryRow(ctx, `UPDATE users SET status = 'deleted', deleted_at = COALESCE(deleted_at, $2)
		WHERE id = $1 AND ((status = 'active' AND deletion_scheduled_at <= $2) OR status = 'deleted') RETURNING email::text`, userID, now).Scan(&email)
	if err != nil {
		if errs.Is(mapErr(err, "user"), errs.NotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return email, true, nil
}

// Busy reports a running job that must finish before the data can go.
func (r *Deletions) Busy(ctx context.Context, userID uuid.UUID) (string, error) {
	var publishing, exporting bool
	err := r.db.q(ctx).QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM post_targets WHERE user_id = $1 AND status = 'publishing'),
		EXISTS (SELECT 1 FROM data_exports WHERE user_id = $1 AND status IN ('pending','running'))`, userID).Scan(&publishing, &exporting)
	switch {
	case err != nil:
		return "", err
	case publishing:
		return "a post is being published", nil
	case exporting:
		return "an export is being built", nil
	}
	return "", nil
}

// Counts counts what the purge is about to remove.
func (r *Deletions) Counts(ctx context.Context, userID uuid.UUID) (account.Counts, error) {
	var c account.Counts
	err := r.db.q(ctx).QueryRow(ctx, `SELECT
		(SELECT count(*) FROM posts WHERE user_id = $1), (SELECT count(*) FROM media WHERE user_id = $1),
		(SELECT count(*) FROM social_accounts WHERE user_id = $1), (SELECT count(*) FROM api_keys WHERE user_id = $1)`, userID).
		Scan(&c.Posts, &c.Media, &c.SocialAccounts, &c.APIKeys)
	return c, err
}

// SaveCounts stores the counts of the first attempt only.
func (r *Deletions) SaveCounts(ctx context.Context, userID uuid.UUID, c account.Counts) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = r.db.q(ctx).Exec(ctx, `UPDATE account_deletions SET counts = $2 WHERE user_id = $1 AND purged_at IS NULL AND counts = '{}'::jsonb`, userID, b)
	return mapErr(err, "account deletion")
}

// batchSQL maps the closed set of tables to their statement; the table name is never taken from input.
var batchSQL = map[account.Table]string{
	account.TablePosts:     `DELETE FROM posts WHERE id IN (SELECT id FROM posts WHERE user_id = $1 LIMIT $2)`,
	account.TableAuditLogs: `DELETE FROM audit_logs WHERE id IN (SELECT id FROM audit_logs WHERE user_id = $1 LIMIT $2)`,
	account.TableAnalytics: `DELETE FROM analytics WHERE id IN (SELECT id FROM analytics WHERE user_id = $1 LIMIT $2)`,
	account.TableApprovals: `DELETE FROM action_approvals WHERE id IN (SELECT id FROM action_approvals WHERE user_id = $1 LIMIT $2)`,
}

// DeleteBatch deletes up to limit rows of the user from a table.
func (r *Deletions) DeleteBatch(ctx context.Context, userID uuid.UUID, t account.Table, limit int) (int64, error) {
	sql, ok := batchSQL[t]
	if !ok {
		return 0, fmt.Errorf("postgres: table %q is not purged in batches", t)
	}
	tag, err := r.db.q(ctx).Exec(ctx, sql, userID, limit)
	return tag.RowsAffected(), err
}

// MediaBatch returns up to limit of the user's uploads.
func (r *Deletions) MediaBatch(ctx context.Context, userID uuid.UUID, limit int) ([]account.MediaObject, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT id, storage_key FROM media WHERE user_id = $1 ORDER BY id LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []account.MediaObject
	for rows.Next() {
		var m account.MediaObject
		if err := rows.Scan(&m.ID, &m.Key); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMedia deletes the given rows of the user.
func (r *Deletions) DeleteMedia(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	_, err := r.db.q(ctx).Exec(ctx, `DELETE FROM media WHERE user_id = $1 AND id = ANY($2)`, userID, ids)
	return err
}

// ExportKeys lists the storage keys of the user's export archives.
func (r *Deletions) ExportKeys(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT storage_key FROM data_exports WHERE user_id = $1 AND storage_key <> ''`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteUser removes the user row and stamps the deletion record, in one transaction. The record needs no user row
// (no foreign key) and holds no personal data.
func (r *Deletions) DeleteUser(ctx context.Context, userID uuid.UUID, at time.Time) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		q := r.db.q(ctx)
		if _, err := q.Exec(ctx, `INSERT INTO account_deletions (user_id, requested_at, purged_at) VALUES ($1, $2, $2)
			ON CONFLICT (user_id) DO UPDATE SET purged_at = EXCLUDED.purged_at`, userID, at); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		return err
	})
}

// mustAffectConflict turns "no row matched" into CONFLICT with a message the owner can act on.
func mustAffectConflict(rows int64, err error, msg string) error {
	if err != nil {
		return mapErr(err, "user")
	}
	if rows == 0 {
		return errs.New(errs.Conflict, msg)
	}
	return nil
}
