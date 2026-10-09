package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/developer"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/approval"
)

// APIKeys implements developer.Keys and auth.APIKeys.
type APIKeys struct{ db *DB }

// NewAPIKeys creates the repo.
func NewAPIKeys(db *DB) *APIKeys { return &APIKeys{db: db} }

const keyCols = `id, user_id, name, prefix, key_hash, scopes, expires_at, revoked_at, last_used_at, created_at, dangerous_policy`

func scanKey(row interface{ Scan(...any) error }) (*apikey.Key, error) {
	var k apikey.Key
	var scopes []string
	if err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.KeyHash, &scopes, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt, &k.CreatedAt, &k.DangerousPolicy); err != nil {
		return nil, err
	}
	k.Scopes = apikey.FromStrings(scopes)
	return &k, nil
}

// Create inserts a key (hash only).
func (r *APIKeys) Create(ctx context.Context, k *apikey.Key) error {
	if k.DangerousPolicy == "" { // never store an empty policy: unset means the safe one
		k.DangerousPolicy = approval.PolicyApprove
	}
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO api_keys (id, user_id, name, prefix, key_hash, scopes, expires_at, dangerous_policy)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`,
		k.ID, k.UserID, k.Name, k.Prefix, k.KeyHash, apikey.Strings(k.Scopes), k.ExpiresAt, k.DangerousPolicy).Scan(&k.CreatedAt), "api key")
}

// List returns the user's keys.
func (r *APIKeys) List(ctx context.Context, userID uuid.UUID) ([]apikey.Key, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+keyCols+` FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []apikey.Key{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// Revoke revokes the user's key (idempotent for already-revoked keys).
func (r *APIKeys) Revoke(ctx context.Context, userID, id uuid.UUID, at time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE api_keys SET revoked_at = COALESCE(revoked_at, $3) WHERE id = $1 AND user_id = $2`, id, userID, at)
	return mustAffect(tag, err, "api key")
}

// RevokeAllForUser revokes the user's keys and MCP connections; already revoked ones keep their time.
func (r *APIKeys) RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) (int64, error) {
	q := r.db.q(ctx)
	tag, err := q.Exec(ctx, `UPDATE api_keys SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, at)
	if err != nil {
		return 0, mapErr(err, "api key")
	}
	if _, err := q.Exec(ctx, `UPDATE mcp_connections SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, at); err != nil {
		return 0, mapErr(err, "mcp connection")
	}
	return tag.RowsAffected(), nil
}

// GetByHash looks up a key by SHA-256 hash (authentication).
func (r *APIKeys) GetByHash(ctx context.Context, hash string) (*apikey.Key, error) {
	k, err := scanKey(r.db.q(ctx).QueryRow(ctx, `SELECT `+keyCols+` FROM api_keys WHERE key_hash = $1`, hash))
	return k, mapErr(err, "api key")
}

// TouchLastUsed updates last_used_at at most once a minute, plus the MCP connection's last_seen_at.
func (r *APIKeys) TouchLastUsed(ctx context.Context, keyID uuid.UUID, now time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE api_keys SET last_used_at = $2
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < $2::timestamptz - interval '1 minute')`, keyID, now)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	_, err = r.db.q(ctx).Exec(ctx, `UPDATE mcp_connections SET last_seen_at = $2 WHERE api_key_id = $1`, keyID, now)
	return err
}

// MCPConnections implements developer.Connections.
type MCPConnections struct{ db *DB }

// NewMCPConnections creates the repo.
func NewMCPConnections(db *DB) *MCPConnections { return &MCPConnections{db: db} }

const mcpSelect = `SELECT c.id, c.user_id, c.api_key_id, c.name, c.client_name, c.last_seen_at, c.revoked_at, c.created_at, k.prefix, k.scopes
	FROM mcp_connections c JOIN api_keys k ON k.id = c.api_key_id`

func scanMCP(row interface{ Scan(...any) error }) (*developer.MCPConnection, error) {
	var c developer.MCPConnection
	var scopes []string
	if err := row.Scan(&c.ID, &c.UserID, &c.APIKeyID, &c.Name, &c.ClientName, &c.LastSeenAt, &c.RevokedAt, &c.CreatedAt, &c.KeyPrefix, &scopes); err != nil {
		return nil, err
	}
	c.Scopes = apikey.FromStrings(scopes)
	return &c, nil
}

// Create inserts a connection.
func (r *MCPConnections) Create(ctx context.Context, c *developer.MCPConnection) error {
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO mcp_connections (id, user_id, api_key_id, name, client_name)
		VALUES ($1,$2,$3,$4,$5) RETURNING created_at`, c.ID, c.UserID, c.APIKeyID, c.Name, c.ClientName).Scan(&c.CreatedAt), "mcp connection")
}

// List returns the user's connections.
func (r *MCPConnections) List(ctx context.Context, userID uuid.UUID) ([]developer.MCPConnection, error) {
	rows, err := r.db.q(ctx).Query(ctx, mcpSelect+` WHERE c.user_id = $1 AND k.user_id = $1 ORDER BY c.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []developer.MCPConnection{}
	for rows.Next() {
		c, err := scanMCP(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Get returns one connection of the user.
func (r *MCPConnections) Get(ctx context.Context, userID, id uuid.UUID) (*developer.MCPConnection, error) {
	c, err := scanMCP(r.db.q(ctx).QueryRow(ctx, mcpSelect+` WHERE c.id = $1 AND c.user_id = $2`, id, userID))
	return c, mapErr(err, "mcp connection")
}

// Revoke marks the connection revoked.
func (r *MCPConnections) Revoke(ctx context.Context, userID, id uuid.UUID, at time.Time) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE mcp_connections SET revoked_at = COALESCE(revoked_at, $3) WHERE id = $1 AND user_id = $2`, id, userID, at)
	return mustAffect(tag, err, "mcp connection")
}
