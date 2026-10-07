package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// PollState keeps what a single long-polling consumer needs in Redis: a lease
// (so only one process polls) and the next update offset. Keys are
// <prefix>lock and <prefix>offset.
type PollState struct {
	c      *goredis.Client
	prefix string
}

// NewPollState creates the state under a key prefix such as "socialos:telegram:publish:".
func NewPollState(c *goredis.Client, prefix string) *PollState {
	return &PollState{c: c, prefix: prefix}
}

func (s *PollState) lockKey() string   { return s.prefix + "lock" }
func (s *PollState) offsetKey() string { return s.prefix + "offset" }

// Offset returns the stored offset, or 0 when none was saved yet.
func (s *PollState) Offset(ctx context.Context) (int64, error) {
	v, err := s.c.Get(ctx, s.offsetKey()).Int64()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	return v, err
}

// PollLease is a held lease. The lock value is a random token, so a process
// whose lease expired can never renew, commit or release somebody else's.
type PollLease struct {
	s     *PollState
	token string
}

// Acquire takes the lease with SET NX PX. ok is false when someone else holds it.
func (s *PollState) Acquire(ctx context.Context, ttl time.Duration) (*PollLease, bool, error) {
	token := uuid.NewString()
	ok, err := s.c.SetNX(ctx, s.lockKey(), token, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	return &PollLease{s: s, token: token}, true, nil
}

var (
	renewScript = goredis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('pexpire', KEYS[1], ARGV[2]) end
return 0`)
	releaseScript = goredis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) end
return 0`)
	commitScript = goredis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then redis.call('set', KEYS[2], ARGV[2]) return 1 end
return 0`)
)

// Renew extends the lease; false means it expired or was taken over.
func (l *PollLease) Renew(ctx context.Context, ttl time.Duration) (bool, error) {
	n, err := renewScript.Run(ctx, l.s.c, []string{l.s.lockKey()}, l.token, ttl.Milliseconds()).Int()
	return n == 1, err
}

// Commit stores the offset only while the lease is still ours.
func (l *PollLease) Commit(ctx context.Context, offset int64) error {
	n, err := commitScript.Run(ctx, l.s.c, []string{l.s.lockKey(), l.s.offsetKey()}, l.token, strconv.FormatInt(offset, 10)).Int()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("redis: polling lease lost, offset not stored")
	}
	return nil
}

// Release drops the lease if it is still ours.
func (l *PollLease) Release(ctx context.Context) error {
	return releaseScript.Run(ctx, l.s.c, []string{l.s.lockKey()}, l.token).Err()
}
