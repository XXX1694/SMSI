package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/accounts"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Accounts implements accounts.Repo.
type Accounts struct{ db *DB }

// NewAccounts creates the repo.
func NewAccounts(db *DB) *Accounts { return &Accounts{db: db} }

const accountCols = `id, user_id, provider, provider_account_id, username, display_name, avatar_url, scopes, metadata, status, connected_at, created_at, updated_at`

func scanAccount(row interface{ Scan(...any) error }) (*socialaccount.Account, error) {
	var a socialaccount.Account
	var meta []byte
	if err := row.Scan(&a.ID, &a.UserID, &a.Provider, &a.ProviderAccountID, &a.Username, &a.DisplayName, &a.AvatarURL,
		&a.Scopes, &meta, &a.Status, &a.ConnectedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.Metadata = map[string]any{}
	_ = json.Unmarshal(meta, &a.Metadata)
	return &a, nil
}

// Upsert inserts or reactivates (user, provider, provider_account_id).
func (r *Accounts) Upsert(ctx context.Context, a *socialaccount.Account) error {
	meta, err := json.Marshal(a.Metadata)
	if err != nil {
		return err
	}
	row := r.db.q(ctx).QueryRow(ctx, `
		INSERT INTO social_accounts (user_id, provider, provider_account_id, username, display_name, avatar_url, scopes, metadata, status, connected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (user_id, provider, provider_account_id) DO UPDATE SET
		  username = EXCLUDED.username, display_name = EXCLUDED.display_name, avatar_url = EXCLUDED.avatar_url,
		  scopes = EXCLUDED.scopes, metadata = EXCLUDED.metadata, status = EXCLUDED.status, connected_at = EXCLUDED.connected_at
		RETURNING `+accountCols,
		a.UserID, a.Provider, a.ProviderAccountID, a.Username, a.DisplayName, a.AvatarURL, a.Scopes, meta, a.Status, a.ConnectedAt)
	got, err := scanAccount(row)
	if err != nil {
		return mapErr(err, "social account")
	}
	*a = *got
	return nil
}

// List returns non-revoked accounts of the user.
func (r *Accounts) List(ctx context.Context, userID uuid.UUID) ([]socialaccount.Account, error) {
	rows, err := r.db.q(ctx).Query(ctx, `SELECT `+accountCols+` FROM social_accounts
		WHERE user_id = $1 AND status <> 'revoked' ORDER BY connected_at DESC, id`, userID)
	if err != nil {
		return nil, mapErr(err, "social account")
	}
	defer rows.Close()
	out := []socialaccount.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Get returns one account of the user (including revoked, for history).
func (r *Accounts) Get(ctx context.Context, userID, id uuid.UUID) (*socialaccount.Account, error) {
	a, err := scanAccount(r.db.q(ctx).QueryRow(ctx, `SELECT `+accountCols+` FROM social_accounts WHERE id = $1 AND user_id = $2`, id, userID))
	return a, mapErr(err, "social account")
}

// SetStatus updates account status.
func (r *Accounts) SetStatus(ctx context.Context, userID, id uuid.UUID, st socialaccount.Status) error {
	tag, err := r.db.q(ctx).Exec(ctx, `UPDATE social_accounts SET status = $3 WHERE id = $1 AND user_id = $2`, id, userID, st)
	return mustAffect(tag, err, "social account")
}

// SaveCredentials upserts encrypted tokens (account id resolved by a tenant-scoped read).
func (r *Accounts) SaveCredentials(ctx context.Context, accountID uuid.UUID, c accounts.EncryptedCredentials) error {
	_, err := r.db.q(ctx).Exec(ctx, `
		INSERT INTO oauth_credentials (social_account_id, access_token_enc, refresh_token_enc, expires_at, refresh_expires_at, key_version)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (social_account_id) DO UPDATE SET access_token_enc = EXCLUDED.access_token_enc,
		  refresh_token_enc = EXCLUDED.refresh_token_enc, expires_at = EXCLUDED.expires_at,
		  refresh_expires_at = EXCLUDED.refresh_expires_at, key_version = EXCLUDED.key_version`,
		accountID, c.AccessTokenEnc, c.RefreshTokenEnc, c.ExpiresAt, c.RefreshExpiresAt, c.KeyVersion)
	return mapErr(err, "credentials")
}

// GetCredentials loads encrypted tokens.
func (r *Accounts) GetCredentials(ctx context.Context, accountID uuid.UUID) (*accounts.EncryptedCredentials, error) {
	var c accounts.EncryptedCredentials
	err := r.db.q(ctx).QueryRow(ctx, `SELECT access_token_enc, refresh_token_enc, expires_at, refresh_expires_at, key_version
		FROM oauth_credentials WHERE social_account_id = $1`, accountID).
		Scan(&c.AccessTokenEnc, &c.RefreshTokenEnc, &c.ExpiresAt, &c.RefreshExpiresAt, &c.KeyVersion)
	if err != nil {
		return nil, mapErr(err, "credentials")
	}
	return &c, nil
}

// DeleteCredentials destroys stored tokens.
func (r *Accounts) DeleteCredentials(ctx context.Context, accountID uuid.UUID) error {
	_, err := r.db.q(ctx).Exec(ctx, `DELETE FROM oauth_credentials WHERE social_account_id = $1`, accountID)
	return mapErr(err, "credentials")
}

// OAuthStates implements accounts.States.
type OAuthStates struct{ db *DB }

// NewOAuthStates creates the repo.
func NewOAuthStates(db *DB) *OAuthStates { return &OAuthStates{db: db} }

// Create stores a pending OAuth state.
func (r *OAuthStates) Create(ctx context.Context, s *accounts.OAuthState) error {
	return mapErr(r.db.q(ctx).QueryRow(ctx, `INSERT INTO oauth_states (user_id, provider, state_hash, code_verifier, redirect_after, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, s.UserID, s.Provider, s.StateHash, s.CodeVerifierEnc, s.RedirectAfter, s.ExpiresAt).Scan(&s.ID), "oauth state")
}

// Consume atomically marks a valid state used (single use, unexpired, same user+provider).
func (r *OAuthStates) Consume(ctx context.Context, userID uuid.UUID, provider, stateHash string, now time.Time) (*accounts.OAuthState, error) {
	var s accounts.OAuthState
	err := r.db.q(ctx).QueryRow(ctx, `
		UPDATE oauth_states SET used_at = $4
		WHERE state_hash = $1 AND user_id = $2 AND provider = $3 AND used_at IS NULL AND expires_at > $4
		RETURNING id, user_id, provider, state_hash, code_verifier, redirect_after, expires_at`,
		stateHash, userID, provider, now).
		Scan(&s.ID, &s.UserID, &s.Provider, &s.StateHash, &s.CodeVerifierEnc, &s.RedirectAfter, &s.ExpiresAt)
	if err != nil {
		return nil, mapErr(err, "oauth state")
	}
	return &s, nil
}
