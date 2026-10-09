package account

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/dataexport"
	"github.com/socialos/backend/internal/domain/errs"
)

// batchSize bounds how many rows are held at once while a dataset is written.
const batchSize = 200

// Build is the worker side of an export: it streams the archive to object storage and records the outcome. A
// failure is recorded on the export (the user can ask again) and is not returned, so the queue does not retry a
// build that would only repeat the same work; only an unknown or already claimed export is silently skipped.
func (s *ExportService) Build(ctx context.Context, id uuid.UUID) error {
	// One build at a time per worker is the queue's job: exports run on their own queue with concurrency 1.
	e, err := s.d.Exports.Claim(ctx, id)
	if errs.Is(err, errs.NotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	a := actor.System(e.UserID, "export")
	key := dataexport.ObjectKey(e.UserID, e.ID)
	size, err := s.stream(ctx, e, key)
	// The outcome is written on a context that survives a worker shutdown, otherwise the row would stay "running".
	done := context.WithoutCancel(ctx)
	if err != nil {
		s.fail(done, a, e, key, err)
		return nil
	}
	expires := s.d.Clock.Now().Add(s.d.Retention)
	if err := s.d.Exports.MarkReady(done, e.ID, key, size, expires); err != nil {
		s.removeArchive(done, key)
		return err
	}
	return s.d.Audit.Record(done, a, audit.ActionExportReady, "data_export", e.ID.String(), map[string]any{"size_bytes": size})
}

func (s *ExportService) fail(ctx context.Context, a actor.Actor, e *dataexport.Export, key string, cause error) {
	code := dataexport.ErrBuildFailed
	if errors.Is(cause, context.Canceled) {
		code = dataexport.ErrInterrupted
	}
	s.d.Log.ErrorContext(ctx, "export failed", slog.String("export_id", e.ID.String()), slog.String("user_id", e.UserID.String()), slog.Any("error", cause))
	s.removeArchive(ctx, key) // a failed Put leaves nothing, a failed archive may have been fully stored
	if err := s.d.Exports.MarkFailed(ctx, e.ID, code); err != nil {
		s.d.Log.ErrorContext(ctx, "export not marked failed", slog.String("export_id", e.ID.String()), slog.Any("error", err))
	}
	if err := s.d.Audit.Record(ctx, a, audit.ActionExportFailed, "data_export", e.ID.String(), map[string]any{"error_code": code}); err != nil {
		s.d.Log.ErrorContext(ctx, "export audit failed", slog.String("export_id", e.ID.String()), slog.Any("error", err))
	}
}

// stream writes the archive into a pipe that object storage reads with unknown length (bounded 5 MiB parts, D-015),
// so neither the archive nor a temp file is ever held whole. It returns the archive size.
func (s *ExportService) stream(ctx context.Context, e *dataexport.Export, key string) (int64, error) {
	pr, pw := io.Pipe()
	var size atomic.Int64
	werr := make(chan error, 1)
	go func() {
		err := writeArchive(ctx, &countingWriter{w: pw, n: &size}, s.d, e, s.d.Clock.Now())
		_ = pw.CloseWithError(err) // nil closes with EOF
		werr <- err
	}()
	putErr := s.d.Store.Put(port.WithPutTimeout(ctx, dataexport.BuildTimeout), key, pr, -1, "application/zip")
	_ = pr.CloseWithError(putErr) // unblocks the writer when the store gave up early
	if err := <-werr; err != nil {
		return 0, err
	}
	return size.Load(), putErr
}

type countingWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n.Add(int64(n))
	return n, err
}

// writeArchive writes the whole ZIP. Layout: README.txt, profile.json, one JSON array per dataset, media.json and
// media/<id><ext>. JSON is deflated; media is stored as is because it is already compressed.
func writeArchive(ctx context.Context, w io.Writer, d Deps, e *dataexport.Export, now time.Time) error {
	zw := zip.NewWriter(w)
	if err := addFile(zw, "README.txt", zip.Deflate, now, strings.NewReader(readme(now))); err != nil {
		return err
	}
	fetch := func(ds Dataset) func(after uuid.UUID) ([]Row, error) {
		return func(after uuid.UUID) ([]Row, error) { return d.Data.Rows(ctx, ds, e.UserID, after, batchSize) }
	}
	if err := writeObject(zw, "profile.json", now, fetch(DatasetProfile)); err != nil {
		return err
	}
	for _, ds := range []Dataset{DatasetSocialAccounts, DatasetPosts, DatasetAPIKeys, DatasetMCP, DatasetApprovals, DatasetAuditLogs, DatasetSignInMethods} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeArray(ctx, zw, string(ds)+".json", now, fetch(ds)); err != nil {
			return fmt.Errorf("%s: %w", ds, err)
		}
	}
	if err := writeMedia(ctx, zw, d, e.UserID, now); err != nil {
		return fmt.Errorf("media: %w", err)
	}
	return zw.Close()
}

func readme(now time.Time) string {
	return "Steerpost data export, created " + now.UTC().Format(time.RFC3339) + `

profile.json          your account (no password hash; has_password says whether one is set)
social_accounts.json  connected networks (no tokens or other credentials)
posts.json            posts with their targets and every publishing attempt
media.json            your uploads; the files are in media/
api_keys.json         API keys: name, prefix, scopes, dates (never the key or its hash)
mcp_connections.json  MCP connections
approvals.json        approvals requested by your API keys
audit_logs.json       the audit log
sign_in_methods.json  Google or GitHub accounts you sign in with: provider, account id, email, dates

All times are UTC (RFC 3339). Passwords, API keys and network credentials are not included.
`
}

func addFile(zw *zip.Writer, name string, method uint16, now time.Time, r io.Reader) error {
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: now})
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, r)
	return err
}

// writeObject writes the single row of a dataset as a JSON object.
func writeObject(zw *zip.Writer, name string, now time.Time, fetch func(uuid.UUID) ([]Row, error)) error {
	rows, err := fetch(uuid.Nil)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return fmt.Errorf("%s: expected one row, got %d", name, len(rows))
	}
	return addFile(zw, name, zip.Deflate, now, strings.NewReader(string(rows[0].JSON)+"\n"))
}

// writeArray writes a dataset as a JSON array, batch by batch.
func writeArray(ctx context.Context, zw *zip.Writer, name string, now time.Time, fetch func(uuid.UUID) ([]Row, error)) error {
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
	if err != nil {
		return err
	}
	if _, err := io.WriteString(fw, "["); err != nil {
		return err
	}
	after, first := uuid.Nil, true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := fetch(after)
		if err != nil {
			return err
		}
		for _, r := range rows {
			sep := ",\n"
			if first {
				sep, first = "\n", false
			}
			if _, err := io.WriteString(fw, sep); err != nil {
				return err
			}
			if _, err := fw.Write(r.JSON); err != nil {
				return err
			}
			after = r.ID
		}
		if len(rows) < batchSize {
			break
		}
	}
	_, err = io.WriteString(fw, "\n]\n")
	return err
}

// removeArchive deletes a partial or orphaned archive; a failure is logged, the hourly sweep cannot find it (no row has its key).
func (s *ExportService) removeArchive(ctx context.Context, key string) {
	if err := s.d.Store.Delete(ctx, key); err != nil {
		s.d.Log.ErrorContext(ctx, "export object not deleted", slog.String("key", key), slog.Any("error", err))
	}
}
