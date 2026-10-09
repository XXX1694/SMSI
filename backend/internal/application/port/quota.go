package port

import (
	"context"

	"github.com/google/uuid"
)

// QuotaGate enforces the plan limits (D-014). Every method takes a lock on the user's quota and must run in the
// transaction that makes the counted change, so two concurrent requests cannot both pass the check. Failures are
// QUOTA_EXCEEDED.
type QuotaGate interface {
	// EnforceAccount allows connecting one more account unless the user is at the limit. The identity being
	// (re)connected is not counted against itself, so reconnecting an existing account is free.
	EnforceAccount(ctx context.Context, userID uuid.UUID, provider, providerAccountID string) error
	// PrecheckAccount refuses early, before an approval is spent or a network is called, when the user is at the limit
	// and has no account on this provider (so the connection cannot be a reconnect). The authoritative check is
	// EnforceAccount in the transaction that stores the account. It takes no lock.
	PrecheckAccount(ctx context.Context, userID uuid.UUID, provider string) error
	// EnforcePost allows counting one more post in the current UTC month.
	EnforcePost(ctx context.Context, userID uuid.UUID) error
	// EnforceMedia allows storing size more bytes.
	EnforceMedia(ctx context.Context, userID uuid.UUID, size int64) error
}

// NoQuota enforces nothing (tests, and callers that were not given a gate).
type NoQuota struct{}

func (NoQuota) EnforceAccount(context.Context, uuid.UUID, string, string) error { return nil }
func (NoQuota) PrecheckAccount(context.Context, uuid.UUID, string) error        { return nil }
func (NoQuota) EnforcePost(context.Context, uuid.UUID) error                    { return nil }
func (NoQuota) EnforceMedia(context.Context, uuid.UUID, int64) error            { return nil }
