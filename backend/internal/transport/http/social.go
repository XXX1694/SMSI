package http

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/transport/httpx"
)

func (a *API) listProviders(w http.ResponseWriter, r *http.Request) {
	ps, err := a.svc.Accounts.Providers(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]providerDTO, len(ps))
	for i, p := range ps {
		out[i] = toProvider(p)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (a *API) listAccounts(w http.ResponseWriter, r *http.Request) {
	accs, err := a.svc.Accounts.List(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]accountDTO, len(accs))
	for i := range accs {
		out[i] = toAccount(&accs[i])
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		acc, gerr := a.svc.Accounts.Get(r.Context(), actorOf(r), id)
		if gerr == nil {
			httpx.JSON(w, http.StatusOK, toAccount(acc))
			return
		}
		err = gerr
	}
	httpx.Error(w, r, err)
}

func (a *API) disconnectAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = a.svc.Accounts.Disconnect(r.Context(), actorOf(r), id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// connect starts OAuth: 302 to the provider, or JSON {authorize_url} for SPA callers.
func (a *API) connect(w http.ResponseWriter, r *http.Request) {
	authURL, err := a.svc.Accounts.BeginOAuth(r.Context(), actorOf(r), chi.URLParam(r, "provider"), r.URL.Query().Get("redirect"), r.URL.Query().Get("account"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if r.URL.Query().Get("format") == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		httpx.JSON(w, http.StatusOK, map[string]string{"authorize_url": authURL})
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

var errCodeRe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// callback finishes OAuth and always redirects back to the web app.
func (a *API) callback(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	q := r.URL.Query()
	if pe := q.Get("error"); pe != "" {
		a.redirectWeb(w, r, "/accounts", url.Values{"error": {errCodeRe.ReplaceAllString(pe, "")}, "provider": {providerName}})
		return
	}
	act, ok := actor.From(r.Context())
	if !ok {
		a.redirectWeb(w, r, "/accounts", url.Values{"error": {string(errs.Unauthenticated)}, "provider": {providerName}})
		return
	}
	redirect, _, err := a.svc.Accounts.CompleteOAuth(r.Context(), act, providerName, q.Get("code"), q.Get("state"))
	if redirect == "" {
		redirect = "/accounts"
	}
	if err != nil {
		a.redirectWeb(w, r, redirect, url.Values{"error": {string(errs.CodeOf(err))}, "provider": {providerName}})
		return
	}
	a.redirectWeb(w, r, redirect, url.Values{"connected": {providerName}})
}

func (a *API) redirectWeb(w http.ResponseWriter, r *http.Request, path string, q url.Values) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	http.Redirect(w, r, a.opt.WebBaseURL+path+sep+q.Encode(), http.StatusFound)
}

type chatReq struct {
	Chat string `json:"chat"`
}

func (a *API) connectTelegram(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	acc, err := a.svc.Accounts.ConnectChat(r.Context(), actorOf(r), "telegram", req.Chat)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toAccount(acc))
}
