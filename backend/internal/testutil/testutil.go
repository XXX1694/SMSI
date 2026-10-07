// Package testutil provisions isolated Postgres databases and Redis for
// integration tests. Tests are skipped unless TEST_DATABASE_URL is set.
package testutil

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/socialos/backend/internal/infrastructure/postgres"
)

// DatabaseURL returns TEST_DATABASE_URL or skips the test.
func DatabaseURL(t testing.TB) string {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	return u
}

// RedisURL returns TEST_REDIS_URL or skips the test.
func RedisURL(t testing.TB) string {
	t.Helper()
	u := os.Getenv("TEST_REDIS_URL")
	if u == "" {
		t.Skip("TEST_REDIS_URL not set; skipping integration test")
	}
	return u
}

// Logger discards output unless TEST_VERBOSE_LOGS is set.
func Logger() *slog.Logger {
	if os.Getenv("TEST_VERBOSE_LOGS") != "" {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// FreshDatabase creates a uniquely named, migrated database and drops it on cleanup.
// It returns the database URL.
func FreshDatabase(t testing.TB) string {
	t.Helper()
	base := DatabaseURL(t)
	ctx := context.Background()
	name := "socialos_it_" + strings.ReplaceAll(uuid.NewString()[:13], "-", "")
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create db: %v", err)
	}
	_ = admin.Close(ctx)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()
	if err := postgres.Migrate(ctx, dbURL, "up", Logger()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, base)
		if err != nil {
			return
		}
		defer func() { _ = c.Close(ctx) }()
		_, _ = c.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name))
	})
	return dbURL
}

// OpenDB opens a fresh migrated database.
func OpenDB(t testing.TB) *postgres.DB {
	t.Helper()
	db, err := postgres.Open(context.Background(), FreshDatabase(t), 10)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}
