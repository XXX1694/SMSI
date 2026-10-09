package http

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	appmedia "github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/transport/httpx"
)

// UploadIdleTimeout cuts an upload whose client sends nothing for this long; UploadMaxDuration bounds the whole upload.
// Together they stop a 1 byte/s client from holding an upload slot (the server-wide ReadTimeout would allow minutes).
var (
	UploadIdleTimeout = 30 * time.Second
	UploadMaxDuration = 20 * time.Minute
)

// deadlineBody moves the connection read deadline forward before every read: idle after the last progress, never past end.
type deadlineBody struct {
	rc   *http.ResponseController
	r    io.ReadCloser
	idle time.Duration
	end  time.Time
}

func (d *deadlineBody) Read(p []byte) (int, error) {
	_ = d.rc.SetReadDeadline(minTime(time.Now().Add(d.idle), d.end)) // unsupported writers just keep the server default
	return d.r.Read(p)
}

func (d *deadlineBody) Close() error { return d.r.Close() }

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

const maxUploadBody = media.MaxVideoBytes + 1<<20 // largest file + multipart overhead

// uploadMedia streams the multipart body: the file part goes to object storage as it arrives, so a 100 MB video is
// never held in memory or written to a temp file (D-015).
func (a *API) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > maxUploadBody {
		httpx.Error(w, r, tooLarge())
		return
	}
	r.Body = http.MaxBytesReader(w, &deadlineBody{rc: http.NewResponseController(w), r: r.Body, idle: UploadIdleTimeout,
		end: time.Now().Add(UploadMaxDuration)}, maxUploadBody)
	part, err := filePart(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := a.svc.Media.Upload(r.Context(), actorOf(r), appmedia.UploadInput{File: part, OriginalName: part.FileName()})
	if err != nil {
		var cut *http.MaxBytesError
		if errors.As(err, &cut) {
			err = tooLarge()
		}
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toMediaWithURL(m))
}

func tooLarge() error {
	return errs.Validationf("file exceeds the 100 MB limit").WithField("file", "too large")
}

// filePart advances to the first part named "file" that carries a filename; the parts before it are skipped.
func filePart(r *http.Request) (*multipart.Part, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, errs.Validationf("expected multipart/form-data with a 'file' field")
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errs.Validationf("missing 'file' field").WithField("file", "required")
		}
		if err != nil {
			var cut *http.MaxBytesError
			if errors.As(err, &cut) {
				return nil, tooLarge()
			}
			return nil, errs.Validationf("expected multipart/form-data with a 'file' field")
		}
		if part.FormName() == "file" && part.FileName() != "" {
			return part, nil
		}
	}
}

func (a *API) listMedia(w http.ResponseWriter, r *http.Request) {
	pg, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := a.svc.Media.List(r.Context(), actorOf(r), pg)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]mediaDTO, len(res.Items))
	for i := range res.Items {
		items[i] = toMediaWithURL(&res.Items[i])
	}
	httpx.JSON(w, http.StatusOK, newPage(items, res.NextCursor))
}

func (a *API) getMedia(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := a.svc.Media.Get(r.Context(), actorOf(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toMediaWithURL(m))
}

func (a *API) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = a.svc.Media.Delete(r.Context(), actorOf(r), id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
