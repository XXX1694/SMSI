package http

import (
	"net/http"
	"time"

	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/apikey"
	"github.com/socialos/backend/internal/domain/user"
	"github.com/socialos/backend/internal/transport/httpx"
	"github.com/socialos/backend/internal/transport/middleware"
)

type credentialsReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	AcceptTerms bool   `json:"accept_terms"`
}

func (a *API) setSessionCookies(w http.ResponseWriter, s auth.IssuedSession) {
	maxAge := int(time.Until(s.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookie, Value: s.Token, Path: "/", Domain: a.opt.CookieDomain,
		MaxAge: maxAge, HttpOnly: true, Secure: a.opt.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: middleware.CSRFCookie, Value: s.CSRFToken, Path: "/", Domain: a.opt.CookieDomain,
		MaxAge: maxAge, HttpOnly: false, Secure: a.opt.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (a *API) clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{middleware.SessionCookie, middleware.CSRFCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", Domain: a.opt.CookieDomain, MaxAge: -1,
			HttpOnly: name == middleware.SessionCookie, Secure: a.opt.CookieSecure, SameSite: http.SameSiteLaxMode})
	}
}

// meResp is the body of GET /me and of register/login. The canonical shape is
// {user, scopes, csrf_token, auth_type, api_key}; id/email/display_name are
// flat copies of the user fields for the browser client.
type meResp struct {
	User      userDTO   `json:"user"`
	Scopes    []string  `json:"scopes"`
	CSRFToken *string   `json:"csrf_token"`
	AuthType  string    `json:"auth_type"`
	APIKey    *keyBrief `json:"api_key"`
	// VerificationEnforced: unverified owners get 403 EMAIL_NOT_VERIFIED on connect, schedule, publish and key creation.
	VerificationEnforced bool `json:"verification_enforced"`
	// MailDelivery is "smtp" when mail really leaves the server and "log" when it is only logged.
	MailDelivery string `json:"mail_delivery"`

	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	AcceptTerms bool   `json:"accept_terms"`
}

type keyBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *API) meFor(u *user.User, act actor.Actor, csrf string) meResp {
	resp := meResp{User: toUser(u), AuthType: "session", Scopes: apikey.Strings(act.EffectiveScopes()),
		ID: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName,
		VerificationEnforced: a.opt.RequireVerification, MailDelivery: a.opt.MailDelivery}
	if csrf != "" && act.Type != actor.TypeAPIKey {
		resp.CSRFToken = &csrf
	}
	if act.Type == actor.TypeAPIKey {
		resp.AuthType = "api_key"
		resp.APIKey = &keyBrief{ID: act.APIKeyID.String(), Name: act.Label}
	}
	return resp
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var req credentialsReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, sess, err := a.svc.Auth.Register(r.Context(), auth.RegisterInput{Email: req.Email, Password: req.Password, DisplayName: req.DisplayName, AcceptTerms: req.AcceptTerms},
		middleware.ClientInfoFrom(r, a.trusted))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	a.setSessionCookies(w, sess)
	httpx.JSON(w, http.StatusCreated, a.meFor(u, actor.Actor{Type: actor.TypeUser}, sess.CSRFToken))
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req credentialsReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, sess, err := a.svc.Auth.Login(r.Context(), req.Email, req.Password, middleware.ClientInfoFrom(r, a.trusted))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	a.setSessionCookies(w, sess)
	httpx.JSON(w, http.StatusOK, a.meFor(u, actor.Actor{Type: actor.TypeUser}, sess.CSRFToken))
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Auth.Logout(r.Context(), actorOf(r)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a.clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	act := actorOf(r)
	u, err := a.svc.Auth.Me(r.Context(), act)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a.meFor(u, act, middleware.CSRFToken(r.Context())))
}
