package e2e

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/infrastructure/crypto"
	"github.com/socialos/backend/internal/testutil/githubfake"
)

func TestSocialProvidersAreListedOnlyWhenConfigured(t *testing.T) {
	off := newEnv(t, envOpts{})
	c := off.browser()
	list := c.must("GET", "/api/v1/auth/providers", nil, 200)["providers"].([]any)
	if len(list) != 0 {
		t.Fatalf("no credentials, no providers: %v", list)
	}
	if r := c.do("GET", apiOAuth+"github/start", nil); r.status != 404 || r.errCode(t) != "NOT_FOUND" {
		t.Fatalf("a disabled provider cannot be started: %d %s", r.status, r.body)
	}
	res := c.stopRedirects().do("GET", apiOAuth+"github/callback?code=x&state=y", nil)
	if loc, _ := url.Parse(res.header.Get("Location")); res.status != http.StatusFound || errorOf(loc) != "oauth_provider_error" {
		t.Fatalf("the callback of a disabled provider still redirects to the web app with an error: %d %s", res.status, res.header.Get("Location"))
	}

	r := newSocialRig(t, envOpts{})
	list = r.e.browser().must("GET", "/api/v1/auth/providers", nil, 200)["providers"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["id"] != "google" || list[1].(map[string]any)["name"] != "GitHub" {
		t.Fatalf("providers: %v", list)
	}
	if r := r.e.browser().do("GET", apiOAuth+"myspace/start", nil); r.status != 404 {
		t.Fatalf("unknown provider: %d", r.status)
	}
}

// New user: start, callback to the Terms form, complete, /me.
func TestSocialNewUserCompletesSignUp(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	ctx := context.Background()
	c := r.e.browser().stopRedirects()

	f := r.start(t, c, "github", "/posts?draft=1")
	ck := f.setCookie
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/api/v1/auth/oauth" || ck.MaxAge != 600 || ck.Value != f.state {
		t.Fatalf("state cookie: %+v", ck)
	}
	if want := r.e.srv.URL + "/api/v1/auth/oauth/github/callback"; f.redirectURI != want {
		t.Fatalf("redirect_uri %q, want %q", f.redirectURI, want)
	}
	// At rest: only hashes, and the PKCE verifier is encrypted with the app cipher.
	var stateHash, verifierEnc string
	if err := r.e.app.DB.Pool.QueryRow(ctx, `SELECT state_hash, code_verifier_enc FROM auth_oauth_flows`).Scan(&stateHash, &verifierEnc); err != nil {
		t.Fatal(err)
	}
	if stateHash != crypto.SHA256Hex(f.state) || !strings.HasPrefix(verifierEnc, "v1:") {
		t.Fatalf("flow row: state_hash=%q verifier=%q", stateHash, verifierEnc)
	}
	if strings.Contains(verifierEnc, f.state) {
		t.Fatal("the verifier column must be ciphertext")
	}

	loc := r.callback(t, c, "github", r.githubCode(f, 1001, "gina", "Gina@Example.com"), f.state)
	if loc.Path != "/signup/complete" || loc.RawQuery != "" {
		t.Fatalf("redirect: %s", loc)
	}
	if hasSessionCookie(c, r.e) {
		t.Fatal("no session before the Terms are accepted")
	}
	if n := r.countRows(`SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("no user row before the Terms: %d", n)
	}
	if n := r.countRows(`SELECT count(*) FROM auth_oauth_flows WHERE ticket_hash IS NOT NULL AND ticket_hash <> ''`); n != 1 {
		t.Fatalf("one sign-up ticket expected, got %d", n)
	}

	pending := c.must("GET", apiOAuth+"pending", nil, 200)
	if pending["email"] != "gina@example.com" || pending["provider"] != "github" || pending["display_name"] != "Gina gina" || pending["next"] != "/posts?draft=1" {
		t.Fatalf("pending: %v", pending)
	}

	// Without the Terms the account is refused and the ticket survives, so the form can be corrected.
	for _, body := range []map[string]any{{"display_name": "Gina"}, {"display_name": "Gina", "accept_terms": false}} {
		res := c.do("POST", apiOAuth+"complete", body)
		if res.status != 400 || res.errBody(t)["fields"].(map[string]any)["accept_terms"] == nil {
			t.Fatalf("accept_terms must be required: %d %s", res.status, res.body)
		}
	}
	c.must("GET", apiOAuth+"pending", nil, 200)
	if n := r.countRows(`SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("a refused completion created %d users", n)
	}
	// A form post (cross-site style) is not JSON and is refused.
	req, _ := http.NewRequest("POST", r.e.srv.URL+apiOAuth+"complete", strings.NewReader("accept_terms=true"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if res, err := c.http.Do(req); err != nil || res.StatusCode != 400 {
		t.Fatalf("form post: %v %v", res, err)
	}

	m := c.must("POST", apiOAuth+"complete", map[string]any{"display_name": "  Gina G  ", "accept_terms": true}, 201)
	c.csrf, _ = m["csrf_token"].(string)
	user := m["user"].(map[string]any)
	if user["email"] != "gina@example.com" || user["display_name"] != "Gina G" || user["email_verified"] != true {
		t.Fatalf("created user: %v", user)
	}
	me := c.must("GET", "/api/v1/me", nil, 200)
	if me["user"].(map[string]any)["id"] != user["id"] {
		t.Fatalf("/me: %v", me)
	}
	var hashNull bool
	var termsVersion string
	if err := r.e.app.DB.Pool.QueryRow(ctx, `SELECT password_hash IS NULL, terms_version FROM users`).Scan(&hashNull, &termsVersion); err != nil || !hashNull || termsVersion == "" {
		t.Fatalf("password_hash NULL=%v terms_version=%q err=%v", hashNull, termsVersion, err)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE provider = 'github' AND subject = '1001'`); n != 1 {
		t.Fatalf("identity rows: %d", n)
	}
	// The ticket is spent: the form cannot be replayed, and the cookie was cleared.
	c.must("GET", apiOAuth+"pending", nil, 404)
	if res := c.do("POST", apiOAuth+"complete", map[string]any{"accept_terms": true}); res.status != 404 {
		t.Fatalf("replayed completion: %d %s", res.status, res.body)
	}
	if n := r.countRows(`SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("users: %d", n)
	}
	if meta := r.auditMeta(t, "user.registered"); meta != `{"method": "github"}` {
		t.Fatalf("audit metadata: %s", meta)
	}
}

func (r *socialRig) auditMeta(t *testing.T, action string) string {
	t.Helper()
	var meta string
	if err := r.e.app.DB.Pool.QueryRow(context.Background(), `SELECT metadata::text FROM audit_logs WHERE action = $1 ORDER BY created_at DESC LIMIT 1`, action).Scan(&meta); err != nil {
		t.Fatalf("audit %s: %v", action, err)
	}
	return meta
}

func TestSocialReturningUserSignsInAndFollowsNext(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	first := r.githubSignUp(t, 2002, "rita", "rita@example.com")
	meID := first.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"]

	c := r.e.browser()
	loc := r.githubSignIn(t, c, 2002, "rita", "rita@example.com", "/posts/new")
	if loc.Path != "/posts/new" || errorOf(loc) != "" {
		t.Fatalf("redirect: %s", loc)
	}
	if got := c.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"]; got != meID {
		t.Fatalf("signed in as %v, want %v", got, meID)
	}
	if meta := r.auditMeta(t, "user.login"); meta != `{"method": "github"}` {
		t.Fatalf("audit metadata: %s", meta)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE last_login_at IS NOT NULL`); n != 1 {
		t.Fatalf("last_login_at not recorded: %d", n)
	}
	// Without a next the default landing page is used; the old password flow still refuses this account.
	if loc := r.githubSignIn(t, r.e.browser(), 2002, "rita", "rita@example.com", ""); loc.Path != "/dashboard" {
		t.Fatalf("default redirect: %s", loc)
	}
	if res := r.e.browser().do("POST", "/api/v1/auth/login", map[string]any{"email": "rita@example.com", "password": "correct horse battery"}); res.status != 401 {
		t.Fatalf("a social-only account has no password: %d", res.status)
	}
}

func TestSocialLinksVerifiedLocalAccountAndRefusesUnverifiedOne(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	ctx := context.Background()

	verified := r.e.browser()
	verified.register("verified@example.com")
	if _, err := r.e.app.DB.Pool.Exec(ctx, `UPDATE users SET email_verified_at = now() WHERE email = 'verified@example.com'`); err != nil {
		t.Fatal(err)
	}
	uid := verified.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"]

	c := r.e.browser()
	loc := r.githubSignIn(t, c, 3003, "vera", "verified@example.com", "")
	if loc.Path != "/dashboard" || errorOf(loc) != "" {
		t.Fatalf("auto-link redirect: %s", loc)
	}
	if got := c.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"]; got != uid {
		t.Fatalf("linked sign-in is user %v, want %v", got, uid)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE user_id = $1`, uid); n != 1 {
		t.Fatalf("identity rows: %d", n)
	}
	if meta := r.auditMeta(t, "user.identity_linked"); strings.Contains(meta, "@") || !strings.Contains(meta, `"provider": "github"`) {
		t.Fatalf("audit metadata: %s", meta)
	}

	// An unverified password account is never linked (pre-hijacking): the person is told to sign in with the password.
	r.e.browser().register("squatter@example.com")
	c2 := r.e.browser()
	loc = r.githubSignIn(t, c2, 4004, "victim", "squatter@example.com", "")
	if errorOf(loc) != "account_exists" || hasSessionCookie(c2, r.e) {
		t.Fatalf("unverified local account: %s", loc)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE subject = '4004'`); n != 0 {
		t.Fatalf("an identity was linked to an unverified account")
	}
	if n := r.countRows(`SELECT count(*) FROM auth_oauth_flows WHERE ticket_hash IS NOT NULL AND ticket_hash <> ''`); n != 0 {
		t.Fatal("a refused flow must not leave a sign-up ticket")
	}
}

func TestSocialProviderWithoutAVerifiedAddressIsRefused(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.e.browser()
	f := r.start(t, c, "github", "")
	code := r.gh.Code(&githubfake.Account{ID: 5005, Login: "noverify", PKCEChallenge: f.challenge,
		Emails: []githubfake.Email{{Email: "nv@example.com", Primary: true, Verified: false}}})
	if got := errorOf(r.callback(t, c, "github", code, f.state)); got != "email_unverified" {
		t.Fatalf("got %q", got)
	}
	// Google through the fake issuer is not accounts.google.com, so its email is never authoritative either.
	f = r.start(t, c, "google", "")
	code = r.google.Code(googleGrant(f, "g-1", "someone@gmail.com"))
	if got := errorOf(r.callback(t, c, "google", code, f.state)); got != "email_unverified" {
		t.Fatalf("got %q", got)
	}
	if n := r.countRows(`SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("users: %d", n)
	}
}
