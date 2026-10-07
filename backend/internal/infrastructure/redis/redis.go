// Package redis provides Redis connection helpers shared by the API and worker.
package redis

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"
)

// Conn holds a go-redis client (readiness checks) and Asynq connection options.
type Conn struct {
	Client *goredis.Client
	Asynq  asynq.RedisConnOpt
}

// Open parses a redis:// URL.
func Open(url string) (*Conn, error) {
	opt, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}
	aopt, err := asynq.ParseRedisURI(url)
	if err != nil {
		return nil, fmt.Errorf("redis: parse asynq url: %w", err)
	}
	return &Conn{Client: goredis.NewClient(opt), Asynq: aopt}, nil
}

// Ping checks connectivity.
func (c *Conn) Ping(ctx context.Context) error { return c.Client.Ping(ctx).Err() }

// Close releases the client.
func (c *Conn) Close() error { return c.Client.Close() }
