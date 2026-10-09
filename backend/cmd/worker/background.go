package main

import (
	"log/slog"
	"sync"
	"time"
)

// background owns the worker's long-running goroutines (reconciler, health server, purge job, Telegram poller).
// Each one stops when the context it was started with ends; Wait lets main block until they have, so the database
// and Redis are not closed under a running job.
type background struct {
	wg  sync.WaitGroup
	log *slog.Logger
}

// Go runs fn in a goroutine that Wait accounts for.
func (b *background) Go(fn func()) { b.wg.Go(fn) }

// Wait blocks until every goroutine has returned or the timeout passes. It reports whether all of them returned.
func (b *background) Wait(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		b.log.Warn("background jobs did not stop in time; closing anyway", slog.Duration("timeout", timeout))
		return false
	}
}
