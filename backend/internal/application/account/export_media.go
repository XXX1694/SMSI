package account

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	dmedia "github.com/socialos/backend/internal/domain/media"
)

// mediaPage reads one batch of the user's uploads and names each file's path inside the archive.
func mediaPage(ctx context.Context, d Deps, userID, after uuid.UUID) ([]MediaFile, error) {
	files, err := d.Data.MediaFiles(ctx, userID, after, batchSize)
	for i := range files {
		_, ext, _ := dmedia.Classify(files[i].MimeType)
		files[i].File = "media/" + files[i].ID.String() + ext
	}
	return files, err
}

// writeMedia writes media.json and then each file, streamed from storage through a 32 KiB copy buffer. A file that
// cannot be read is listed in media/MISSING.txt instead of failing the whole export.
func writeMedia(ctx context.Context, zw *zip.Writer, d Deps, userID uuid.UUID, now time.Time) error {
	err := writeArray(ctx, zw, "media.json", now, func(after uuid.UUID) ([]Row, error) {
		files, err := mediaPage(ctx, d, userID, after)
		rows := make([]Row, 0, len(files))
		for _, f := range files {
			b, merr := json.Marshal(f)
			if merr != nil {
				return nil, merr
			}
			rows = append(rows, Row{ID: f.ID, JSON: b})
		}
		return rows, err
	})
	if err != nil {
		return err
	}
	var missing []string
	for after := uuid.Nil; ; {
		if err := ctx.Err(); err != nil {
			return err
		}
		files, err := mediaPage(ctx, d, userID, after)
		if err != nil {
			return err
		}
		for _, f := range files {
			if err := copyMedia(ctx, zw, d.Store, f, now); err != nil {
				var broken *archiveError
				if ctx.Err() != nil || errors.As(err, &broken) {
					return err
				}
				d.Log.WarnContext(ctx, "export: media file skipped", slog.String("media_id", f.ID.String()), slog.Any("error", err))
				missing = append(missing, f.File)
			}
			after = f.ID
		}
		if len(files) < batchSize {
			break
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return addFile(zw, "media/MISSING.txt", zip.Deflate, now,
		strings.NewReader("These files could not be read from storage:\n"+strings.Join(missing, "\n")+"\n"))
}

// copyMedia adds one file. The object is opened before the archive entry is created, so an unreadable object leaves
// no half-written entry behind; a read that fails midway aborts the export (the archive would be corrupt).
func copyMedia(ctx context.Context, zw *zip.Writer, store ObjectStore, f MediaFile, now time.Time) error {
	rc, err := store.Get(ctx, f.StorageKey)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: f.File, Method: zip.Store, Modified: now})
	if err != nil {
		return &archiveError{err}
	}
	if _, err := io.Copy(fw, rc); err != nil {
		return &archiveError{err}
	}
	return nil
}

// archiveError marks a failure after the entry was started: the caller must not carry on with this archive.
type archiveError struct{ error }

func (e *archiveError) Unwrap() error { return e.error }
