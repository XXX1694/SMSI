package e2e

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/testutil/oidcfake"
)

// browserWith is a fresh client that sends exactly the given state cookie (or none), as another browser would.
func (r *socialRig) browserWith(cookie string) *client {
	c := r.e.browser().stopRedirects()
	if cookie != "" {
		u, _ := url.Parse(r.e.srv.URL)
		c.http.Jar.SetCookies(u, []*http.Cookie{{Name: stateCookie, Value: cookie, Path: "/api/v1/auth/oauth"}})
	}
	return c
}

// replayAt copies c's cookies for the OAuth routes (where the ticket cookie lives) into a new browser.
func (r *socialRig) replayAt(c *client) *client {
	u, _ := url.Parse(r.e.srv.URL + apiOAuth)
	other := r.e.browser()
	other.http.Jar.SetCookies(u, c.http.Jar.Cookies(u))
	return other
}

func (r *socialRig) flowUsed(state string) bool {
	return r.countRows(`SELECT count(*) FROM auth_oauth_flows WHERE used_at IS NOT NULL AND state_hash = encode(sha256($1::bytea), 'hex')`, state) == 1
}

func TestSocialReplayedStateIsRefused(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.e.browser().stopRedirects()
	f := r.start(t, c, "github", "")
	if loc := r.callback(t, c, "github", r.githubCode(f, 6001, "rey", "rey@example.com"), f.state); loc.Path != "/signup/complete" {
		t.Fatalf("first use: %s", loc)
	}
	// The same state and cookie again, from the same browser or another: the row is spent.
	for _, replay := range []*client{c, r.browserWith(f.state)} {
		loc := r.callback(t, replay, "github", r.githubCode(f, 6001, "rey", "rey@example.com"), f.state)
		if got := errorOf(loc); got != "oauth_state_invalid" {
			t.Fatalf("replay: got %q (%s)", got, loc)
		}
	}
	if n := r.countRows(`SELECT count(*) FROM auth_oauth_flows WHERE ticket_hash IS NOT NULL AND ticket_hash <> ''`); n != 1 {
		t.Fatalf("a replay must not mint another ticket: %d", n)
	}
}

func TestSocialStateCookieMustMatchTheQueryState(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	starter := r.e.browser().stopRedirects()
	f := r.start(t, starter, "github", "")
	code := func() string { return r.githubCode(f, 7001, "mal", "mal@example.com") }

	// Login CSRF: a victim's browser is sent to the callback with the attacker's state and code, but has no (or another) cookie.
	for name, victim := range map[string]*client{"no cookie": r.browserWith(""), "other cookie": r.browserWith("some-other-state-value")} {
		loc := r.callback(t, victim, "github", code(), f.state)
		if got := errorOf(loc); got != "oauth_state_invalid" {
			t.Fatalf("%s: got %q", name, got)
		}
		if hasSessionCookie(victim, r.e) {
			t.Fatalf("%s: a session was issued", name)
		}
	}
	// A bare mismatch does not burn the flow; the browser that started it can still finish.
	if r.flowUsed(f.state) {
		t.Fatal("a rejected callback consumed the state")
	}
	// The query state must be the one in the cookie even when both name live flows.
	other := r.start(t, starter, "github", "")
	if loc := r.callback(t, starter, "github", code(), f.state); errorOf(loc) != "oauth_state_invalid" {
		t.Fatalf("cookie holds the second flow's state, query the first's: %s", loc)
	}
	if loc := r.callback(t, starter, "github", r.githubCode(other, 7002, "mal2", "mal2@example.com"), other.state); loc.Path != "/signup/complete" {
		t.Fatalf("matching state and cookie: %s", loc)
	}
	// An empty state never matches an empty cookie.
	if loc := r.callbackRaw(t, r.browserWith(""), "github", "code=x", nil); errorOf(loc) != "oauth_state_invalid" {
		t.Fatalf("empty state: %s", loc)
	}
}

func TestSocialStateIsBoundToItsProvider(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.e.browser().stopRedirects()
	f := r.start(t, c, "github", "")
	loc := r.callback(t, c, "google", r.google.Code(googleGrant(f, "g-9", "x@gmail.com")), f.state)
	if got := errorOf(loc); got != "oauth_state_invalid" {
		t.Fatalf("a GitHub state at the Google callback: %q", got)
	}
	if r.flowUsed(f.state) {
		t.Fatal("the wrong provider's callback consumed the GitHub flow")
	}
	if loc := r.callback(t, c, "github", r.githubCode(f, 8001, "ok", "ok@example.com"), f.state); loc.Path != "/signup/complete" {
		t.Fatalf("the right provider still works: %s", loc)
	}
}

