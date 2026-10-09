package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/identity"
)

// OAuthFlows implements auth.OAuthFlows.
type OAuthFlows struct{ db *DB }

var _ auth.OAuthFlows = (*OAuthFlows)(nil)

// NewOAuthFlows creates the repo.
func NewOAuthFlows(db *DB) *OAuthFlows { return &OAuthFlows{db: db} }

const flowCols = `id, provider, intent, link_user_id, state_hash, nonce_hash, code_verifier_enc, redirect_after,
	expires_at, used_at, COALESCE(ticket_hash, ''), ticket_expires_at, pending`

func scanFlow(row interface{ Scan(...any) error }) (*identity.Flow, error) {
	var f identity.Flow
	var pending []byte
	if err := row.Scan(&f.ID, &f.Provider, &f.Intent, &f.LinkUserID, &f.StateHash, &f.NonceHash, &f.CodeVerifierEnc,
		&f.RedirectAfter, &f.ExpiresAt, &f.UsedAt, &f.TicketHash, &f.TicketExpiresAt, &pending); err != nil {
		return nil, err
	}
	if pending != nil {
		f.Pending = &identity.PendingSignup{}
		if err := json.Unmarshal(pending, f.Pending); err != nil {
			return nil, err
		}
	}
	return &f, nil
}

// Create stores a new flow.
func (r *OAuthFlows) Create(ctx context.Context, f *identity.Flow) error {
	got, err := scanFlow(r.db.q(ctx).QueryRow(ctx,
		`INSERT INTO auth_oauth_flows (provider, intent, link_user_id, state_hash, nonce_hash, code_verifier_enc, redirect_after, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+flowCols,
		f.Provider, f.Intent, f.LinkUserID, f.StateHash, f.NonceHash, f.CodeVerifierEnc, f.RedirectAfter, f.ExpiresAt))
	if err != nil {
		return mapErr(err, "oauth flow")
	}
	*f = *got
	return nil
}

// ConsumeState marks the flow used in one statement, so a replayed or concurrent callback finds nothing. The provider
// must be the one the flow was started for: a state from the GitHub start is useless at the Google callback.
func (r *OAuthFlows) ConsumeState(ctx context.Context, p identity.Provider, stateHash string, now time.Time) (*identity.Flow, error) {
	f, err := scanFlow(r.db.q(ctx).QueryRow(ctx,
		`UPDATE auth_oauth_flows SET used_at = $2 WHERE state_hash = $1 AND used_at IS NULL AND expires_at > $2 AND provider = $3
		 RETURNING `+flowCols, stateHash, now, p))
	return f, mapErr(err, "oauth flow")
}

// SetPending attaches a sign-up ticket to a consumed flow, once.
func (r *OAuthFlows) SetPending(ctx context.Context, id uuid.UUID, ticketHash string, expiresAt time.Time, p identity.PendingSignup) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tag, err := r.db.q(ctx).Exec(ctx,
		`UPDATE auth_oauth_flows SET ticket_hash = $2, ticket_expires_at = $3, pending = $4
		 WHERE id = $1 AND used_at IS NOT NULL AND ticket_hash IS NULL`, id, ticketHash, expiresAt, raw)
	return mustAffect(tag, err, "oauth flow")
}

// GetByTicket reads a live ticket without redeeming it.
func (r *OAuthFlows) GetByTicket(ctx context.Context, ticketHash string, now time.Time) (*identity.Flow, error) {
	f, err := scanFlow(r.db.q(ctx).QueryRow(ctx,
		`SELECT `+flowCols+` FROM auth_oauth_flows WHERE ticket_hash = $1 AND ticket_expires_at > $2`, ticketHash, now))
	return f, mapErr(err, "oauth flow")
}

// ConsumeTicket redeems a ticket by deleting its row in one statement.
func (r *OAuthFlows) ConsumeTicket(ctx context.Context, ticketHash string, now time.Time) (*identity.Flow, error) {
	f, err := scanFlow(r.db.q(ctx).QueryRow(ctx,
		`DELETE FROM auth_oauth_flows WHERE ticket_hash = $1 AND ticket_expires_at > $2 RETURNING `+flowCols, ticketHash, now))
	return f, mapErr(err, "oauth flow")
}

// DeleteExpired removes flows whose state has expired and which hold no live ticket (system; served by the expires_at index).
func (r *OAuthFlows) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	tag, err := r.db.q(ctx).Exec(ctx,
		`DELETE FROM auth_oauth_flows WHERE expires_at <= $1 AND (ticket_expires_at IS NULL OR ticket_expires_at <= $1)`, now)
	return tag.RowsAffected(), mapErr(err, "oauth flow")
}
