// Command worker runs Asynq publish jobs and the scheduler reconciler.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/socialos/backend/internal/adapters/telegram"
	"github.com/socialos/backend/internal/app"
	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/infrastructure/queue"
	"github.com/socialos/backend/internal/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker exited", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := observability.NewLogger(cfg.LogLevel, cfg.LogFormat).With(slog.String("component", "worker"))
	slog.SetDefault(log)
	cfg.LogWarnings(log)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.Build(ctx, cfg, log, app.Overrides{})
	if err != nil {
		return err
	}
	defer a.Close()

	srv := queue.NewServer(a.Redis.Asynq, queue.ServerConfig{Queue: cfg.QueueName, Concurrency: cfg.WorkerConc,
		ShutdownTimeout: cfg.WorkerShutdownTimeout, Mailer: a.Mailer, Auth: a.Services.Auth}, a.Publisher, log)
	if err := srv.Start(); err != nil {
		return err
	}
	bg := &background{log: log}
	bg.Go(func() { a.Reconciler.Loop(ctx, cfg.ReconcileEvery) })
	bg.Go(func() { serveHealth(ctx, a, cfg.WorkerHTTPAddr, log) })
	bg.Go(func() { purgeApprovals(ctx, a, cfg.ApprovalRetention, log) })
	startTelegramIntake(ctx, bg, a, cfg, log)
	log.Info("worker started", slog.String("queue", cfg.QueueName), slog.Int("concurrency", cfg.WorkerConc))
	<-ctx.Done()
	// Order matters: ctx is already cancelled, so the background jobs are winding down and nothing new is scheduled.
	// Shutdown then stops taking tasks and lets in-flight publishes finish (up to WORKER_SHUTDOWN_TIMEOUT) before the
	// database and Redis are closed by the deferred a.Close().
	log.Info("shutting down worker", slog.Duration("timeout", cfg.WorkerShutdownTimeout))
	srv.Shutdown()
	bg.Wait(10 * time.Second)
	log.Info("worker stopped")
	return nil
}

// purgeApprovals deletes decided and expired approvals older than the retention, once an hour, until ctx ends.
func purgeApprovals(ctx context.Context, a *app.App, retention time.Duration, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := a.Services.Approvals.Purge(ctx, retention); err != nil {
			log.WarnContext(ctx, "approval retention failed", slog.Any("error", err))
		} else if n > 0 {
			log.InfoContext(ctx, "old approvals deleted", slog.Int64("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// startTelegramIntake long-polls the Bot API for the messages that prove chat
// ownership. In webhook mode the API receives them instead and nothing runs here.
// Several workers may run this: a Redis lease lets exactly one of them poll.
func startTelegramIntake(ctx context.Context, bg *background, a *app.App, cfg *config.Config, log *slog.Logger) {
	if cfg.TelegramUpdatesMode != config.TelegramModePolling {
		log.Info("telegram updates arrive by webhook; polling is off", slog.String("mode", cfg.TelegramUpdatesMode))
		return
	}
	poller := a.NewTelegramPoller(telegram.PollerOptions{})
	if poller == nil {
		log.Info("telegram is not configured (TELEGRAM_BOT_TOKEN is empty); not polling for updates")
		return
	}
	bg.Go(func() { poller.Run(ctx) })
}

// serveHealth exposes /health and /metrics for the worker on WORKER_HTTP_ADDR (default :8081).
func serveHealth(ctx context.Context, a *app.App, addr string, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.DB.Ping(r.Context()); err != nil {
			http.Error(w, `{"status":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/metrics", promhttp.HandlerFor(a.Metrics.Registry, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Warn("worker health server stopped", slog.Any("error", err))
	}
}
