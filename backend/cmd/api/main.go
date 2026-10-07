// Command api serves the SocialOS REST API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/socialos/backend/internal/app"
	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/observability"
)

func main() {
	// `api healthcheck [url]` is the container HEALTHCHECK: the distroless image has no shell or curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Args[2:]))
	}
	if err := run(); err != nil {
		slog.Error("api exited", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := observability.NewLogger(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.MigrateOnStart {
		if err := postgres.Migrate(ctx, cfg.DatabaseURL, "up", log); err != nil {
			return err
		}
	}
	a, err := app.Build(ctx, cfg, log, app.Overrides{})
	if err != nil {
		return err
	}
	defer a.Close()
	go housekeeping(ctx, a)

	srv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: a.Router(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: 2 * time.Minute,
		IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", slog.String("addr", cfg.HTTPAddr), slog.Bool("mock_providers", cfg.MockProviders))
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	log.Info("shutting down api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// healthcheck GETs /health on the configured listen address (or the URL given) and returns the exit code.
func healthcheck(args []string) int {
	target := ""
	if len(args) > 0 {
		target = args[0]
	} else {
		addr := os.Getenv("HTTP_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck: bad HTTP_ADDR:", err)
			return 2
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		target = "http://" + net.JoinHostPort(host, port) + "/health"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", res.StatusCode)
		return 1
	}
	return 0
}

// housekeeping sweeps idle rate-limit buckets and expired sessions.
func housekeeping(ctx context.Context, a *app.App) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.APILimiter.Sweep()
			a.AuthLimit.Sweep()
			if n, err := a.Services.Auth.PurgeExpiredSessions(ctx); err != nil {
				a.Log.Warn("session purge failed", slog.Any("error", err))
			} else if n > 0 {
				a.Log.Info("purged expired sessions", slog.Int64("count", n))
			}
		}
	}
}