func TestSocialExpiredStateAndTicket(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	r := newSocialRig(t, withClock(clk))
	c := r.e.browser().stopRedirects()

	f := r.start(t, c, "github", "")
	clk.set(clk.Now().Add(10*time.Minute + time.Second))
	if got := errorOf(r.callback(t, c, "github", r.githubCode(f, 9001, "late", "late@example.com"), f.state)); got != "oauth_state_invalid" {
		t.Fatalf("expired state: %q", got)
	}

	f = r.start(t, c, "github", "")
	if loc := r.callback(t, c, "github", r.githubCode(f, 9001, "late", "late@example.com"), f.state); loc.Path != "/signup/complete" {
		t.Fatalf("fresh state: %s", loc)
	}
	clk.set(clk.Now().Add(10*time.Minute + time.Second))
	c.must("GET", apiOAuth+"pending", nil, 404)
	if res := c.do("POST", apiOAuth+"complete", map[string]any{"accept_terms": true}); res.status != 404 {
		t.Fatalf("expired ticket: %d %s", res.status, res.body)
	}
	if n := r.countRows(`SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("users: %d", n)
	}
	// The worker's purge removes what has expired and nothing else.
	r.start(t, c, "github", "")
	if n, err := r.e.app.Services.Auth.PurgeExpiredOAuthFlows(context.Background()); err != nil || n != 2 {
		t.Fatalf("purge removed %d (%v), want the 2 expired flows", n, err)
	}
	if r.countRows(`SELECT count(*) FROM auth_oauth_flows`) != 1 {
		t.Fatal("the live flow must survive the purge")
	}
}

func TestSocialNextCannotLeaveTheWebApp(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	r.githubSignUp(t, 1101, "nina", "nina@example.com")
	for _, next := range []string{"https://evil.test/", "//evil.test", "/\\evil.test", "javascript:alert(1)", "/ok\r\nSet-Cookie: x=y", "http://evil.test@web.test/", "/a?u=https://evil.test"} {
		c := r.e.browser()
		loc := r.githubSignIn(t, c, 1101, "nina", "nina@example.com", next)
		if loc.Host != "web.test" || loc.Path != "/dashboard" {
			t.Fatalf("next=%q led to %s", next, loc)
		}
	}
	// An in-app path is kept, query included.
	if loc := r.githubSignIn(t, r.e.browser(), 1101, "nina", "nina@example.com", "/posts?tab=drafts"); loc.Path != "/posts" || loc.RawQuery != "tab=drafts" {
		t.Fatalf("in-app next: %s", loc)
	}
}

// The redirect URI sent to the provider comes from API_PUBLIC_URL, whatever Host the request claims.
func TestSocialRedirectURIIgnoresTheHostHeader(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	req, _ := http.NewRequest("GET", r.e.srv.URL+apiOAuth+"github/start", nil)
	req.Host = "evil.test"
	req.Header.Set("X-Forwarded-Host", "evil.test")
	req.Header.Set("X-Forwarded-Proto", "https")
	c := r.e.browser().stopRedirects()
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := url.Parse(res.Header.Get("Location"))
	if got, want := loc.Query().Get("redirect_uri"), r.e.srv.URL+"/api/v1/auth/oauth/github/callback"; got != want {
		t.Fatalf("redirect_uri %q, want %q", got, want)
	}
}

func TestSocialGoogleNonceAndIssuerAreEnforced(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	for name, mutate := range map[string]func(*oidcfake.Grant){
		"nonce of another login": func(g *oidcfake.Grant) { g.Nonce = "someone-elses-nonce" },
		"wrong audience":         func(g *oidcfake.Grant) { g.Fault = oidcfake.FaultWrongAudience },
		"expired token":          func(g *oidcfake.Grant) { g.Fault = oidcfake.FaultExpired },
		"unsigned token":         func(g *oidcfake.Grant) { g.Fault = oidcfake.FaultAlgNone },
		"wrong pkce challenge":   func(g *oidcfake.Grant) { g.CodeChallenge = "not-the-challenge" },
	} {
		c := r.e.browser().stopRedirects()
		f := r.start(t, c, "google", "")
		g := googleGrant(f, "g-"+name, "gus@gmail.com")
		mutate(&g)
		loc := r.callback(t, c, "google", r.google.Code(g), f.state)
		if got := errorOf(loc); got != "oauth_provider_error" || hasSessionCookie(c, r.e) {
			t.Fatalf("%s: got %q", name, got)
		}
	}
	// The user pressing "Cancel" at the provider ends the flow with its own code.
	c := r.e.browser().stopRedirects()
	f := r.start(t, c, "google", "")
	if got := errorOf(r.callbackRaw(t, c, "google", "error=access_denied&state="+url.QueryEscape(f.state), nil)); got != "oauth_cancelled" {
		t.Fatalf("access_denied: %q", got)
	}
	// ...but only with a valid state: a forged cancel for a stranger's flow says nothing.
	if got := errorOf(r.callbackRaw(t, r.browserWith(""), "google", "error=access_denied&state=guess", nil)); got != "oauth_state_invalid" {
		t.Fatalf("forged cancel: %q", got)
	}
	// A provider outage is a provider error, never a 5xx page.
	f = r.start(t, c, "google", "")
	g := googleGrant(f, "g-out", "gus@gmail.com")
	g.Fault = oidcfake.FaultTokenEndpoint
	if got := errorOf(r.callback(t, c, "google", r.google.Code(g), f.state)); got != "oauth_provider_error" {
		t.Fatalf("token endpoint down: %q", got)
	}
}

func TestSocialRefusesDisabledAndDeletingUsers(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := r.e.app.DB.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	r.githubSignUp(t, 1201, "dora", "dora@example.com")
	signIn := func() (*url.URL, *client) {
		c := r.e.browser()
		return r.githubSignIn(t, c, 1201, "dora", "dora@example.com", ""), c
	}

	for name, set := range map[string]string{
		"disabled": `UPDATE users SET status = 'disabled' WHERE email = 'dora@example.com'`,
		"deleted":  `UPDATE users SET status = 'deleted' WHERE email = 'dora@example.com'`,
	} {
		exec(set)
		loc, c := signIn()
		if got := errorOf(loc); got != "account_unavailable" || hasSessionCookie(c, r.e) {
			t.Fatalf("%s: got %q", name, got)
		}
	}

	// A scheduled deletion does not lock a password-less owner out: they sign in and can cancel. Their session cannot
	// schedule or publish, same as a password sign-in.
	exec(`UPDATE users SET status = 'active', deletion_scheduled_at = now() WHERE email = 'dora@example.com'`)
	loc, c := signIn()
	if errorOf(loc) != "" || !hasSessionCookie(c, r.e) {
		t.Fatalf("deletion scheduled: %s", loc)
	}
	me := c.must("GET", "/api/v1/me", nil, 200)
	c.csrf, _ = me["csrf_token"].(string)
	c.must("POST", "/api/v1/account/delete/cancel", nil, 204)

	// Back to normal: the same sign-in works again.
	exec(`UPDATE users SET status = 'active', deletion_scheduled_at = NULL WHERE email = 'dora@example.com'`)
	if loc, _ := signIn(); errorOf(loc) != "" {
		t.Fatalf("active again: %s", loc)
	}

	// Auto-link never reaches a disabled local account either.
	vic := r.e.browser()
	vic.register("lina@example.com")
	exec(`UPDATE users SET email_verified_at = now(), status = 'disabled' WHERE email = 'lina@example.com'`)
	c = r.e.browser()
	if got := errorOf(r.githubSignIn(t, c, 1202, "lina", "lina@example.com", "")); got != "account_unavailable" {
		t.Fatalf("disabled local account: %q", got)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE subject = '1202'`); n != 0 {
		t.Fatal("linked to a disabled account")
	}
}

