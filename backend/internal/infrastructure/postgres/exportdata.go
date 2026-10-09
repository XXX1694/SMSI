package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/account"
)

// ExportData implements account.ExportData. Every statement lists its columns explicitly: no password hash, token
// hash, key hash, encrypted credential, session or CSRF value can reach the archive, and a column added to a table
// later is not exported until someone adds it here. All statements filter by user and page by id, which the
// (user_id, id) indexes of migration 00005 (posts, audit_logs, media, action_approvals) or the table's user index serve.
type ExportData struct{ db *DB }

// NewExportData creates the repo.
func NewExportData(db *DB) *ExportData { return &ExportData{db: db} }

// $1 user, $2 after id, $3 limit.
var exportQueries = map[account.Dataset]string{
	account.DatasetProfile: `SELECT id, jsonb_build_object('id', id, 'email', email, 'display_name', display_name, 'status', status,
		'plan', plan, 'email_verified_at', email_verified_at, 'terms_accepted_at', terms_accepted_at,
		'terms_version', terms_version, 'has_password', password_hash IS NOT NULL, 'created_at', created_at)::text
		FROM users WHERE id = $1 AND id > $2 ORDER BY id LIMIT $3`,
	account.DatasetSocialAccounts: `SELECT id, jsonb_build_object('id', id, 'provider', provider, 'provider_account_id', provider_account_id,
		'username', username, 'display_name', display_name, 'avatar_url', avatar_url, 'scopes', scopes, 'metadata', metadata,
		'status', status, 'connected_at', connected_at, 'created_at', created_at)::text
		FROM social_accounts WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
	account.DatasetPosts: `SELECT p.id, jsonb_build_object('id', p.id, 'title', p.title, 'content', p.content, 'status', p.status,
		'scheduled_at', p.scheduled_at, 'published_at', p.published_at, 'created_by', p.created_by, 'created_by_ref', p.created_by_ref,
		'deleted_at', p.deleted_at, 'created_at', p.created_at,
		'media_ids', COALESCE((SELECT jsonb_agg(pm.media_id ORDER BY pm.position) FROM post_media pm WHERE pm.post_id = p.id), '[]'::jsonb),
		'targets', COALESCE((SELECT jsonb_agg(jsonb_build_object('id', t.id, 'social_account_id', t.social_account_id,
			'platform', t.platform, 'content', t.content, 'status', t.status, 'external_post_id', t.external_post_id,
			'external_url', t.external_url, 'published_at', t.published_at, 'error_code', t.error_code,
			'error_message', t.error_message, 'attempt_count', t.attempt_count, 'created_at', t.created_at,
			'attempts', COALESCE((SELECT jsonb_agg(jsonb_build_object('attempt_no', a.attempt_no, 'status', a.status,
				'started_at', a.started_at, 'finished_at', a.finished_at, 'error_code', a.error_code,
				'error_message', a.error_message, 'response_metadata', a.response_metadata) ORDER BY a.attempt_no)
				FROM publication_attempts a WHERE a.post_target_id = t.id), '[]'::jsonb)) ORDER BY t.created_at, t.id)
			FROM post_targets t WHERE t.post_id = p.id AND t.user_id = p.user_id), '[]'::jsonb))::text
		FROM posts p WHERE p.user_id = $1 AND p.id > $2 ORDER BY p.id LIMIT $3`,
	account.DatasetAPIKeys: `SELECT id, jsonb_build_object('id', id, 'name', name, 'prefix', prefix, 'scopes', scopes,
		'dangerous_policy', dangerous_policy, 'expires_at', expires_at, 'revoked_at', revoked_at, 'last_used_at', last_used_at,
		'created_at', created_at)::text
		FROM api_keys WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
	account.DatasetMCP: `SELECT id, jsonb_build_object('id', id, 'api_key_id', api_key_id, 'name', name, 'client_name', client_name,
		'last_seen_at', last_seen_at, 'revoked_at', revoked_at, 'created_at', created_at)::text
		FROM mcp_connections WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
	account.DatasetApprovals: `SELECT id, jsonb_build_object('id', id, 'actor_type', actor_type, 'actor_id', actor_id,
		'actor_label', actor_label, 'action', action, 'resource_type', resource_type, 'resource_id', resource_id,
		'summary', summary, 'status', status, 'expires_at', expires_at, 'decided_at', decided_at, 'consumed_at', consumed_at,
		'created_at', created_at)::text
		FROM action_approvals WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
	account.DatasetAuditLogs: `SELECT id, jsonb_build_object('id', id, 'actor_type', actor_type, 'actor_id', actor_id,
		'actor_label', actor_label, 'action', action, 'resource_type', resource_type, 'resource_id', resource_id,
		'metadata', metadata, 'request_id', request_id, 'ip', ip, 'created_at', created_at)::text
		FROM audit_logs WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
	// At most one row per provider; the UNIQUE (user_id, provider) index serves the filter.
	account.DatasetSignInMethods: `SELECT id, jsonb_build_object('id', id, 'provider', provider, 'provider_account_id', subject,
		'email', email, 'email_verified', email_verified, 'linked_at', linked_at, 'last_login_at', last_login_at)::text
		FROM user_identities WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
}

// Rows returns the next batch of a dataset.
func (r *ExportData) Rows(ctx context.Context, ds account.Dataset, userID, after uuid.UUID, limit int) ([]account.Row, error) {
	sql, ok := exportQueries[ds]
	if !ok {
		return nil, fmt.Errorf("postgres: unknown export dataset %q", ds)
	}
	rows, err := r.db.q(ctx).Query(ctx, sql, userID, after, limit)
	if err != nil {
		return nil, mapErr(err, "export data")
	}
	defer rows.Close()
	out := []account.Row{}
	for rows.Next() {
		var row account.Row
		var js string
		if err := rows.Scan(&row.ID, &js); err != nil {
			return nil, err
		}
		row.JSON = []byte(js)
		out = append(out, row)
	}
	return out, rows.Err()
}

// MediaFiles returns the next batch of the user's uploads (served by media_user_id_idx).
func (r *ExportData) MediaFiles(ctx context.Context, userID, after uuid.UUID, limit int) ([]account.MediaFile, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT id, kind, mime_type, size_bytes, original_name, width, height, sha256, created_at, storage_key
		FROM media WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT $3`, userID, after, limit)
	if err != nil {
		return nil, mapErr(err, "media")
	}
	defer rows.Close()
	out := []account.MediaFile{}
	for rows.Next() {
		var m account.MediaFile
		if err := rows.Scan(&m.ID, &m.Kind, &m.MimeType, &m.SizeBytes, &m.OriginalName, &m.Width, &m.Height, &m.SHA256, &m.CreatedAt, &m.StorageKey); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
