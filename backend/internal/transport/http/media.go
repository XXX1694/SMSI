package http

import (
	"errors"
	"net/http"

	appmedia "github.com/socialos/backend/internal/application/media"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/transport/httpx"
)

const (
	maxUploadBody   = media.MaxVideoBytes + 1<<20 // largest file + multipart overhead
	multipartMemory = 8 << 20                     // larger parts spill to temp files
)

func (a *API) uploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(w, r, errs.Validationf("file exceeds the 100 MB limit").WithField("file", "too large"))
			return
		}
		httpx.Error(w, r, errs.Validationf("expected multipart/form-data with a 'file' field"))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, r, errs.Validationf("missing 'file' field").WithField("file", "required"))
		return
	}
	defer func() { _ = file.Close() }()
	m, err := a.svc.Media.Upload(r.Context(), actorOf(r), appmedia.UploadInput{File: file, Size: header.Size, OriginalName: header.Filename})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toMediaWithURL(m))
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