func TestSocialCompleteRefusesWhenTheWorldChangedMeanwhile(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	ctx := context.Background()
	pending := func(id int64, login, email string) *client {
		c := r.e.browser()
		if loc := r.githubSignIn(t, c, id, login, email, ""); loc.Path != "/signup/complete" {
			t.Fatalf("setup: %s", loc)
		}
		return c
	}
	accept := map[string]any{"display_name": "X", "accept_terms": true}
	count := func(sql string) int { return r.countRows(sql) }

	// The address was registered with a password while the Terms form was open.
	c := pending(1301, "ella", "ella@example.com")
	r.e.browser().register("ella@example.com")
	if res := c.do("POST", apiOAuth+"complete", accept); res.status != 409 {
		t.Fatalf("email taken: %d %s", res.status, res.body)
	}
	if hasSessionCookie(c, r.e) || count(`SELECT count(*) FROM user_identities`) != 0 {
		t.Fatal("a refused completion issued a session or an identity")
	}
	// A failed completion does not burn the ticket.
	c.must("GET", apiOAuth+"pending", nil, 200)

	// The address belongs to a disabled account: still refused, never signed in.
	c = pending(1302, "finn", "finn@example.com")
	r.e.browser().register("finn@example.com")
	if _, err := r.e.app.DB.Pool.Exec(ctx, `UPDATE users SET status = 'disabled' WHERE email = 'finn@example.com'`); err != nil {
		t.Fatal(err)
	}
	if res := c.do("POST", apiOAuth+"complete", accept); res.status != 409 || hasSessionCookie(c, r.e) {
		t.Fatalf("disabled owner of the address: %d %s", res.status, res.body)
	}

	// Another browser finished the same provider account first: the second completion conflicts instead of making a twin.
	a, b := pending(1303, "gabe", "gabe@example.com"), pending(1303, "gabe", "gabe@example.com")
	a.must("POST", apiOAuth+"complete", accept, 201)
	if res := b.do("POST", apiOAuth+"complete", accept); res.status != 409 {
		t.Fatalf("twin completion: %d %s", res.status, res.body)
	}
	if n := count(`SELECT count(*) FROM users WHERE email = 'gabe@example.com'`); n != 1 {
		t.Fatalf("users: %d", n)
	}

	// ...and when that provider account already belongs to a leaving account it is a conflict too, never a sign-in.
	d := pending(1304, "hana", "hana@example.com")
	if _, err := r.e.app.DB.Pool.Exec(ctx, `INSERT INTO users (email, display_name, status, deletion_scheduled_at) VALUES ('other@example.com', '', 'active', now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.e.app.DB.Pool.Exec(ctx, `INSERT INTO user_identities (user_id, provider, subject, linked_at) SELECT id, 'github', '1304', now() FROM users WHERE email = 'other@example.com'`); err != nil {
		t.Fatal(err)
	}
	if res := d.do("POST", apiOAuth+"complete", accept); res.status != 409 || hasSessionCookie(d, r.e) {
		t.Fatalf("identity of a deleting account: %d %s", res.status, res.body)
	}
}

// One ticket, many parallel completions: exactly one account.
func TestSocialTicketWorksExactlyOnceUnderRace(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.e.browser()
	r.githubSignIn(t, c, 1401, "ivy", "ivy@example.com", "")
	const n = 8
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = r.replayAt(c).do("POST", apiOAuth+"complete", map[string]any{"display_name": "Ivy", "accept_terms": true}).status
		}()
	}
	wg.Wait()
	created, gone := 0, 0
	for _, s := range statuses {
		switch s {
		case 201:
			created++
		case 404:
			gone++
		}
	}
	if created != 1 || gone != n-1 {
		t.Fatalf("statuses %v", statuses)
	}
	if got := r.countRows(`SELECT count(*) FROM users`); got != 1 {
		t.Fatalf("users: %d", got)
	}
}

func TestSocialLimitersCoverStartCallbackAndComplete(t *testing.T) {
	limited := func(c *config.Config) { c.AuthRateRPS, c.AuthRateBurst = 0.0001, 2 }
	for name, hit := range map[string]func(r *socialRig, c *client) int{
		"start": func(_ *socialRig, c *client) int { return c.do("GET", apiOAuth+"github/start", nil).status },
		"callback": func(_ *socialRig, c *client) int {
			return c.do("GET", apiOAuth+"github/callback?code=x&state=y", nil).status
		},
		"complete": func(_ *socialRig, c *client) int {
			return c.do("POST", apiOAuth+"complete", map[string]any{"accept_terms": true}).status
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newSocialRig(t, envOpts{mutate: limited})
			c := r.e.browser().stopRedirects()
			for i := range 2 {
				if s := hit(r, c); s == http.StatusTooManyRequests {
					t.Fatalf("request %d was limited too early", i+1)
				}
			}
			if s := hit(r, c); s != http.StatusTooManyRequests {
				t.Fatalf("third request: %d, want 429", s)
			}
		})
	}
}
