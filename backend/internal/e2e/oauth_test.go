package e2e

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/infrastructure/crypto"
)

// oauthStart begins the mock OAuth flow for c and returns the callback path
// (with code and state) the mock "consent screen" redirects to. The browser
// client never leaves the API, so the test decides who completes the flow.
func oauthStart(t *testing.T, c *client, provider, redirect string) (callbackPath, state string) {
	t.Helper()
	path := "/api/v1/social/" + provider + "/connect"
	if redirect != "" {
		path += "?redirect=" + url.QueryEscape(redirect)
	}
	r := c.do("GET", path, nil)
	if r.status != http.StatusFound {
		t.Fatalf("connect %s: %d %s", provider, r.status, r.body)
	}
	loc, err := url.Parse(r.header.Get("Location"))
	if err != nil || loc.Query().Get("state") == "" || loc.Query().Get("code") == "" {
		t.Fatalf("bad authorize redirect %q", r.header.Get("Location"))
	}
	return loc.Path + "?" + loc.RawQuery, loc.Query().Get("state")
}

// callback performs the provider callback and returns the web-app redirect.
func callback(t *testing.T, c *client, path string) *url.URL {
	t.Helper()
	r := c.do("GET", path, nil)
	if r.status != http.StatusFound {
		t.Fatalf("callback %s: want 302, got %d %s", path, r.status, r.body)
	}
	loc, err := url.Parse(r.header.Get("Location"))
	if err != nil || loc.Host != "web.test" {
		t.Fatalf("callback must redirect to the web app, got %q", r.header.Get("Location"))
	}
	return loc
}

// stopRedirects makes the client return 3xx responses instead of following
// them, so the test can observe (and control) every hop of the OAuth flow.
func (c *client) stopRedirects() *client {
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func accountCount(t *testing.T, c *client) int {
	t.Helper()
	return len(c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any))
}

