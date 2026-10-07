// Package developer manages API keys, MCP connections and usage stats.
// All mutations are browser-session only: API keys can never mint or revoke keys.
package developer

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// MCPConnection is an MCP client bound to one API key.
type MCPConnection struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	APIKeyID   uuid.UUID
	Name       string
	ClientName string
	LastSeenAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	KeyPrefix  string
	Scopes     []apikey.Scope
}

// KeyUsage is request volume per key.
type KeyUsage struct {
	APIKeyID   uuid.UUID
	Name       string
	Prefix     string
	Requests   int64
	LastUsedAt *time.Time
}

// Keys persists API keys (tenant-scoped).
type Keys interface {
	Create(ctx context.Context, k *apikey.Key) error
	List(ctx context.Context, userID uuid.UUID) ([]apikey.Key, error)
	Revoke(ctx context.Context, userID, id uuid.UUID, at time.Time) error
}

// Connections persists MCP connections (tenant-scoped).
type Connections interface {
	Create(ctx context.Context, c *MCPConnection) error
	List(ctx context.Context, userID uuid.UUID) ([]MCPConnection, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*MCPConnection, error)
	Revoke(ctx context.Context, userID, id uuid.UUID, at time.Time) error
}

// DayUsage is request volume of all the user's API keys on one UTC day.
type DayUsage struct {
	Day      time.Time
	Requests int64
}

// UsageReport is the GET /developer/usage payload.
type UsageReport struct {
	WindowDays int
	Total      int64
	Keys       []KeyUsage
	Days       []DayUsage
}

// UsageWindowDays is the period covered by usage statistics.
const UsageWindowDays = 30

// Usage reads request counts from the audit log.
type Usage interface {
	KeyUsage(ctx context.Context, userID uuid.UUID, since time.Time) ([]KeyUsage, error)
	// DailyUsage returns one row per UTC day from `since` to now (zero days included).
	DailyUsage(ctx context.Context, userID uuid.UUID, since time.Time) ([]DayUsage, error)
}

// Service implements developer use cases.
type Service struct {
	keys         Keys
	conns        Connections
	usage        Usage
	tx           port.TxRunner
	audit        port.AuditRecorder
	clock        port.Clock
	mcpPublicURL string
	apiPublicURL string
}

// Deps bundles dependencies.
type Deps struct {
	Keys         Keys
	Connections  Connections
	Usage        Usage
	Tx           port.TxRunner
	Audit        port.AuditRecorder
	Clock        port.Clock
	MCPPublicURL string
	APIPublicURL string
}

// NewService creates the developer service.
func NewService(d Deps) *Service {
	return &Service{keys: d.Keys, conns: d.Connections, usage: d.Usage, tx: d.Tx, audit: d.Audit, clock: d.Clock,
		mcpPublicURL: d.MCPPublicURL, apiPublicURL: d.APIPublicURL}
}

// CreateKeyInput is POST /developer/api-keys.
type CreateKeyInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

func (s *Service) validateKeyInput(in CreateKeyInput) (string, []apikey.Scope, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", nil, errs.Validationf("name is required (max 100 characters)").WithField("name", "invalid")
	}
	scopes, err := apikey.ParseScopes(in.Scopes)
	if err != nil {
		return "", nil, err
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.clock.Now()) {
		return "", nil, errs.Validationf("expires_at must be in the future").WithField("expires_at", "in the past")
	}
	return name, scopes, nil
}

// CreateKey mints a key; the raw secret is returned once and never stored.
func (s *Service) CreateKey(ctx context.Context, a actor.Actor, in CreateKeyInput) (*apikey.Key, string, error) {
	if err := a.RequireSession(); err != nil {
		return nil, "", err
	}
	name, scopes, err := s.validateKeyInput(in)
	if err != nil {
		return nil, "", err
	}
	var key *apikey.Key
	var raw string
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		key, raw, err = s.insertKey(ctx, a, name, scopes, in.ExpiresAt)
		return err
	})
	return key, raw, err
}

func (s *Service) insertKey(ctx context.Context, a actor.Actor, name string, scopes []apikey.Scope, exp *time.Time) (*apikey.Key, string, error) {
	gen, err := crypto.GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}
	k := &apikey.Key{ID: uuid.New(), UserID: a.UserID, Name: name, Prefix: gen.Prefix, KeyHash: gen.Hash, Scopes: scopes, ExpiresAt: exp}
	if err := s.keys.Create(ctx, k); err != nil {
		return nil, "", err
	}
	if err := s.audit.Record(ctx, a, audit.ActionAPIKeyCreated, "api_key", k.ID.String(),
		map[string]any{"name": name, "scopes": apikey.Strings(scopes), "prefix": gen.Prefix}); err != nil {
		return nil, "", err
	}
	return k, gen.Raw, nil
}

// ListKeys returns the tenant's keys (never secrets).
func (s *Service) ListKeys(ctx context.Context, a actor.Actor) ([]apikey.Key, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	return s.keys.List(ctx, a.UserID)
}

// RevokeKey revokes a key immediately.
func (s *Service) RevokeKey(ctx context.Context, a actor.Actor, id uuid.UUID) error {
	if err := a.RequireSession(); err != nil {
		return err
	}
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.keys.Revoke(ctx, a.UserID, id, s.clock.Now()); err != nil {
			return err
		}
		return s.audit.Record(ctx, a, audit.ActionAPIKeyRevoked, "api_key", id.String(), nil)
	})
}

// Usage returns per-key and per-day request counts for the last 30 days.
func (s *Service) Usage(ctx context.Context, a actor.Actor) (*UsageReport, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	since := s.clock.Now().UTC().AddDate(0, 0, -(UsageWindowDays - 1))
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC)
	keys, err := s.usage.KeyUsage(ctx, a.UserID, since)
	if err != nil {
		return nil, err
	}
	days, err := s.usage.DailyUsage(ctx, a.UserID, since)
	if err != nil {
		return nil, err
	}
	rep := &UsageReport{WindowDays: UsageWindowDays, Keys: keys, Days: days}
	for _, k := range keys {
		rep.Total += k.Requests
	}
	return rep, nil
}
