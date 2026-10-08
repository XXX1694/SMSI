package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/developer"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/audit"
)

// Audit implements audit.Repo and developer.Usage.
type Audit struct{ db *DB }

// NewAudit creates the repo.
func NewAudit(db *DB) *Audit { return &Audit{db: db} }

// Insert writes an entry.
func (r *Audit) Insert(ctx context.Context, e *audit.Entry) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO audit_logs (user_id, actor_type, actor_id, actor_label, action, resource_type, resource_id, metadata, request_id, ip, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		e.UserID, e.ActorType, e.ActorID, e.ActorLabel, e.Action, e.ResourceType, e.ResourceID, meta, e.RequestID, e.IP, e.CreatedAt).Scan(&e.ID), "audit log")
}

// List pages the user's audit log, newest first; a non-empty action keeps only entries of that action.
func (r *Audit) List(ctx context.Context, userID uuid.UUID, action string, page port.Page) ([]audit.Entry, error) {
	sql := `SELECT id, user_id, actor_type, actor_id, actor_label, action, resource_type, resource_id, metadata, request_id, ip, created_at
		FROM audit_logs WHERE user_id = $1 AND ($2 = '' OR action = $2)`
	args := []any{userID, action}
	if page.Cursor != nil {
		sql += ` AND (created_at, id) < ($3, $4) ORDER BY created_at DESC, id DESC LIMIT $5`
		args = append(args, page.Cursor.At, page.Cursor.ID, page.Limit)
	} else {
		sql += ` ORDER BY created_at DESC, id DESC LIMIT $3`
		args = append(args, page.Limit)
	}
	rows, err := r.db.q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []audit.Entry{}
	for rows.Next() {
		var e audit.Entry
		var meta []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.ActorType, &e.ActorID, &e.ActorLabel, &e.Action, &e.ResourceType,
			&e.ResourceID, &meta, &e.RequestID, &e.IP, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Metadata = map[string]any{}
		_ = json.Unmarshal(meta, &e.Metadata)
		out = append(out, e)
	}
	return out, rows.Err()
}

// KeyUsage counts API-key requests per key since `since`.
func (r *Audit) KeyUsage(ctx context.Context, userID uuid.UUID, since time.Time) ([]developer.KeyUsage, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT k.id, k.name, k.prefix, k.last_used_at,
		(SELECT count(*) FROM audit_logs a WHERE a.user_id = $1 AND a.actor_type = 'api_key' AND a.actor_id = k.id::text
		   AND a.action IN ('api_key.request','mcp.tool_call') AND a.created_at >= $2)
		FROM api_keys k WHERE k.user_id = $1 ORDER BY k.created_at DESC`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []developer.KeyUsage{}
	for rows.Next() {
		var u developer.KeyUsage
		if err := rows.Scan(&u.APIKeyID, &u.Name, &u.Prefix, &u.LastUsedAt, &u.Requests); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DailyUsage counts API-key requests per UTC day, including days without requests.
func (r *Audit) DailyUsage(ctx context.Context, userID uuid.UUID, since time.Time) ([]developer.DayUsage, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT (d.day AT TIME ZONE 'UTC'), count(a.id)
		FROM generate_series(date_trunc('day', $2::timestamptz AT TIME ZONE 'UTC'), date_trunc('day', now() AT TIME ZONE 'UTC'), interval '1 day') AS d(day)
		LEFT JOIN audit_logs a ON a.user_id = $1 AND a.actor_type = 'api_key' AND a.action = 'api_key.request'
		  AND date_trunc('day', a.created_at AT TIME ZONE 'UTC') = d.day
		GROUP BY d.day ORDER BY d.day`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []developer.DayUsage{}
	for rows.Next() {
		var u developer.DayUsage
		if err := rows.Scan(&u.Day, &u.Requests); err != nil {
			return nil, err
		}
		u.Day = u.Day.UTC()
		out = append(out, u)
	}
	return out, rows.Err()
}
