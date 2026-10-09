package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/approvals"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/approval"
)

// Approvals implements approvals.Repo. Every query filters by user_id.
type Approvals struct{ db *DB }

// NewApprovals creates the repo.
func NewApprovals(db *DB) *Approvals { return &Approvals{db: db} }

const approvalCols = `id, user_id, actor_type, actor_id, actor_label, action, resource_type, resource_id, fingerprint,
	summary, status, expires_at, decided_at, consumed_at, created_at`

func scanApproval(row interface{ Scan(...any) error }) (*approval.Approval, error) {
	var a approval.Approval
	var summary []byte
	if err := row.Scan(&a.ID, &a.UserID, &a.ActorType, &a.ActorID, &a.ActorLabel, &a.Action, &a.ResourceType, &a.ResourceID,
		&a.Fingerprint, &summary, &a.Status, &a.ExpiresAt, &a.DecidedAt, &a.ConsumedAt, &a.CreatedAt); err != nil {
		return nil, err
	}
	a.Summary = map[string]any{}
	if err := json.Unmarshal(summary, &a.Summary); err != nil {
		return nil, fmt.Errorf("postgres: approval summary: %w", err)
	}
	return &a, nil
}

// Create inserts a pending approval.
func (r *Approvals) Create(ctx context.Context, a *approval.Approval) error {
	summary, err := json.Marshal(a.Summary)
	if err != nil {
		return err
	}
	_, err = r.db.q(ctx).Exec(ctx, `INSERT INTO action_approvals (id, user_id, actor_type, actor_id, actor_label, action,
		resource_type, resource_id, fingerprint, summary, status, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'pending',$11,$12)`,
		a.ID, a.UserID, a.ActorType, a.ActorID, a.ActorLabel, a.Action, a.ResourceType, a.ResourceID, a.Fingerprint,
		summary, a.ExpiresAt, a.CreatedAt)
	return mapErr(err, "approval")
}

// FindPending returns the open approval with the same binding (uses action_approvals_user_idx).
func (r *Approvals) FindPending(ctx context.Context, userID uuid.UUID, b approvals.Binding, now time.Time) (*approval.Approval, error) {
	a, err := scanApproval(r.db.q(ctx).QueryRow(ctx, `SELECT `+approvalCols+` FROM action_approvals
		WHERE user_id = $1 AND status = 'pending' AND expires_at > $2 AND actor_type = $3 AND actor_id = $4
		  AND action = $5 AND resource_type = $6 AND resource_id = $7 AND fingerprint = $8
		ORDER BY created_at DESC LIMIT 1`,
		userID, now, b.ActorType, b.ActorID, b.Action, b.ResourceType, b.ResourceID, b.Fingerprint))
	return a, mapErr(err, "approval")
}

// CountPending counts the open approvals of one key (action_approvals_user_idx: user, status).
func (r *Approvals) CountPending(ctx context.Context, userID uuid.UUID, actorID string, now time.Time) (int, error) {
	var n int
	err := r.db.q(ctx).QueryRow(ctx, `SELECT count(*) FROM action_approvals
		WHERE user_id = $1 AND status = 'pending' AND expires_at > $2 AND actor_id = $3`, userID, now, actorID).Scan(&n)
	return n, err
}

// LockActor serialises check-then-insert for one (user, key) until the surrounding transaction ends.
func (r *Approvals) LockActor(ctx context.Context, userID uuid.UUID, actorID string) error {
	_, err := r.db.q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID.String()+"/"+actorID)
	return err
}

// DeleteDecidedBefore deletes every approval whose deadline passed before t. Whatever its state, such a row is over:
// a pending or approved one expired, a denied or consumed one was decided before its deadline (action_approvals_expires_idx).
func (r *Approvals) DeleteDecidedBefore(ctx context.Context, t time.Time) (int64, error) {
	tag, err := r.db.q(ctx).Exec(ctx, `DELETE FROM action_approvals WHERE expires_at < $1`, t)
	return tag.RowsAffected(), err
}

// Consume flips one approved approval to consumed. The single UPDATE is the lock: of two concurrent callers only one
// sees a row.
func (r *Approvals) Consume(ctx context.Context, userID, id uuid.UUID, b approvals.Binding, now time.Time) (bool, error) {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE action_approvals SET status = 'consumed', consumed_at = $9
		WHERE id = $1 AND user_id = $2 AND status = 'approved' AND expires_at > $9 AND actor_type = $3 AND actor_id = $4
		  AND action = $5 AND resource_type = $6 AND resource_id = $7 AND fingerprint = $8`,
		id, userID, b.ActorType, b.ActorID, b.Action, b.ResourceType, b.ResourceID, b.Fingerprint, now)
	if err != nil {
		return false, mapErr(err, "approval")
	}
	return tag.RowsAffected() == 1, nil
}

// Get returns one approval of the user.
func (r *Approvals) Get(ctx context.Context, userID, id uuid.UUID) (*approval.Approval, error) {
	a, err := scanApproval(r.db.q(ctx).QueryRow(ctx, `SELECT `+approvalCols+` FROM action_approvals WHERE id = $1 AND user_id = $2`, id, userID))
	return a, mapErr(err, "approval")
}

// List pages the user's approvals, newest first.
func (r *Approvals) List(ctx context.Context, userID uuid.UUID, pendingOnly bool, now time.Time, page port.Page) ([]approval.Approval, error) {
	sql := `SELECT ` + approvalCols + ` FROM action_approvals WHERE user_id = $1
		AND (NOT $2 OR (status = 'pending' AND expires_at > $3))`
	args := []any{userID, pendingOnly, now}
	if page.Cursor != nil {
		sql += ` AND (created_at, id) < ($4, $5) ORDER BY created_at DESC, id DESC LIMIT $6`
		args = append(args, page.Cursor.At, page.Cursor.ID, page.Limit)
	} else {
		sql += ` ORDER BY created_at DESC, id DESC LIMIT $4`
		args = append(args, page.Limit)
	}
	rows, err := r.db.q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []approval.Approval{}
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Decide moves an unexpired pending approval to approved or denied.
func (r *Approvals) Decide(ctx context.Context, userID, id uuid.UUID, to approval.Status, now time.Time) (bool, error) {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE action_approvals SET status = $3, decided_at = $4
		WHERE id = $1 AND user_id = $2 AND status = 'pending' AND expires_at > $4`, id, userID, to, now)
	if err != nil {
		return false, mapErr(err, "approval")
	}
	return tag.RowsAffected() == 1, nil
}
