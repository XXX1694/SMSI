// Package github is the sign-in adapter for GitHub. GitHub is plain OAuth 2.0, not OpenID Connect: there is no ID
// token, so after the code exchange the adapter asks the API who the user is (GET /user) and which address to trust
// (GET /user/emails). Only the primary address, and only when GitHub says it is verified, counts as verified.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/identity"
)

const (
	requestTimeout = 10 * time.Second
	maxBody        = 1 << 20
	perPage        = 100
	// maxEmailPages bounds the pagination loop (100 addresses per page); a user has a handful.
	maxEmailPages = 5
)

// Config describes the GitHub endpoints; the zero values of the URLs mean github.com.
type Config struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	// APIURL is the REST base without a trailing slash.
	APIURL     string
	HTTPClient *http.Client
}

// Adapter implements auth.IdentityProvider for GitHub.
type Adapter struct {
	oauth  *oauth2.Config
	apiURL string
	client *http.Client
}

var _ auth.IdentityProvider = (*Adapter)(nil)

// New builds the adapter. It makes no network call.
func New(cfg Config) (*Adapter, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errs.New(errs.Internal, "github: client id and secret are required")
	}
	def := func(v *string, d string) {
		if *v == "" {
			*v = d
		}
	}
	def(&cfg.AuthURL, "https://github.com/login/oauth/authorize")
	def(&cfg.TokenURL, "https://github.com/login/oauth/access_token")
	def(&cfg.APIURL, "https://api.github.com")
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Adapter{client: client, apiURL: strings.TrimRight(cfg.APIURL, "/"), oauth: &oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Scopes: []string{"user:email"},
		Endpoint: oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}}, nil
}

// ID names the provider.
func (a *Adapter) ID() identity.Provider { return identity.GitHub }

// AuthorizeURL builds the consent URL with PKCE (S256). GitHub has no nonce: the state cookie guards the round trip.
func (a *Adapter) AuthorizeURL(r auth.AuthorizeRequest) string {
	c := *a.oauth
	c.RedirectURL = r.RedirectURI
	return c.AuthCodeURL(r.State, oauth2.S256ChallengeOption(r.CodeVerifier))
}

// Exchange redeems the code, reads the profile and the primary address, and drops the access token.
func (a *Adapter) Exchange(ctx context.Context, r auth.ExchangeRequest) (identity.Claims, error) {
	ctx, cancel := context.WithTimeout(context.WithValue(ctx, oauth2.HTTPClient, a.client), requestTimeout)
	defer cancel()
	c := *a.oauth
	c.RedirectURL = r.RedirectURI
	tok, err := c.Exchange(ctx, r.Code, oauth2.VerifierOption(r.CodeVerifier))
	if err != nil {
		return identity.Claims{}, rejected("token exchange failed", err)
	}
	var profile struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if _, err := a.get(ctx, tok.AccessToken, "/user", &profile); err != nil {
		return identity.Claims{}, err
	}
	if profile.ID <= 0 {
		return identity.Claims{}, rejected("profile has no id", nil)
	}
	email, err := a.primaryEmail(ctx, tok.AccessToken)
	if err != nil {
		return identity.Claims{}, err
	}
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = profile.Login
	}
	return identity.Claims{Provider: identity.GitHub, Subject: strconv.FormatInt(profile.ID, 10),
		Email: strings.ToLower(strings.TrimSpace(email.Email)), EmailVerified: email.Primary && email.Verified,
		EmailPrimary: email.Primary, DisplayName: name}, nil
}

type emailEntry struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// primaryEmail returns the account's primary address (verified or not), or the zero value when there is none. The
// public profile email is never used: it need not be verified, and need not be the primary one.
func (a *Adapter) primaryEmail(ctx context.Context, token string) (emailEntry, error) {
	path := fmt.Sprintf("/user/emails?per_page=%d", perPage)
	for range maxEmailPages {
		var page []emailEntry
		next, err := a.get(ctx, token, path, &page)
		if err != nil {
			return emailEntry{}, err
		}
		for _, e := range page {
			if e.Primary {
				return e, nil
			}
		}
		if next == "" {
			return emailEntry{}, nil
		}
		path = next
	}
	return emailEntry{}, nil
}

// get calls the API and returns the rel="next" link, if any, as an absolute URL.
func (a *Adapter) get(ctx context.Context, token, path string, into any) (next string, err error) {
	target := path
	if !strings.HasPrefix(path, "http") {
		target = a.apiURL + path
	} else if !strings.HasPrefix(path, a.apiURL+"/") {
		return "", rejected("pagination left the API host", nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", rejected("api request invalid", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "steerpost")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", rejected("api request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", rejected("api response unreadable", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", rejected(fmt.Sprintf("api answered %d for %s", resp.StatusCode, strings.SplitN(path, "?", 2)[0]), nil)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return "", rejected("api response is not the expected json", err)
	}
	return nextLink(resp.Header.Get("Link")), nil
}

// nextLink extracts the rel="next" URL from a Link header.
func nextLink(h string) string {
	for _, part := range strings.Split(h, ",") {
		seg := strings.Split(strings.TrimSpace(part), ";")
		if len(seg) < 2 {
			continue
		}
		for _, p := range seg[1:] {
			if strings.ReplaceAll(strings.TrimSpace(p), " ", "") == `rel="next"` {
				return strings.Trim(strings.TrimSpace(seg[0]), "<>")
			}
		}
	}
	return ""
}

func rejected(msg string, cause error) error {
	detail := fmt.Errorf("github: %s", msg)
	if cause != nil {
		detail = fmt.Errorf("github: %s: %w", msg, cause)
	}
	return errs.Wrap(errs.ProviderError, "the sign-in provider could not confirm your identity", detail)
}
