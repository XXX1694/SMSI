package config

import "time"

// WorkerConfig tunes the worker process lifecycle (issue #39).
type WorkerConfig struct {
	// WorkerShutdownTimeout is how long the worker lets in-flight tasks finish after SIGTERM before it gives up on them.
	// Keep it below the worker's stop_grace_period (45s in the compose files) or Docker kills the process first.
	WorkerShutdownTimeout time.Duration
	// WorkerHTTPAddr is where the worker serves /health and /metrics.
	WorkerHTTPAddr string
}

func loadWorkerConfig() WorkerConfig {
	return WorkerConfig{
		WorkerShutdownTimeout: envDuration("WORKER_SHUTDOWN_TIMEOUT", 40*time.Second),
		WorkerHTTPAddr:        env("WORKER_HTTP_ADDR", ":8081"),
	}
}

func (c *Config) validateWorker() []string {
	var p []string
	if c.WorkerShutdownTimeout < 5*time.Second || c.WorkerShutdownTimeout > 10*time.Minute {
		p = append(p, "WORKER_SHUTDOWN_TIMEOUT must be between 5s and 10m")
	}
	if c.WorkerHTTPAddr == "" {
		p = append(p, "WORKER_HTTP_ADDR must not be empty")
	}
	return p
}
