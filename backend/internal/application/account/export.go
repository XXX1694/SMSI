package account

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/dataexport"
	"github.com/socialos/backend/internal/domain/errs"
)

// Export tuning. The cooldown stops a user from rebuilding a multi-gigabyte archive over and over; the link is short
// because the ZIP holds everything the user owns.
const (
	// ExportCooldown is the minimum time between two successful exports of a user.
	ExportCooldown = 24 * time.Hour
	// LinkTTL is how long a download link works.
	LinkTTL = 5 * time.Minute
	// DefaultExportRetention is how long a finished ZIP stays downloadable (EXPORT_RETENTION_DAYS).
	DefaultExportRetention = 7 * 24 * time.Hour

	listLimit = 20
	// pendingGrace is how long a pending export may wait for its task before the sweep enqueues it again.
	pendingGrace = 10 * time.Minute
	// runningLimit is when a running export is declared dead (a crashed worker leaves the row running).
	runningLimit = 2 * time.Hour
	sweepBatch   = 50
)

// Deps bundles the export service's dependencies.
type Deps struct {
	Exports   ExportRepo
	Data      ExportData
	Store     ObjectStore
	Queue     ExportQueue
	Tx        port.TxRunner
	Audit     port.AuditRecorder
	Clock     port.Clock
	Log       *slog.Logger
	Retention time.Duration
}

// ExportService requests, builds, lists and expires data exports.
type ExportService struct {
	d    Deps
	slot chan struct{} // one build at a time per worker (D-018)
}

// NewExportService creates the service. A zero Retention means DefaultExportRetention.
func NewExportService(d Deps) *ExportService {
	if d.Retention <= 0 {
		d.Retention = DefaultExportRetention
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &ExportService{d: d, slot: make(chan struct{}, 1)}
}

// Request starts an export for the session user. 409 while one is being prepared; 429 within the cooldown after a
// successful one (the owner can download that one for the retention period instead).
func (s *ExportService) Request(ctx context.Context, a actor.Actor) (*dataexport.Export, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	var e *dataexport.Export
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		recent, err := s.d.Exports.List(ctx, a.UserID, 1)
		if err != nil {
			return err
		}
		if len(recent) == 1 {
			if recent[0].Active() {
				return errs.New(errs.Conflict, "an export is already being prepared")
			}
			if wait := recent[0].CreatedAt.Add(ExportCooldown).Sub(s.d.Clock.Now()); recent[0].Status == dataexport.StatusReady && wait > 0 {
				return errs.New(errs.RateLimited, "you exported your data less than 24 hours ago; download that export or try again later").WithRetryAfter(wait)
			}
		}
		if err := s.d.Exports.ExpireReady(ctx, a.UserID); err != nil {
			return err
		}
		if e, err = s.d.Exports.Create(ctx, a.UserID); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, a, audit.ActionExportRequested, "data_export", e.ID.String(), nil)
	})
	if err != nil {
		return nil, err
	}
	if err := s.d.Queue.EnqueueExport(ctx, e.ID); err != nil {
		// Without a task the row would block the user's next request, so close it; the sweep repairs a crash here.
		s.d.Log.ErrorContext(ctx, "export not queued", slog.String("export_id", e.ID.String()), slog.Any("error", err))
		if ferr := s.d.Exports.MarkFailed(context.WithoutCancel(ctx), e.ID, dataexport.ErrQueueFailed); ferr != nil {
			s.d.Log.ErrorContext(ctx, "export not closed", slog.String("export_id", e.ID.String()), slog.Any("error", ferr))
		}
		return nil, errs.Wrap(errs.Internal, "could not start the export", err)
	}
	return e, nil
}

// List returns the session user's exports, newest first.
func (s *ExportService) List(ctx context.Context, a actor.Actor) ([]dataexport.Export, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	items, err := s.d.Exports.List(ctx, a.UserID, listLimit)
	now := s.d.Clock.Now()
	for i := range items {
		items[i].Status = items[i].EffectiveStatus(now)
	}
	return items, err
}

// Link is a ready export with a short-lived download URL.
type Link struct {
	Export    *dataexport.Export
	URL       string
	ExpiresAt time.Time
}

// Download returns a presigned URL for a ready export of the session user. Another user's id, an unknown id and an
// export that is not ready are all answered without a URL (NOT_FOUND for the first two, CONFLICT for the third).
func (s *ExportService) Download(ctx context.Context, a actor.Actor, id uuid.UUID) (*Link, error) {
	if err := a.RequireSession(); err != nil {
		return nil, err
	}
	e, err := s.d.Exports.Get(ctx, a.UserID, id)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	if !e.Downloadable(now) {
		return nil, errs.New(errs.Conflict, "this export is not available for download")
	}
	url, err := s.d.Store.PresignGet(ctx, e.StorageKey, LinkTTL)
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "could not create the download link", err)
	}
	if err := s.d.Audit.Record(ctx, a, audit.ActionExportDownloaded, "data_export", e.ID.String(), nil); err != nil {
		return nil, err
	}
	return &Link{Export: e, URL: url, ExpiresAt: now.Add(LinkTTL)}, nil
}

// Sweep deletes expired ZIPs, fails exports whose worker died and re-queues exports whose task was lost. It is
// idempotent and runs hourly in the worker.
func (s *ExportService) Sweep(ctx context.Context) error {
	now := s.d.Clock.Now()
	due, err := s.d.Exports.Sweepable(ctx, now, now.Add(-pendingGrace), now.Add(-runningLimit), sweepBatch)
	if err != nil {
		return err
	}
	for i := range due {
		e := &due[i]
		switch e.Status {
		case dataexport.StatusPending:
			err = s.d.Queue.EnqueueExport(ctx, e.ID)
		case dataexport.StatusRunning:
			err = s.d.Exports.MarkFailed(ctx, e.ID, dataexport.ErrTimedOut)
		default:
			err = s.dropObject(ctx, e)
		}
		if err != nil {
			s.d.Log.WarnContext(ctx, "export sweep step failed", slog.String("export_id", e.ID.String()), slog.Any("error", err))
		}
	}
	return nil
}

func (s *ExportService) dropObject(ctx context.Context, e *dataexport.Export) error {
	if e.StorageKey != "" {
		if err := s.d.Store.Delete(ctx, e.StorageKey); err != nil {
			return err
		}
	}
	return s.d.Exports.ClearObject(ctx, e.ID)
}
