package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/approval"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/transport/httpx"
)

// approvalDTO is an approval as the owner sees it. The fingerprint stays server-side.
type approvalDTO struct {
	ID           uuid.UUID      `json:"id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	ActorLabel   string         `json:"actor_label"`
	Summary      map[string]any `json:"summary"`
	Status       string         `json:"status"`
	ExpiresAt    time.Time      `json:"expires_at"`
	DecidedAt    *time.Time     `json:"decided_at"`
	CreatedAt    time.Time      `json:"created_at"`
}

func toApproval(ap *approval.Approval) approvalDTO {
	return approvalDTO{ID: ap.ID, Action: string(ap.Action), ResourceType: ap.ResourceType, ResourceID: ap.ResourceID,
		ActorLabel: ap.ActorLabel, Summary: ap.Summary, Status: string(ap.Status),
		ExpiresAt: utc(ap.ExpiresAt), DecidedAt: utcp(ap.DecidedAt), CreatedAt: utc(ap.CreatedAt)}
}

func (a *API) listApprovals(w http.ResponseWriter, r *http.Request) {
	pg, err := pageParams(r)
	status := r.URL.Query().Get("status")
	if err == nil && status != "" && status != "pending" && status != "all" {
		err = errs.Validationf("status must be pending or all").WithField("status", "invalid")
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := a.svc.Approvals.List(r.Context(), actorOf(r), status == "all", pg)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]approvalDTO, len(res.Items))
	for i := range res.Items {
		items[i] = toApproval(&res.Items[i])
	}
	httpx.JSON(w, http.StatusOK, newPage(items, res.NextCursor))
}

func (a *API) getApproval(w http.ResponseWriter, r *http.Request) {
	a.approvalCall(w, r, a.svc.Approvals.Get)
}

func (a *API) approveApproval(w http.ResponseWriter, r *http.Request) {
	a.approvalCall(w, r, a.svc.Approvals.Approve)
}

func (a *API) denyApproval(w http.ResponseWriter, r *http.Request) {
	a.approvalCall(w, r, a.svc.Approvals.Deny)
}

// approvalCall runs one use case on the approval named by {id} and renders it.
func (a *API) approvalCall(w http.ResponseWriter, r *http.Request, call func(ctx context.Context, act actor.Actor, id uuid.UUID) (*approval.Approval, error)) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	ap, err := call(r.Context(), actorOf(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toApproval(ap))
}
