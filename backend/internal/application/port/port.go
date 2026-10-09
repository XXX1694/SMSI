// Package port contains cross-cutting ports shared by application use cases.
package port

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/errs"
)

// TxRunner executes fn in a database transaction carried by ctx.
// Nested calls join the outer transaction.
type TxRunner interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Clock abstracts time for testability.
type Clock interface{ Now() time.Time }

// AuditRecorder writes audit entries (inside the caller's tx when present).
type AuditRecorder interface {
	Record(ctx context.Context, a actor.Actor, action, resourceType, resourceID string, meta map[string]any) error
}

// Encryptor seals/opens secrets at rest.
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
	KeyVersion() string
}

// Page is a keyset pagination request.
type Page struct {
	Limit  int
	Cursor *Cursor
}

// Cursor is a keyset position (created_at, id) for descending lists.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Default and maximum page sizes.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// NewPage validates limit/cursor query values.
func NewPage(limit int, cursor string) (Page, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	p := Page{Limit: limit}
	if cursor == "" {
		return p, nil
	}
	c, err := DecodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	p.Cursor = &c
	return p, nil
}

// Encode returns the opaque cursor string.
func (c Cursor) Encode() string {
	raw := c.At.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses an opaque cursor.
func DecodeCursor(s string) (Cursor, error) {
	bad := errs.Validationf("invalid cursor").WithField("cursor", "invalid")
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, bad
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return Cursor{}, bad
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return Cursor{}, bad
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return Cursor{}, bad
	}
	return Cursor{At: at, ID: uid}, nil
}

// Result is a page of items.
type Result[T any] struct {
	Items      []T
	NextCursor string
}

// Paginate trims a limit+1 fetch and computes the next cursor.
func Paginate[T any](items []T, limit int, key func(T) Cursor) Result[T] {
	if len(items) <= limit {
		return Result[T]{Items: items}
	}
	items = items[:limit]
	return Result[T]{Items: items, NextCursor: key(items[len(items)-1]).Encode()}
}

// ApprovalGate decides whether an actor may perform a dangerous action now. Require returns nil for browser sessions,
// background actors and trusted keys; for any other API key it consumes a matching approved approval carried in ctx
// (inside the caller's transaction) or answers errs.ApprovalRequired with a fresh pending approval (D-013).
type ApprovalGate interface {
	Require(ctx context.Context, a actor.Actor, req approval.Request) error
}

// FailClosedGate is the gate used when none is wired: agents that need approval are refused.
type FailClosedGate struct{}

// Require implements ApprovalGate.
func (FailClosedGate) Require(_ context.Context, a actor.Actor, _ approval.Request) error {
	if a.NeedsApproval() {
		return errs.New(errs.Internal, "approval gate is not configured")
	}
	return nil
}
