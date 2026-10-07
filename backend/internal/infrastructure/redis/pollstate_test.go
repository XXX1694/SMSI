package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/infrastructure/redis"
	"github.com/socialos/backend/internal/testutil"
)

func newState(t *testing.T) (*redis.PollState, string) {
	t.Helper()
	conn, err := redis.Open(testutil.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	prefix := "test:poll:" + uuid.NewString()[:8] + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		conn.Client.Del(ctx, prefix+"lock", prefix+"offset")
	})
	return redis.NewPollState(conn.Client, prefix), prefix
}

func TestOnlyOneHolderOfTheLease(t *testing.T) {
	s, _ := newState(t)
	ctx := context.Background()
	a, ok, err := s.Acquire(ctx, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: %v %v", ok, err)
	}
	if _, ok, err := s.Acquire(ctx, time.Minute); err != nil || ok {
		t.Fatalf("second acquire must fail while held: %v %v", ok, err)
	}
	if ok, err := a.Renew(ctx, time.Minute); err != nil || !ok {
		t.Fatalf("holder renews: %v %v", ok, err)
	}
	if err := a.Release(ctx); err != nil {
		t.Fatal(err)
	}
	b, ok, err := s.Acquire(ctx, time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire after release: %v %v", ok, err)
	}
	// The first holder's handle is stale now: it must not be able to touch the new holder's lease or offset.
	if ok, _ := a.Renew(ctx, time.Minute); ok {
		t.Fatal("stale lease renewed")
	}
	if err := a.Commit(ctx, 99); err == nil {
		t.Fatal("stale lease committed an offset")
	}
	_ = a.Release(ctx)
	if ok, _ := b.Renew(ctx, time.Minute); !ok {
		t.Fatal("a stale Release must not drop the current holder's lease")
	}
}

func TestLeaseExpires(t *testing.T) {
	s, _ := newState(t)
	ctx := context.Background()
	a, ok, _ := s.Acquire(ctx, 80*time.Millisecond)
	if !ok {
		t.Fatal("acquire")
	}
	time.Sleep(150 * time.Millisecond)
	if ok, _ := a.Renew(ctx, time.Minute); ok {
		t.Fatal("an expired lease cannot be renewed")
	}
	if err := a.Commit(ctx, 5); err == nil {
		t.Fatal("an expired lease cannot commit")
	}
	if _, ok, _ := s.Acquire(ctx, time.Minute); !ok {
		t.Fatal("another instance can take over after expiry")
	}
}

func TestOffsetPersistsAcrossLeases(t *testing.T) {
	s, _ := newState(t)
	ctx := context.Background()
	if off, err := s.Offset(ctx); err != nil || off != 0 {
		t.Fatalf("initial offset %d %v", off, err)
	}
	a, _, _ := s.Acquire(ctx, time.Minute)
	if err := a.Commit(ctx, 123456789012); err != nil {
		t.Fatal(err)
	}
	_ = a.Release(ctx)
	if off, err := s.Offset(ctx); err != nil || off != 123456789012 {
		t.Fatalf("offset %d %v", off, err)
	}
}
