package config

import "time"

// WorkerConfig tunes the worker process lifecycle (issue #39).
type WorkerConfig struct {
	// WorkerShutdownTimeout is how long the worker lets in-flight tasks finish after SIGTERM before it gives up on them.
	// The worker may need up to workerShutdownMargin more for its background jobs, and the whole stop must fit in the
	// compose stop_grace_period (45s), so the value is capped at MaxWorkerShutdownTimeout.
	WorkerShutdownTimeout time.Duration
	// WorkerHTTPAddr is where the worker serves /health and /metrics.
	WorkerHTTPAddr string
}

// MaxWorkerShutdownTimeout is the largest WORKER_SHUTDOWN_TIMEOUT: 35s plus the 5s background-job margin stays under
// the 45s stop_grace_period in docker-compose.yml, deploy/docker-compose.prod.yml and deploy/docker-compose.host-proxy.yml.
// Raise the compose value first if you raise this.
const MaxWorkerShutdownTimeout = 35 * time.Second

func loadWorkerConfig() WorkerConfig {
	return WorkerConfig{
		WorkerShutdownTimeout: envDuration("WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),
		WorkerHTTPAddr:        env("WORKER_HTTP_ADDR", ":8081"),
	}
}

func (c *Config) validateWorker() []string {
	var p []string
	if c.WorkerShutdownTimeout < 5*time.Second || c.WorkerShutdownTimeout > MaxWorkerShutdownTimeout {
		p = append(p, "WORKER_SHUTDOWN_TIMEOUT must be between 5s and 35s (the worker's stop_grace_period is 45s)")
	}
	if c.WorkerHTTPAddr == "" {
		p = append(p, "WORKER_HTTP_ADDR must not be empty")
	}
	return p
}
