package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/transport/httpx"
	"github.com/socialos/backend/internal/transport/middleware"
)

// The two cookies of the sign-in round trip. Both are scoped to the OAuth routes only, so they are never sent with
// ordinary API calls: the state cookie binds the callback to the browser that started it, and the ticket cookie is the
// bearer of the pending sign-up.
const (
	oauthStateCookie  = "socialos_oauth"
	oauthTicketCookie = "socialos_oauth_ticket"
	oauthCookiePath   = "/api/v1/auth/oauth"
	oauthCookieMaxAge = int(auth.FlowTTL / time.Second)
)

func (a *API) setOAuthCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: oauthCookiePath, Domain: a.opt.CookieDomain, MaxAge: maxAge,
		HttpOnly: true, Secure: a.opt.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (a *API) clearOAuthCookie(w http.ResponseWriter, name string) { a.setOAuthCookie(w, name, "", -1) }

func cookieValue(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

func providerParam(r *http.Request) (identity.Provider, error) {
	p := identity.Provider(chi.URLParam(r, "provider"))
	if !p.Valid() {
		return "", errs.NotFoundf("sign-in provider")
	}
	return p, nil
}

type signInProviderDTO struct {
	ID   identity.Provider `json:"id"`
	Name string            `json:"name"`
}

// authProviders serves GET /auth/providers: the sign-in providers this server has enabled.
func (a *API) authProviders(w http.ResponseWriter, _ *http.Request) {
	list := a.svc.Auth.SocialProviders()
	out := make([]signInProviderDTO, 0, len(list))
	for _, p := range list {
		out = append(out, signInProviderDTO{ID: p.ID, Name: p.Name})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"providers": out})
}

// oauthStart serves GET /auth/oauth/{provider}/start?next=: it sets the state cookie and sends the browser to the provider.
func (a *API) oauthStart(w http.ResponseWriter, r *http.Request) {
	id, err := providerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := a.svc.Auth.StartSocial(r.Context(), id, r.URL.Query().Get("next"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.setOAuthCookie(w, oauthStateCookie, res.State, oauthCookieMaxAge)
	http.Redirect(w, r, res.URL, http.StatusFound)
}

// oauthCallback serves GET /auth/oauth/{provider}/callback. It is a browser navigation, so it never renders an error
// body: every outcome is a 302 to the web app (the user's `next`, the sign-up page, or /login?error=<code>).
func (a *API) oauthCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := providerParam(r)
	if err != nil {
		a.redirectToWeb(w, r, "/login?error="+auth.ErrProviderError)
		return
	}
	q := r.URL.Query()
	var sessionUser uuid.UUID // a link flow finishes only in the browser session it was started in
	if act := actorOf(r); act.IsSession() {
		sessionUser = act.UserID
	}
	res := a.svc.Auth.SocialCallback(r.Context(), id, auth.CallbackInput{Code: q.Get("code"), State: q.Get("state"),
		CookieState: cookieValue(r, oauthStateCookie), ProviderError: q.Get("error"), SessionUserID: sessionUser}, middleware.ClientInfoFrom(r, a.trusted))
	if res.StateSpent {
		a.clearOAuthCookie(w, oauthStateCookie)
	}
	if res.Session != nil {
		a.setSessionCookies(w, *res.Session)
	}
	if res.Ticket != "" {
		a.setOAuthCookie(w, oauthTicketCookie, res.Ticket, int(auth.TicketTTL/time.Second))
	}
	a.redirectToWeb(w, r, res.Redirect)
}

// redirectToWeb sends the browser to an in-app path. The path is built by the service (sanitised `next`, or a fixed
// page), and the origin comes from configuration.
func (a *API) redirectToWeb(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, strings.TrimRight(a.opt.WebBaseURL, "/")+path, http.StatusFound)
}

// oauthPending serves GET /auth/oauth/pending: what the sign-up form will create, from the ticket cookie.
func (a *API) oauthPending(w http.ResponseWriter, r *http.Request) {
	v, err := a.svc.Auth.PendingSignup(r.Context(), cookieValue(r, oauthTicketCookie))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"provider": v.Provider, "email": v.Email, "display_name": v.DisplayName, "next": v.Next})
}

type completeReq struct {
	DisplayName string `json:"display_name"`
	AcceptTerms bool   `json:"accept_terms"`
}

// oauthComplete serves POST /auth/oauth/complete: the new social user accepts the Terms and the account is created.
// There is no session to carry a CSRF token yet; the ticket cookie is SameSite=Lax (never sent on a cross-site POST)
// and the body must be JSON.
func (a *API) oauthComplete(w http.ResponseWriter, r *http.Request) {
	var req completeReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, sess, err := a.svc.Auth.CompleteSignup(r.Context(), auth.CompleteSignupInput{Ticket: cookieValue(r, oauthTicketCookie),
		DisplayName: req.DisplayName, AcceptTerms: req.AcceptTerms}, middleware.ClientInfoFrom(r, a.trusted))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	a.clearOAuthCookie(w, oauthTicketCookie)
	a.setSessionCookies(w, sess)
	a.writeMe(w, r, http.StatusCreated, u, actor.Actor{Type: actor.TypeUser}, sess.CSRFToken)
}

type identityDTO struct {
	Provider    identity.Provider `json:"provider"`
	Email       string            `json:"email"`
	LinkedAt    time.Time         `json:"linked_at"`
	LastLoginAt *time.Time        `json:"last_login_at"`
}

// listIdentities serves GET /auth/identities: the providers linked to the session user, and whether a password exists.
func (a *API) listIdentities(w http.ResponseWriter, r *http.Request) {
	m, err := a.svc.Auth.SignInMethods(r.Context(), actorOf(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]identityDTO, 0, len(m.Identities))
	for _, i := range m.Identities {
		out = append(out, identityDTO{Provider: i.Provider, Email: i.Email, LinkedAt: utc(i.LinkedAt), LastLoginAt: utcp(i.LastLoginAt)})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"identities": out, "has_password": m.HasPassword})
}

// linkIdentity serves POST /auth/identities/{provider}/link: it sets the state cookie and returns the provider's consent
// URL, which the web app navigates to. The provider then calls the ordinary OAuth callback.
func (a *API) linkIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := providerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := a.svc.Auth.StartLink(r.Context(), actorOf(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.setOAuthCookie(w, oauthStateCookie, res.State, oauthCookieMaxAge)
	httpx.JSON(w, http.StatusOK, map[string]any{"authorize_url": res.URL})
}

// unlinkIdentity serves DELETE /auth/identities/{provider}.
func (a *API) unlinkIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := providerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := a.svc.Auth.Unlink(r.Context(), actorOf(r), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setPasswordReq struct {
	NewPassword string `json:"new_password"`
}

// setPassword serves POST /auth/password/set: the first password of a user who signed up with a provider.
func (a *API) setPassword(w http.ResponseWriter, r *http.Request) {
	var req setPasswordReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := a.svc.Auth.SetInitialPassword(r.Context(), actorOf(r), req.NewPassword); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
