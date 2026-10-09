package e2e

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/identity/github"
	"github.com/socialos/backend/internal/adapters/identity/oidc"
	"github.com/socialos/backend/internal/application/auth"
	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/testutil/githubfake"
	"github.com/socialos/backend/internal/testutil/oidcfake"
)

const (
	stateCookie  = "socialos_oauth"
	ticketCookie = "socialos_oauth_ticket"
	apiOAuth     = "/api/v1/auth/oauth/"
)

// socialRig is the app with Google (an OIDC fake) and GitHub (a GitHub fake) switched on. The OIDC fake is not
// accounts.google.com, so Google emails are never authoritative here (the domain rules are tested in domain/identity);
// the happy paths run through GitHub and Google covers state, nonce and issuer handling.
type socialRig struct {
	e      *env
	gh     *githubfake.Fake
	google *oidcfake.Fake
}

func newSocialRig(t *testing.T, o envOpts) *socialRig {
	t.Helper()
	gh, goog := githubfake.New(t), oidcfake.New(t)
	ghAdapter, err := github.New(github.Config{ClientID: gh.ClientID, ClientSecret: gh.ClientSecret, AuthURL: gh.AuthURL(), TokenURL: gh.TokenURL(), APIURL: gh.APIURL()})
	if err != nil {
		t.Fatal(err)
	}
	googAdapter, err := oidc.New(oidc.ProviderConfig{ID: identity.Google, Issuer: goog.Issuer(), AuthURL: goog.AuthURL(), TokenURL: goog.TokenURL(),
		JWKSURL: goog.JWKSURL(), ClientID: goog.ClientID, ClientSecret: goog.ClientSecret})
	if err != nil {
		t.Fatal(err)
	}
	o.signIn = []auth.IdentityProvider{googAdapter, ghAdapter}
	return &socialRig{e: newEnv(t, o), gh: gh, google: goog}
}

// flow is one started sign-in, as the browser sees it after /start.
type flow struct {
	provider string
	state    string
	nonce    string
	// challenge is the S256 PKCE challenge the provider received.
	challenge   string
	redirectURI string
	setCookie   *http.Cookie
}

// start begins a sign-in with c and returns what the provider was told. The client keeps the state cookie.
func (r *socialRig) start(t *testing.T, c *client, provider, next string) flow {
	t.Helper()
	path := apiOAuth + provider + "/start"
	if next != "" {
		path += "?next=" + url.QueryEscape(next)
	}
	res := c.do("GET", path, nil)
	if res.status != http.StatusFound {
		t.Fatalf("start %s: %d %s", provider, res.status, res.body)
	}
	f := parseAuthorize(t, provider, res.header.Get("Location"))
	for _, ck := range (&http.Response{Header: res.header}).Cookies() {
		if ck.Name == stateCookie {
			f.setCookie = ck
		}
	}
	return f
}

// parseAuthorize reads what a provider's consent URL carries.
func parseAuthorize(t *testing.T, provider, rawURL string) flow {
	t.Helper()
	loc, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	q := loc.Query()
	f := flow{provider: provider, state: q.Get("state"), nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri")}
	if f.state == "" || f.challenge == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL lacks state or S256 PKCE: %s", loc)
	}
	return f
}

// githubCode registers a GitHub account (bound to the flow's PKCE challenge) and returns its authorization code.
func (r *socialRig) githubCode(f flow, id int64, login, email string) string {
	return r.gh.Code(&githubfake.Account{ID: id, Login: login, Name: "Gina " + login, PKCEChallenge: f.challenge,
		Emails: []githubfake.Email{{Email: email, Primary: true, Verified: true}}})
}

// callback visits the provider callback as c (which holds the state cookie) and returns the redirect.
func (r *socialRig) callback(t *testing.T, c *client, provider, code, state string) *url.URL {
	t.Helper()
	return r.callbackRaw(t, c, provider, "code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), nil)
}

func (r *socialRig) callbackRaw(t *testing.T, c *client, provider, query string, headers map[string]string) *url.URL {
	t.Helper()
	res := c.doWith("GET", apiOAuth+provider+"/callback?"+query, nil, headers)
	if res.status != http.StatusFound {
		t.Fatalf("callback must always be a 302, got %d %s", res.status, res.body)
	}
	loc, err := url.Parse(res.header.Get("Location"))
	if err != nil || loc.Host != "web.test" {
		t.Fatalf("callback must redirect to the web app, got %q", res.header.Get("Location"))
	}
	return loc
}

// signIn runs start and callback for a GitHub account and returns the callback redirect.
func (r *socialRig) githubSignIn(t *testing.T, c *client, id int64, login, email, next string) *url.URL {
	t.Helper()
	f := r.start(t, c, "github", next)
	return r.callback(t, c, "github", r.githubCode(f, id, login, email), f.state)
}

// signUp takes a new GitHub user through callback and the Terms form, and returns the signed-in client.
func (r *socialRig) githubSignUp(t *testing.T, id int64, login, email string) *client {
	t.Helper()
	c := r.e.browser()
	if loc := r.githubSignIn(t, c, id, login, email, ""); loc.Path != "/signup/complete" {
		t.Fatalf("a new user must be sent to the sign-up form, got %s", loc)
	}
	m := c.must("POST", apiOAuth+"complete", map[string]any{"display_name": "Gina", "accept_terms": true}, 201)
	c.csrf, _ = m["csrf_token"].(string)
	return c
}

func (r *socialRig) countRows(sql string, args ...any) int { return r.e.count(sql, args...) }

// errorOf is the `?error=` code of a /login redirect ("" when it is not an error redirect).
func errorOf(loc *url.URL) string {
	if loc.Path != "/login" {
		return ""
	}
	return loc.Query().Get("error")
}

func hasSessionCookie(c *client, e *env) bool {
	u, _ := url.Parse(e.srv.URL)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "socialos_session" {
			return true
		}
	}
	return false
}

// withClock gives the rig a stepping clock and long sessions, for the tests that move time.
func withClock(clk *steppingClock) envOpts {
	return envOpts{clock: clk, mutate: func(c *config.Config) { c.SessionTTL = 24 * time.Hour }}
}

// googleGrant is what the OIDC fake answers for the flow: the nonce the login sent and its PKCE challenge.
func googleGrant(f flow, subject, email string) oidcfake.Grant {
	return oidcfake.Grant{Subject: subject, Email: email, EmailVerified: true, Name: "Gus", Nonce: f.nonce,
		CodeChallenge: f.challenge, RedirectURI: f.redirectURI}
}
