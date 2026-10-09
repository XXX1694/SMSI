package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/dataexport"
	"github.com/socialos/backend/internal/domain/quota"
	"github.com/socialos/backend/internal/transport/httpx"
)

// accountUsage serves GET /account/usage: the plan, the current period and used/limit per quota.
func (a *API) accountUsage(w http.ResponseWriter, r *http.Request) {
	rep, err := a.svc.Quota.Report(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	quotas := make(map[quota.Metric]map[string]any, len(rep.Items))
	for m, it := range rep.Items {
		q := map[string]any{"limit": it.Limit}
		if it.Used != nil {
			q["used"] = *it.Used
		}
		quotas[m] = q
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"plan": rep.Plan, "period_start": utc(rep.PeriodStart),
		"period_end": utc(rep.PeriodEnd), "quotas": quotas})
}

type exportDTO struct {
	ID        uuid.UUID  `json:"id"`
	Status    string     `json:"status"`
	SizeBytes int64      `json:"size_bytes"`
	ErrorCode *string    `json:"error_code"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func toExport(e dataexport.Export) exportDTO {
	return exportDTO{ID: e.ID, Status: string(e.Status), SizeBytes: e.SizeBytes, ErrorCode: nullable(e.ErrorCode),
		CreatedAt: utc(e.CreatedAt), ExpiresAt: utcp(e.ExpiresAt)}
}

// requestExport serves POST /account/exports: it queues a data export for the session user.
func (a *API) requestExport(w http.ResponseWriter, r *http.Request) {
	e, err := a.svc.Exports.Request(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, toExport(*e))
}

// listExports serves GET /account/exports.
func (a *API) listExports(w http.ResponseWriter, r *http.Request) {
	items, err := a.svc.Exports.List(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]exportDTO, len(items))
	for i, e := range items {
		out[i] = toExport(e)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

// getExport serves GET /account/exports/{id}: the export and, when it is ready, a short-lived download URL.
func (a *API) getExport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	l, err := a.svc.Exports.Download(r.Context(), actorOf(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		exportDTO
		URL          string    `json:"url"`
		URLExpiresAt time.Time `json:"url_expires_at"`
	}{toExport(*l.Export), l.URL, utc(l.ExpiresAt)})
}
