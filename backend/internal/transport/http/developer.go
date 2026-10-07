package http

import (
	"net/http"
	"time"

	"github.com/socialos/backend/internal/application/developer"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/transport/httpx"
)

type createKeyReq struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.svc.Developer.ListKeys(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]apiKeyDTO, len(keys))
	for i := range keys {
		out[i] = toKey(&keys[i])
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "available_scopes": scopeCatalog()})
}

func scopeCatalog() []map[string]any {
	var out []map[string]any
	for _, s := range apikey.AllScopes() {
		out = append(out, map[string]any{"scope": s, "risk": apikey.RiskOf(s), "dangerous": s.Dangerous()})
	}
	return out
}

func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	var req createKeyReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	k, raw, err := a.svc.Developer.CreateKey(r.Context(), actorOf(r), developer.CreateKeyInput{Name: req.Name, Scopes: req.Scopes, ExpiresAt: req.ExpiresAt})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"api_key": toKey(k), "key": raw, "raw_key": raw,
		"warning": "Store this key now; it will not be shown again."})
}

func (a *API) revokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = a.svc.Developer.RevokeKey(r.Context(), actorOf(r), id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createMCPReq struct {
	Name       string   `json:"name"`
	ClientName string   `json:"client_name"`
	Scopes     []string `json:"scopes"`
}

func (a *API) listMCP(w http.ResponseWriter, r *http.Request) {
	cs, err := a.svc.Developer.ListMCPConnections(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]mcpDTO, len(cs))
	for i := range cs {
		out[i] = toMCP(&cs[i])
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (a *API) createMCP(w http.ResponseWriter, r *http.Request) {
	var req createMCPReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := a.svc.Developer.CreateMCPConnection(r.Context(), actorOf(r), developer.CreateMCPInput{Name: req.Name, ClientName: req.ClientName, Scopes: req.Scopes})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"connection": toMCP(&res.Connection), "key": res.RawKey, "raw_key": res.RawKey,
		"config": res.Config, "warning": "Store this key now; it will not be shown again."})
}

func (a *API) revokeMCP(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = a.svc.Developer.RevokeMCPConnection(r.Context(), actorOf(r), id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) usage(w http.ResponseWriter, r *http.Request) {
	rep, err := a.svc.Developer.Usage(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	byKey := make([]map[string]any, len(rep.Keys))
	for i, u := range rep.Keys {
		byKey[i] = map[string]any{"api_key_id": u.APIKeyID, "name": u.Name, "prefix": u.Prefix, "requests": u.Requests,
			"last_used_at": utcp(u.LastUsedAt)}
	}
	byDay := make([]map[string]any, len(rep.Days))
	for i, d := range rep.Days {
		byDay[i] = map[string]any{"day": d.Day.UTC().Format("2006-01-02"), "requests": d.Requests}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"window_days": rep.WindowDays, "total_requests": rep.Total,
		"by_key": byKey, "by_day": byDay, "items": byKey})
}
