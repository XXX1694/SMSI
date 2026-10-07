package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// UpdateSource is the getUpdates side of the Bot API.
type UpdateSource interface {
	GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]json.RawMessage, error)
}

// Lease is the right to be the one polling instance. It is renewed while
// polling and lost when it expires or another instance takes over.
type Lease interface {
	// Renew extends the lease; false means it is no longer ours.
	Renew(ctx context.Context, ttl time.Duration) (bool, error)
	// Commit persists the next getUpdates offset, only while the lease is still ours.
	Commit(ctx context.Context, offset int64) error
	// Release gives the lease up (best effort).
	Release(ctx context.Context) error
}

// PollerStore holds the poller's shared state (Redis in production), so several
// worker instances can run while exactly one polls and the offset survives restarts.
type PollerStore interface {
	// Acquire takes the single-poller lease; ok is false when another instance holds it.
	Acquire(ctx context.Context, ttl time.Duration) (lease Lease, ok bool, err error)
	// Offset returns the stored offset (0 when none).
	Offset(ctx context.Context) (int64, error)
}

// UpdateHandler processes one raw update. A returned error means "transient,
// try again"; updates that are simply not actionable must return nil.
type UpdateHandler func(ctx context.Context, raw []byte) error

// PollerOptions tune the polling loop; zero values select the defaults.
type PollerOptions struct {
	Timeout         time.Duration // long-poll duration passed to getUpdates (default 25s)
	LockTTL         time.Duration // lease lifetime (default 60s); renewed every third of it
	RetryMin        time.Duration // first backoff after an error (default 1s)
	RetryMax        time.Duration // backoff ceiling (default 30s)
	StandbyInterval time.Duration // how often an instance without the lease retries (default 10s)
	MaxAttempts     int           // handler failures tolerated per update before it is dropped (default 5)
}

func (o *PollerOptions) defaults() {
	if o.Timeout <= 0 {
		o.Timeout = 25 * time.Second
	}
	if o.LockTTL <= 0 {
		o.LockTTL = 60 * time.Second
	}
	if o.RetryMin <= 0 {
		o.RetryMin = time.Second
	}
	if o.RetryMax < o.RetryMin {
		o.RetryMax = 30 * time.Second
		if o.RetryMax < o.RetryMin {
			o.RetryMax = o.RetryMin
		}
	}
	if o.StandbyInterval <= 0 {
		o.StandbyInterval = 10 * time.Second
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
}

// Poller long-polls getUpdates and hands every update to a handler. Only the
// holder of the store's lease polls, so any number of workers can run it.
type Poller struct {
	src    UpdateSource
	store  PollerStore
	handle UpdateHandler
	log    *slog.Logger
	o      PollerOptions

	failID int64
	failN  int
}

// NewPoller creates a poller. log may be nil.
func NewPoller(src UpdateSource, store PollerStore, handle UpdateHandler, log *slog.Logger, o PollerOptions) *Poller {
	if log == nil {
		log = slog.Default()
	}
	o.defaults()
	return &Poller{src: src, store: store, handle: handle, log: log.With(slog.String("component", "telegram_poller")), o: o}
}

// Run polls until ctx is cancelled. It never returns an error: every failure is
// logged and retried with exponential backoff.
func (p *Poller) Run(ctx context.Context) {
	p.log.Info("telegram polling started")
	defer p.log.Info("telegram polling stopped")
	bo := newBackoff(p.o.RetryMin, p.o.RetryMax)
	for ctx.Err() == nil {
		lease, ok, err := p.store.Acquire(ctx, p.o.LockTTL)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			p.log.Warn("cannot acquire the telegram polling lease", slog.Any("error", err))
			if !sleep(ctx, bo.next()) {
				return
			}
			continue
		case !ok:
			p.log.Debug("another instance is polling telegram; standing by")
			if !sleep(ctx, p.o.StandbyInterval) {
				return
			}
			continue
		}
		if p.pollWith(ctx, lease) {
			bo.reset()
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		_ = lease.Release(rctx)
		cancel()
		// Reached when the lease was lost or the offset could not be read: do not spin.
		if !sleep(ctx, bo.next()) {
			return
		}
	}
}

// pollWith polls while the lease is held; it returns when ctx ends or the lease
// is lost, reporting whether at least one getUpdates call succeeded.
func (p *Poller) pollWith(ctx context.Context, lease Lease) (healthy bool) {
	pctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.renewLoop(pctx, cancel, lease)
	}()
	defer func() {
		cancel()
		wg.Wait()
	}()

	offset, err := p.store.Offset(pctx)
	if err != nil {
		p.log.Warn("cannot read the telegram offset", slog.Any("error", err))
		return false
	}
	bo := newBackoff(p.o.RetryMin, p.o.RetryMax)
	for pctx.Err() == nil {
		raws, err := p.src.GetUpdates(pctx, offset, p.o.Timeout)
		if err != nil {
			if pctx.Err() != nil {
				return healthy
			}
			p.logPollError(err)
			wait := bo.next()
			if ra := provider.RetryAfterOf(err); ra > wait {
				wait = ra
			}
			if !sleep(pctx, wait) {
				return healthy
			}
			continue
		}
		healthy = true
		bo.reset()
		next, herr := p.process(pctx, raws, offset)
		if next > offset {
			cctx, ccancel := context.WithTimeout(context.WithoutCancel(pctx), 3*time.Second)
			if err := lease.Commit(cctx, next); err != nil {
				p.log.Warn("cannot store the telegram offset", slog.Any("error", err))
			}
			ccancel()
			offset = next
		}
		if herr != nil && !sleep(pctx, bo.next()) {
			return healthy
		}
	}
	return healthy
}

