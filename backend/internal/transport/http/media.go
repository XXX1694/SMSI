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

// An upload is cut when its client sends nothing for UploadIdleTimeout, or, after UploadGrace, when its average speed
// since the start is below the floor (UPLOAD_MIN_KBPS). The total time is bounded by size limit / floor, so one rule
// covers all three: a 1 byte/s client cannot hold an upload slot (the server-wide ReadTimeout would allow minutes).
var (
	UploadIdleTimeout = 30 * time.Second
	UploadGrace       = 5 * time.Second
)

const defaultUploadMinKBps = 32

// deadlineBody moves the connection read deadline before every read. Below the speed floor the deadline is "now", so the
// read fails and the handler returns, which frees the upload slot.
type deadlineBody struct {
	rc       *http.ResponseController
	r        io.ReadCloser
	start    time.Time
	bytes    int64
	minBps   float64
	idle     time.Duration
	grace    time.Duration
	deadline time.Time // start + size limit / floor
}

func newDeadlineBody(w http.ResponseWriter, body io.ReadCloser, minKBps int) *deadlineBody {
	if minKBps < 1 {
		minKBps = defaultUploadMinKBps
	}
	bps := float64(minKBps) * 1024
	now := time.Now()
	return &deadlineBody{rc: http.NewResponseController(w), r: body, start: now, minBps: bps, idle: UploadIdleTimeout,
		grace: UploadGrace, deadline: now.Add(time.Duration(float64(maxUploadBody) / bps * float64(time.Second)))}
}

func (d *deadlineBody) Read(p []byte) (int, error) {
	now := time.Now()
	dl := minTime(now.Add(d.idle), d.deadline)
	if el := now.Sub(d.start); el > d.grace && float64(d.bytes) < d.minBps*el.Seconds() {
		dl = now
	}
	_ = d.rc.SetReadDeadline(dl) // unsupported writers just keep the server default
	n, err := d.r.Read(p)
	d.bytes += int64(n)
	return n, err
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
	r.Body = http.MaxBytesReader(w, newDeadlineBody(w, r.Body, a.opt.UploadMinKBps), maxUploadBody)
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
