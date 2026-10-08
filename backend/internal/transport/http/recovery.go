package http

import (
	"net/http"

	"github.com/socialos/backend/internal/transport/httpx"
	"github.com/socialos/backend/internal/transport/middleware"
)

type tokenReq struct {
	Token string `json:"token"`
}

type forgotReq struct {
	Email string `json:"email"`
}

type resetReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	// RevokeKeys also revokes every API key and MCP connection (opt-in).
	RevokeKeys bool `json:"revoke_keys"`
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	RevokeKeys      bool   `json:"revoke_keys"`
}

// verifyEmail redeems a mailed verification token.
func (a *API) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := a.svc.Auth.VerifyEmail(r.Context(), req.Token, middleware.ClientInfoFrom(r, a.trusted)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"email_verified": true})
}

// resendVerification mails the session user a new verification link.
func (a *API) resendVerification(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Auth.ResendVerification(r.Context(), actorOf(r)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "delivery": a.opt.MailDelivery})
}

// forgotPassword always answers 202 with the same body, whether or not the
// address is registered.
func (a *API) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := a.svc.Auth.ForgotPassword(r.Context(), req.Email); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "delivery": a.opt.MailDelivery})
}

func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := a.svc.Auth.ResetPassword(r.Context(), req.Token, req.Password, req.RevokeKeys, middleware.ClientInfoFrom(r, a.trusted)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := a.svc.Auth.ChangePassword(r.Context(), actorOf(r), req.CurrentPassword, req.NewPassword, req.RevokeKeys); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
