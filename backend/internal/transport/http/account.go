package http

import (
	"net/http"

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
