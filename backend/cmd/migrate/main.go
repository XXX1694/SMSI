// Command migrate applies database migrations: migrate [up|down|status|reset].
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/observability"
)

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(2)
	}
	log := observability.NewLogger(envOr("LOG_LEVEL", "info"), envOr("LOG_FORMAT", "text"))
	if err := postgres.Migrate(context.Background(), url, cmd, log); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