func TestOAuthStateValidation(t *testing.T) {
	e := newEnv(t, envOpts{providers: []provider.Provider{newNoLookup("other")}})
	alice, mallory := e.browser().stopRedirects(), e.browser().stopRedirects()
	alice.register("oauth-alice@example.com")
	mallory.register("oauth-mallory@example.com")
	ctx := context.Background()

	t.Run("state is stored hashed and the PKCE verifier encrypted", func(t *testing.T) {
		_, state := oauthStart(t, alice, "mock", "")
		var n int
		if err := e.app.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM oauth_states WHERE state_hash = $1`, state).Scan(&n); err != nil || n != 0 {
			t.Fatalf("raw state must not be stored (n=%d err=%v)", n, err)
		}
		if err := e.app.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM oauth_states WHERE state_hash = $1`, crypto.SHA256Hex(state)).Scan(&n); err != nil || n != 1 {
			t.Fatalf("sha256(state) must be stored (n=%d err=%v)", n, err)
		}
		var verifier string
		if err := e.app.DB.Pool.QueryRow(ctx, `SELECT code_verifier FROM oauth_states ORDER BY created_at DESC LIMIT 1`).Scan(&verifier); err != nil || !strings.HasPrefix(verifier, "v1:") {
			t.Fatalf("code verifier must be encrypted at rest, got %q (%v)", verifier, err)
		}
	})

	t.Run("state of another user is rejected and stays usable for its owner", func(t *testing.T) {
		path, _ := oauthStart(t, alice, "mock", "")
		loc := callback(t, mallory, path)
		if loc.Query().Get("error") != "VALIDATION_ERROR" || loc.Query().Get("connected") != "" {
			t.Fatalf("foreign state accepted: %s", loc)
		}
		if n := accountCount(t, mallory); n != 0 {
			t.Fatalf("mallory got %d accounts through alice's state", n)
		}
		// The failed attempt must not burn alice's state.
		loc = callback(t, alice, path)
		if loc.Query().Get("connected") != "mock" || loc.Path != "/accounts" {
			t.Fatalf("owner could not complete after foreign attempt: %s", loc)
		}
		if n := accountCount(t, alice); n != 1 {
			t.Fatalf("alice has %d accounts", n)
		}
	})

	t.Run("a state is single use", func(t *testing.T) {
		path, _ := oauthStart(t, mallory, "mock", "")
		if loc := callback(t, mallory, path); loc.Query().Get("connected") != "mock" {
			t.Fatalf("first use failed: %s", loc)
		}
		loc := callback(t, mallory, path)
		if loc.Query().Get("error") != "VALIDATION_ERROR" {
			t.Fatalf("replayed state accepted: %s", loc)
		}
		if n := accountCount(t, mallory); n != 1 {
			t.Fatalf("replay changed accounts: %d", n)
		}
	})

	t.Run("an expired state is rejected", func(t *testing.T) {
		path, state := oauthStart(t, alice, "mock", "")
		if _, err := e.app.DB.Pool.Exec(ctx, `UPDATE oauth_states SET expires_at = now() - interval '1 second' WHERE used_at IS NULL AND user_id =
			(SELECT id FROM users WHERE email = 'oauth-alice@example.com')`); err != nil {
			t.Fatal(err)
		}
		loc := callback(t, alice, path)
		if loc.Query().Get("error") != "VALIDATION_ERROR" {
			t.Fatalf("expired state %q accepted: %s", state[:6], loc)
		}
	})

	t.Run("a state is bound to its provider", func(t *testing.T) {
		u := e.browser().stopRedirects()
		u.register("provider-bind@example.com")
		path, _ := oauthStart(t, u, "mock", "")
		cross := strings.Replace(path, "/social/mock/callback", "/social/other/callback", 1)
		loc := callback(t, u, cross)
		if loc.Query().Get("error") == "" || loc.Query().Get("connected") != "" {
			t.Fatalf("state issued for mock accepted by another provider: %s", loc)
		}
		if n := accountCount(t, u); n != 0 {
			t.Fatalf("cross-provider callback created %d accounts", n)
		}
		// ...and it still works for the provider it was issued for.
		if loc := callback(t, u, path); loc.Query().Get("connected") != "mock" {
			t.Fatalf("state burned by the cross-provider attempt: %s", loc)
		}
	})

	t.Run("forged, missing and malformed parameters are rejected", func(t *testing.T) {
		for _, q := range []string{
			"code=mock-code-abc&state=forged-state-value",
			"code=mock-code-abc",
			"state=whatever",
			"",
			"code=mock-code-abc&state=" + strings.Repeat("A", 300),
		} {
			loc := callback(t, alice, "/api/v1/social/mock/callback?"+q)
			if loc.Query().Get("error") != "VALIDATION_ERROR" {
				t.Errorf("query %q: want VALIDATION_ERROR redirect, got %s", q, loc)
			}
		}
	})

	t.Run("callback without a session never connects anything", func(t *testing.T) {
		path, _ := oauthStart(t, alice, "mock", "")
		anon := e.browser().stopRedirects()
		loc := callback(t, anon, path)
		if loc.Query().Get("error") != "UNAUTHENTICATED" {
			t.Fatalf("anonymous callback: %s", loc)
		}
	})

	t.Run("provider-side errors are passed back to the web app", func(t *testing.T) {
		loc := callback(t, alice, "/api/v1/social/mock/callback?error=access_denied&error_description=user+said+no")
		if loc.Query().Get("error") != "access_denied" || loc.Query().Get("provider") != "mock" {
			t.Fatalf("provider error: %s", loc)
		}
	})

	t.Run("redirect after connect is allow-listed", func(t *testing.T) {
		for _, redirect := range []string{"https://evil.example/steal", "//evil.example", "/\\evil.example", "javascript:alert(1)"} {
			path, _ := oauthStart(t, alice, "mock", redirect)
			loc := callback(t, alice, path)
			if loc.Host != "web.test" || loc.Path != "/accounts" {
				t.Errorf("redirect %q leaked: %s", redirect, loc)
			}
		}
		path, _ := oauthStart(t, alice, "mock", "/calendar")
		if loc := callback(t, alice, path); loc.Path != "/calendar" {
			t.Errorf("same-site redirect lost: %s", loc)
		}
	})

	t.Run("only browser sessions may connect accounts", func(t *testing.T) {
		k := e.apiKeyClient(alice.createKey("tries oauth", "social:read", "posts:read"))
		r := k.do("GET", "/api/v1/social/mock/connect", nil)
		if r.status != http.StatusForbidden || r.errCode(t) != "FORBIDDEN" {
			t.Fatalf("api key started OAuth: %d %s", r.status, r.body)
		}
	})

	t.Run("unsupported and unconfigured providers cannot connect", func(t *testing.T) {
		for _, p := range []string{"instagram", "linkedin", "telegram", "does-not-exist"} {
			r := alice.do("GET", "/api/v1/social/"+p+"/connect", nil)
			if r.status != http.StatusNotImplemented || r.errCode(t) != "PROVIDER_NOT_AVAILABLE" {
				t.Errorf("%s: want 501 PROVIDER_NOT_AVAILABLE, got %d %s", p, r.status, r.body)
			}
		}
	})

	t.Run("reconnecting the same provider account updates instead of duplicating", func(t *testing.T) {
		before := accountCount(t, alice)
		path, _ := oauthStart(t, alice, "mock", "")
		callback(t, alice, path)
		if after := accountCount(t, alice); after != before {
			t.Fatalf("reconnect duplicated the account: %d -> %d", before, after)
		}
	})
}
