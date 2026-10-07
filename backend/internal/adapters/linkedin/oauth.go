package linkedin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// AuthorizeURL builds the consent URL.
func (a *Adapter) AuthorizeURL(p provider.AuthorizeParams) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", a.cfg.ClientID)
	q.Set("redirect_uri", p.RedirectURI)
	q.Set("state", p.State)
	q.Set("scope", strings.Join(a.cfg.Scopes, " "))
	if a.cfg.UsePKCE && p.CodeChallenge != "" {
		q.Set("code_challenge", p.CodeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	return a.cfg.AuthURL + "?" + q.Encode()
}

type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
}

func (t tokenResponse) toToken(now time.Time) provider.Token {
	tok := provider.Token{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken}
	if t.ExpiresIn > 0 {
		e := now.Add(time.Duration(t.ExpiresIn) * time.Second)
		tok.ExpiresAt = &e
	}
	if t.RefreshTokenExpiresIn > 0 {
		e := now.Add(time.Duration(t.RefreshTokenExpiresIn) * time.Second)
		tok.RefreshExpiresAt = &e
	}
	tok.Scopes = append(tok.Scopes, strings.FieldsFunc(t.Scope, func(r rune) bool { return r == ',' || r == ' ' })...)
	return tok
}

// Exchange trades an authorization code for tokens.
func (a *Adapter) Exchange(ctx context.Context, code, verifier, redirectURI string) (provider.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if a.cfg.UsePKCE && verifier != "" {
		form.Set("code_verifier", verifier)
	}
	return a.tokenRequest(ctx, form, provider.KindPermanent)
}

// Refresh uses a refresh token (only available to approved apps).
func (a *Adapter) Refresh(ctx context.Context, refreshToken string) (provider.Token, error) {
	if refreshToken == "" {
		return provider.Token{}, &provider.Error{Kind: provider.KindAuth, Provider: Name, Code: "NO_REFRESH_TOKEN", Message: "LinkedIn authorization has expired"}
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return a.tokenRequest(ctx, form, provider.KindAuth)
}

// tokenRequest posts to the token endpoint. invalidGrantKind classifies a 400.
func (a *Adapter) tokenRequest(ctx context.Context, form url.Values, invalidGrantKind provider.Kind) (provider.Token, error) {
	form.Set("client_id", a.cfg.ClientID)
	form.Set("client_secret", a.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return provider.Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return provider.Token{}, err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		pe := errorFromResponse(resp)
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			pe.Kind = invalidGrantKind
		}
		return provider.Token{}, pe
	}
	var tr tokenResponse
	if err := json.NewDecoder(limited(resp)).Decode(&tr); err != nil || tr.AccessToken == "" {
		return provider.Token{}, &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BAD_TOKEN_RESPONSE", Message: "invalid token response from LinkedIn"}
	}
	return tr.toToken(time.Now()), nil
}

type userinfo struct {
	Sub     string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
	Locale  any    `json:"locale"`
}

// Profile calls the OpenID Connect userinfo endpoint.
func (a *Adapter) Profile(ctx context.Context, accessToken string) (provider.Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.APIBaseURL+"/v2/userinfo", nil)
	if err != nil {
		return provider.Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.http.Do(req)
	if err != nil {
		return provider.Profile{}, err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return provider.Profile{}, errorFromResponse(resp)
	}
	var u userinfo
	if err := json.NewDecoder(limited(resp)).Decode(&u); err != nil || u.Sub == "" {
		return provider.Profile{}, &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BAD_PROFILE", Message: "invalid userinfo response"}
	}
	return provider.Profile{
		ID: u.Sub, Username: u.Email, DisplayName: u.Name, AvatarURL: u.Picture,
		Metadata: map[string]any{"person_urn": personURN(u.Sub)},
	}, nil
}

func personURN(sub string) string { return "urn:li:person:" + sub }
