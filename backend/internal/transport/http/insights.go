package http

import (
	"net/http"
	"time"

	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/transport/httpx"
)

func (a *API) dashboard(w http.ResponseWriter, r *http.Request) {
	s, err := a.svc.Analytics.Dashboard(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"connected_accounts":   s.ConnectedAccounts,
		"scheduled_posts":      s.ScheduledPosts,
		"drafts":               s.Drafts,
		"published_this_month": s.PublishedThisMonth,
		"failed":               s.Failed,
		"upcoming":             toPosts(s.Upcoming),
		"recent":               toPosts(s.Recent),
	})
}

// metricDTO is one aggregated sample. captured_at mirrors day so clients can
// treat `items` as a flat list of {metric, value, captured_at} points.
type metricDTO struct {
	Platform   string    `json:"platform"`
	Metric     string    `json:"metric"`
	Day        time.Time `json:"day"`
	CapturedAt time.Time `json:"captured_at"`
	Value      int64     `json:"value"`
}

func (a *API) analytics(w http.ResponseWriter, r *http.Request) {
	from, err := queryTime(r, "from")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	to, err := queryTime(r, "to")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var f, t time.Time
	if from != nil {
		f = *from
	}
	if to != nil {
		t = *to
	}
	rep, err := a.svc.Analytics.Analytics(r.Context(), actorOf(r), f, t)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows := make([]metricDTO, len(rep.Rows))
	for i, m := range rep.Rows {
		rows[i] = metricDTO{Platform: m.Platform, Metric: m.Metric, Day: utc(m.Day), CapturedAt: utc(m.Day), Value: m.Value}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"from": utc(rep.From), "to": utc(rep.To), "totals": rep.Totals, "items": rows, "series": rows,
		"note": "Network-side analytics are not available for current providers; series contains SocialOS publishing counters."})
}

func (a *API) auditLogs(w http.ResponseWriter, r *http.Request) {
	pg, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	action := r.URL.Query().Get("action")
	if len(action) > 64 {
		httpx.ErrorCode(w, r, errs.Validation, "action filter too long")
		return
	}
	res, err := a.svc.Audit.List(r.Context(), actorOf(r), action, pg)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]auditDTO, len(res.Items))
	for i, e := range res.Items {
		items[i] = toAudit(e)
	}
	httpx.JSON(w, http.StatusOK, newPage(items, res.NextCursor))
}
