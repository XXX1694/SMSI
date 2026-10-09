package http

import (
	"net/http"

	"github.com/socialos/backend/internal/transport/httpx"
)

type connectTokenReq struct {
	Provider string            `json:"provider"`
	Fields   map[string]string `json:"fields"`
}

// connectWithToken connects an account from pasted credentials. The response is
// the public account only; the request body is never logged or echoed.
func (a *API) connectWithToken(w http.ResponseWriter, r *http.Request) {
	var req connectTokenReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	acc, err := a.svc.Accounts.ConnectWithToken(r.Context(), actorOf(r), req.Provider, req.Fields)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toAccount(acc))
}