func (p *Poller) renewLoop(ctx context.Context, lost context.CancelFunc, lease Lease) {
	t := time.NewTicker(p.o.LockTTL / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ok, err := lease.Renew(ctx, p.o.LockTTL)
			switch {
			case err != nil:
				if ctx.Err() == nil {
					p.log.Warn("cannot renew the telegram polling lease", slog.Any("error", err))
				}
			case !ok:
				p.log.Warn("telegram polling lease lost; another instance took over")
				lost()
				return
			}
		}
	}
}

// process hands a batch to the handler and returns the offset to confirm. When
// the handler fails transiently it stops at that update, so Telegram redelivers
// it; after MaxAttempts failures the update is dropped instead of blocking the queue.
func (p *Poller) process(ctx context.Context, raws []json.RawMessage, offset int64) (int64, error) {
	next := offset
	for _, raw := range raws {
		id, err := UpdateID(raw)
		if err != nil {
			p.log.Warn("skipping a telegram update without update_id")
			continue
		}
		if ctx.Err() != nil {
			return next, ctx.Err()
		}
		if herr := p.safeHandle(ctx, raw); herr != nil {
			if ctx.Err() != nil {
				return next, herr
			}
			if p.failID == id {
				p.failN++
			} else {
				p.failID, p.failN = id, 1
			}
			if p.failN < p.o.MaxAttempts {
				p.log.Warn("telegram update handler failed; will retry", slog.Int64("update_id", id), slog.Int("attempt", p.failN), slog.Any("error", herr))
				return next, herr
			}
			p.log.Error("dropping a telegram update after repeated failures", slog.Int64("update_id", id), slog.Any("error", herr))
		}
		p.failID, p.failN = 0, 0
		if id+1 > next {
			next = id + 1
		}
	}
	return next, nil
}

func (p *Poller) safeHandle(ctx context.Context, raw []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("telegram update handler panic: %v", r)
		}
	}()
	return p.handle(ctx, raw)
}

func (p *Poller) logPollError(err error) {
	var pe *provider.Error
	switch {
	case errors.As(err, &pe) && pe.HTTPStatus == http.StatusConflict:
		p.log.Error("getUpdates conflict: a webhook is set for this bot or another process is polling it; "+
			"run `make -C backend telegram-delete-webhook` or set TELEGRAM_UPDATES_MODE=webhook", slog.Any("error", err))
	case errors.As(err, &pe) && pe.Code == "BOT_UNAUTHORIZED":
		p.log.Error("telegram rejected the bot token (TELEGRAM_BOT_TOKEN)", slog.Any("error", err))
	default:
		p.log.Warn("getUpdates failed; backing off", slog.Any("error", err))
	}
}

// backoff doubles from min to max with jitter (between half and the full delay).
type backoff struct{ min, max, cur time.Duration }

func newBackoff(min, max time.Duration) *backoff { return &backoff{min: min, max: max} }

func (b *backoff) next() time.Duration {
	if b.cur == 0 {
		b.cur = b.min
	} else if b.cur *= 2; b.cur > b.max {
		b.cur = b.max
	}
	half := b.cur / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

func (b *backoff) reset() { b.cur = 0 }

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
