package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/application/port"

	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/domain/identity"
	"github.com/socialos/backend/internal/infrastructure/postgres"
)

const (
	identitiesPath = "/api/v1/auth/identities"
	passwordSet    = "/api/v1/auth/password/set"
)

// startLink asks to connect a provider as the signed-in client c and returns what the provider was told.
func (r *socialRig) startLink(t *testing.T, c *client, provider string) flow {
	t.Helper()
	return r.startLinkWith(t, c, provider, nil)
}

// startLinkWith is startLink for a user with a password, who confirms it in the body.
func (r *socialRig) startLinkWith(t *testing.T, c *client, provider string, body map[string]any) flow {
	t.Helper()
	m := c.must("POST", identitiesPath+"/"+provider+"/link", body, 200)
	raw, _ := m["authorize_url"].(string)
	return parseAuthorize(t, provider, raw)
}

// connectGoogle links a Google account to the signed-in client and returns the callback redirect.
func (r *socialRig) connectGoogle(t *testing.T, c *client, subject string) *url.URL {
	t.Helper()
	f := r.startLink(t, c, "google")
	return r.callback(t, c, "google", r.google.Code(googleGrant(f, subject, subject+"@gmail.example")), f.state)
}

func providersOf(m map[string]any) []string {
	out := []string{}
	for _, i := range m["identities"].([]any) {
		out = append(out, i.(map[string]any)["provider"].(string))
	}
	return out
}

func methodsOf(me map[string]any) string {
	var out []string
	for _, m := range me["user"].(map[string]any)["login_methods"].([]any) {
		out = append(out, m.(string))
	}
	return strings.Join(out, ",")
}

func TestSignInMethodsListLinkAndUnlink(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.githubSignUp(t, 2201, "ivy", "ivy@example.com")

	list := c.must("GET", identitiesPath, nil, 200)
	if list["has_password"] != false || strings.Join(providersOf(list), ",") != "github" {
		t.Fatalf("list: %v", list)
	}
	if first := list["identities"].([]any)[0].(map[string]any); first["email"] != "ivy@example.com" || first["linked_at"] == nil || first["subject"] != nil {
		t.Fatalf("identity entry: %v", first)
	}
	me := c.must("GET", "/api/v1/me", nil, 200)
	if u := me["user"].(map[string]any); u["has_password"] != false || methodsOf(me) != "github" {
		t.Fatalf("/me: %v", u)
	}

	// The only way to sign in cannot be removed.
	res := c.do("DELETE", identitiesPath+"/github", nil)
	if res.status != 409 || res.errCode(t) != "CONFLICT" || !strings.Contains(res.errBody(t)["message"].(string), "keep at least one way to sign in") {
		t.Fatalf("last method: %d %s", res.status, res.body)
	}

	// Connect Google: the callback is the sign-in one, it returns to Settings and keeps the session.
	if loc := r.connectGoogle(t, c, "g-2201"); loc.Path != "/settings" || loc.RawQuery != "" {
		t.Fatalf("link redirect: %s", loc)
	}
	list = c.must("GET", identitiesPath, nil, 200)
	if got := strings.Join(providersOf(list), ","); got != "github,google" {
		t.Fatalf("after link: %s", got)
	}
	if methodsOf(c.must("GET", "/api/v1/me", nil, 200)) != "github,google" {
		t.Fatal("/me login_methods after link")
	}
	if meta := r.auditMeta(t, "user.identity_linked"); !strings.Contains(meta, `"provider": "google"`) || !strings.Contains(meta, `"via": "settings"`) || strings.Contains(meta, "@") {
		t.Fatalf("link audit: %s", meta)
	}
	if n := r.countRows(`SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("linking created users: %d", n)
	}
	// One account per provider.
	if res := c.do("POST", identitiesPath+"/google/link", nil); res.status != 409 {
		t.Fatalf("second google: %d %s", res.status, res.body)
	}

	// With two providers either can go, but not both.
	c.must("DELETE", identitiesPath+"/github", nil, 204)
	if got := strings.Join(providersOf(c.must("GET", identitiesPath, nil, 200)), ","); got != "google" {
		t.Fatalf("after unlink: %s", got)
	}
	if meta := r.auditMeta(t, "user.identity_unlinked"); meta != `{"provider": "github"}` {
		t.Fatalf("unlink audit: %s", meta)
	}
	c.must("DELETE", identitiesPath+"/github", nil, 404)
	c.must("DELETE", identitiesPath+"/myspace", nil, 404)
	if res := c.do("DELETE", identitiesPath+"/google", nil); res.status != 409 {
		t.Fatalf("last provider: %d %s", res.status, res.body)
	}

	// A password is another way in, so the last provider can then be removed.
	c.must("POST", passwordSet, map[string]any{"new_password": "a brand new password"}, 204)
	c.must("DELETE", identitiesPath+"/google", map[string]any{"current_password": "a brand new password"}, 204)
	list = c.must("GET", identitiesPath, nil, 200)
	if list["has_password"] != true || len(providersOf(list)) != 0 {
		t.Fatalf("password only: %v", list)
	}
	if methodsOf(c.must("GET", "/api/v1/me", nil, 200)) != "password" {
		t.Fatal("/me login_methods with a password only")
	}
}

// The link flow is the sign-in callback with intent=link, and it finishes only for the user who started it.
func TestSignInLinkIsBoundToTheStartingUser(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.githubSignUp(t, 2301, "jo", "jo@example.com")
	f := r.startLink(t, c, "google")
	code := r.google.Code(googleGrant(f, "g-2301", "jo@gmail.example"))

	// Another user signs in on the same browser before the provider answers: the link is refused and spent.
	other := r.e.browser()
	other.register("other-jo@example.com")
	c.must("POST", "/api/v1/auth/logout", nil, 204)
	if res := c.login("other-jo@example.com", "correct horse battery"); res.status != 200 {
		t.Fatalf("login: %d %s", res.status, res.body)
	}
	loc := r.callback(t, c, "google", code, f.state)
	if loc.Path != "/settings" || loc.Query().Get("error") != "oauth_state_invalid" || loc.Query().Get("provider") != "google" {
		t.Fatalf("hijacked link: %s", loc)
	}
	// Signed out, the same flow cannot be completed either (and it was already spent).
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE provider = 'google'`); n != 0 {
		t.Fatalf("identities: %d", n)
	}

	// An anonymous browser holding a link state gets the same refusal.
	f = r.startLinkWith(t, c, "google", map[string]any{"current_password": "correct horse battery"})
	code = r.google.Code(googleGrant(f, "g-2301", "jo@gmail.example"))
	c.must("POST", "/api/v1/auth/logout", nil, 204)
	if loc := r.callback(t, c, "google", code, f.state); loc.Query().Get("error") != "oauth_state_invalid" {
		t.Fatalf("anonymous link: %s", loc)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE provider = 'google'`); n != 0 {
		t.Fatalf("identities: %d", n)
	}
}

// Changing how the account can be entered needs proof it is the owner: a password, or a fresh session when there is none.
func TestSignInMethodChangesNeedReauthentication(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Now().UTC())
	r := newSocialRig(t, withClock(clk))
	c := r.githubSignUp(t, 2901, "oli", "oli@example.com")
	r.connectGoogle(t, c, "g-2901")

	// Password-less: an old session is refused for link and unlink, and nothing changes.
	clk.set(clk.Now().Add(10*time.Minute + time.Second))
	for _, tc := range []struct{ method, path string }{{"POST", identitiesPath + "/github/link"}, {"DELETE", identitiesPath + "/github"}} {
		if res := c.do(tc.method, tc.path, nil); res.status != 403 || res.errCode(t) != "REAUTH_REQUIRED" {
			t.Fatalf("stale %s %s: %d %s", tc.method, tc.path, res.status, res.body)
		}
	}
	if got := strings.Join(providersOf(c.must("GET", identitiesPath, nil, 200)), ","); got != "github,google" {
		t.Fatalf("a refused request changed identities: %s", got)
	}
	fresh := r.e.browser()
	if loc := r.githubSignIn(t, fresh, 2901, "oli", "oli@example.com", ""); errorOf(loc) != "" {
		t.Fatalf("sign in again: %s", loc)
	}
	fresh.csrf, _ = fresh.must("GET", "/api/v1/me", nil, 200)["csrf_token"].(string)
	fresh.must("DELETE", identitiesPath+"/google", nil, 204)

	// With a password, any session age needs the current password.
	pw := r.e.browser()
	pw.register("pw-oli@example.com")
	clk.set(clk.Now().Add(time.Hour))
	for _, body := range []map[string]any{nil, {"current_password": "wrong password!!"}} {
		for _, tc := range []struct{ method, path string }{{"POST", identitiesPath + "/github/link"}, {"DELETE", identitiesPath + "/github"}} {
			res := pw.do(tc.method, tc.path, body)
			if res.status != 400 || res.errBody(t)["fields"].(map[string]any)["current_password"] == nil {
				t.Fatalf("%s %s with %v: %d %s", tc.method, tc.path, body, res.status, res.body)
			}
		}
	}
	pw.must("POST", identitiesPath+"/github/link", map[string]any{"current_password": "correct horse battery"}, 200)
	pw.must("DELETE", identitiesPath+"/github", map[string]any{"current_password": "correct horse battery"}, 404)
}

func TestSignInMethodEndpointsNeedASessionAndCSRF(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.githubSignUp(t, 2401, "kit", "kit@example.com")
	anon := r.e.browser()
	token := c.csrf

	for _, tc := range []struct{ method, path string }{
		{"GET", identitiesPath}, {"POST", identitiesPath + "/google/link"}, {"DELETE", identitiesPath + "/github"}, {"POST", passwordSet},
	} {
		if res := anon.do(tc.method, tc.path, nil); res.status != 401 {
			t.Errorf("anonymous %s %s: %d", tc.method, tc.path, res.status)
		}
		key := r.e.apiKeyClient(c.createKey("agent", "posts:read"))
		if res := key.do(tc.method, tc.path, nil); res.status != 403 {
			t.Errorf("api key %s %s: %d %s", tc.method, tc.path, res.status, res.body)
		}
	}
	for _, csrf := range []string{"", "wrong"} {
		c.csrf = csrf
		for _, tc := range []struct{ method, path string }{
			{"POST", identitiesPath + "/google/link"}, {"DELETE", identitiesPath + "/github"}, {"POST", passwordSet},
		} {
			if res := c.do(tc.method, tc.path, map[string]any{"new_password": "a brand new password"}); res.status != 403 || res.errCode(t) != "FORBIDDEN" {
				t.Errorf("csrf %q %s %s: %d %s", csrf, tc.method, tc.path, res.status, res.body)
			}
		}
	}
	c.csrf = token
	if n := r.countRows(`SELECT count(*) FROM user_identities WHERE user_id IS NOT NULL`); n != 1 {
		t.Fatalf("a refused request changed identities: %d", n)
	}
	if n := r.countRows(`SELECT count(*) FROM users WHERE password_hash IS NOT NULL`); n != 0 {
		t.Fatal("a refused request set a password")
	}
	// A provider that is switched off cannot be connected.
	c.must("POST", identitiesPath+"/myspace/link", nil, 404)
}

func TestSignInMethodLimitersCoverLinkAndUnlink(t *testing.T) {
	limited := func(c *config.Config) { c.AuthRateRPS, c.AuthRateBurst = 0.0001, 2 }
	for name, hit := range map[string]func(c *client) int{
		"link":   func(c *client) int { return c.do("POST", identitiesPath+"/google/link", nil).status },
		"unlink": func(c *client) int { return c.do("DELETE", identitiesPath+"/github", nil).status },
		"set": func(c *client) int {
			return c.do("POST", passwordSet, map[string]any{"new_password": "a brand new password"}).status
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newSocialRig(t, envOpts{mutate: limited})
			c := r.e.browser()
			c.register("limited-" + name + "@example.com")
			for i := range 2 {
				if s := hit(c); s == 429 {
					t.Fatalf("request %d was limited too early", i+1)
				}
			}
			if s := hit(c); s != 429 {
				t.Fatalf("third request: %d, want 429", s)
			}
		})
	}
}

func TestSetPasswordNeedsAFreshSessionAndNoPassword(t *testing.T) {
	clk := &steppingClock{}
	clk.set(time.Now().UTC())
	r := newSocialRig(t, withClock(clk))
	c := r.githubSignUp(t, 2501, "lux", "lux@example.com")

	// A password is 8 to 128 characters, like everywhere else.
	if res := c.do("POST", passwordSet, map[string]any{"new_password": "short"}); res.status != 400 {
		t.Fatalf("short password: %d %s", res.status, res.body)
	}
	// A session older than 10 minutes must sign in again first; nothing changes meanwhile.
	clk.set(clk.Now().Add(10*time.Minute + time.Second))
	res := c.do("POST", passwordSet, map[string]any{"new_password": "a brand new password"})
	if res.status != 403 || res.errCode(t) != "REAUTH_REQUIRED" {
		t.Fatalf("stale session: %d %s", res.status, res.body)
	}
	if n := r.countRows(`SELECT count(*) FROM users WHERE password_hash IS NOT NULL`); n != 0 {
		t.Fatal("a refused request set a password")
	}

	// Signing in again with the provider makes the session fresh.
	fresh := r.e.browser()
	if loc := r.githubSignIn(t, fresh, 2501, "lux", "lux@example.com", ""); errorOf(loc) != "" {
		t.Fatalf("sign in again: %s", loc)
	}
	fresh.csrf, _ = fresh.must("GET", "/api/v1/me", nil, 200)["csrf_token"].(string)
	fresh.must("POST", passwordSet, map[string]any{"new_password": "a brand new password"}, 204)
	if meta := r.auditMeta(t, "user.password_changed"); !strings.Contains(meta, `"initial": true`) {
		t.Fatalf("audit: %s", meta)
	}
	// The other (stale) session is signed out, and the password now works.
	if res := c.do("GET", "/api/v1/me", nil); res.status != 401 {
		t.Fatalf("old session after setting a password: %d", res.status)
	}
	if res := r.e.browser().login("lux@example.com", "a brand new password"); res.status != 200 {
		t.Fatalf("password login: %d %s", res.status, res.body)
	}
	// Once there is a password, this endpoint is closed: changing it needs the current one.
	if res := fresh.do("POST", passwordSet, map[string]any{"new_password": "another new password"}); res.status != 409 {
		t.Fatalf("second set: %d %s", res.status, res.body)
	}

	// An account that registered with a password is refused outright.
	pw := r.e.browser()
	pw.register("has-pw@example.com")
	if res := pw.do("POST", passwordSet, map[string]any{"new_password": "another new password"}); res.status != 409 || res.errCode(t) != "CONFLICT" {
		t.Fatalf("password account: %d %s", res.status, res.body)
	}
}

func TestMeReportsLoginMethodsOnEveryAuthBody(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	pw := r.e.browser()
	reg := pw.register("both@example.com")
	if u := reg["user"].(map[string]any); u["has_password"] != true || methodsOf(reg) != "password" {
		t.Fatalf("register body: %v", u)
	}
	if methodsOf(pw.must("GET", "/api/v1/me", nil, 200)) != "password" {
		t.Fatal("/me for a password account")
	}
	login := r.e.browser().login("both@example.com", "correct horse battery")
	if methodsOf(login.json(t)) != "password" {
		t.Fatalf("login body: %s", login.body)
	}
	// Auto-link is not possible for an unverified account, so link from Settings instead.
	pw.csrf, _ = pw.must("GET", "/api/v1/me", nil, 200)["csrf_token"].(string)
	f := r.startLinkWith(t, pw, "github", map[string]any{"current_password": "correct horse battery"})
	if loc := r.callback(t, pw, "github", r.githubCode(f, 2601, "both", "elsewhere@example.com"), f.state); loc.Path != "/settings" || loc.RawQuery != "" {
		t.Fatalf("link from settings: %s", loc)
	}
	if methodsOf(pw.must("GET", "/api/v1/me", nil, 200)) != "password,github" {
		t.Fatal("/me after linking")
	}
	// The provider's email differs from the account's and plays no part; the account's email is unchanged.
	if me := pw.must("GET", "/api/v1/me", nil, 200); me["user"].(map[string]any)["email"] != "both@example.com" {
		t.Fatalf("email changed: %v", me["user"])
	}
	// Signing in with the linked provider now reaches the same account.
	again := r.e.browser()
	if loc := r.githubSignIn(t, again, 2601, "both", "elsewhere@example.com", ""); errorOf(loc) != "" {
		t.Fatalf("sign in with the linked provider: %s", loc)
	}
	if methodsOf(again.must("GET", "/api/v1/me", nil, 200)) != "password,github" {
		t.Fatal("same account expected")
	}
}

func TestSignInMethodChangesMailTheOwner(t *testing.T) {
	cm := &captureMailer{}
	r := newSocialRig(t, envOpts{startWorker: true, mailer: cm})
	c := r.githubSignUp(t, 2701, "mia", "mia@example.com")
	r.connectGoogle(t, c, "g-2701")
	c.must("DELETE", identitiesPath+"/github", nil, 204)

	wait := func(template string) port.Message {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			cm.mu.Lock()
			for _, m := range cm.got {
				if m.Template == template {
					cm.mu.Unlock()
					return m
				}
			}
			cm.mu.Unlock()
		}
		t.Fatalf("no %s mail", template)
		return port.Message{}
	}
	linked, unlinked := wait("identity_linked"), wait("identity_unlinked")
	if linked.To != "mia@example.com" || !strings.Contains(linked.Text, "Google") || !strings.Contains(linked.Text, "Settings → Sign-in methods, disconnect Google and change your password") {
		t.Fatalf("link notice: %+v", linked)
	}
	if unlinked.To != "mia@example.com" || !strings.Contains(unlinked.Text, "GitHub") {
		t.Fatalf("unlink notice: %+v", unlinked)
	}
}

// An account waiting for deletion gains no new sign-in method, but can still remove one.
func TestAccountWaitingForDeletionCannotLinkANewProvider(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.githubSignUp(t, 2801, "nia", "nia@example.com")
	if res := c.deleteAccount("", "nia@example.com"); res.status != 200 && res.status != 202 {
		t.Fatalf("schedule deletion: %d %s", res.status, res.body)
	}
	// Scheduling revokes every session; the owner signs in again to reach the cancel banner.
	c = r.e.browser()
	if loc := r.githubSignIn(t, c, 2801, "nia", "nia@example.com", ""); errorOf(loc) != "" {
		t.Fatalf("sign in while deleting: %s", loc)
	}
	c.csrf, _ = c.must("GET", "/api/v1/me", nil, 200)["csrf_token"].(string)
	if res := c.do("POST", identitiesPath+"/google/link", nil); res.status != 409 {
		t.Fatalf("link while deleting: %d %s", res.status, res.body)
	}
}

// A failed provider sign-in tells the page which provider it was and keeps the (sanitised) destination.
func TestFailedSignInRedirectNamesTheProviderAndKeepsNext(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	r.e.browser().register("exists@example.com") // unverified: the email match is refused
	loc := r.githubSignIn(t, r.e.browser(), 3001, "ex", "exists@example.com", "/posts?draft=1")
	if q := loc.Query(); loc.Path != "/login" || q.Get("error") != "account_exists" || q.Get("provider") != "github" || q.Get("next") != "/posts?draft=1" {
		t.Fatalf("redirect: %s", loc)
	}
	loc = r.githubSignIn(t, r.e.browser(), 3001, "ex", "exists@example.com", "https://evil.test/")
	if q := loc.Query(); q.Get("next") != "/dashboard" || q.Get("provider") != "github" {
		t.Fatalf("unsafe next must be replaced: %s", loc)
	}
	// A cancelled consent screen too.
	c := r.e.browser()
	f := r.start(t, c, "github", "/posts")
	loc = r.callbackRaw(t, c, "github", "error=access_denied&state="+url.QueryEscape(f.state), nil)
	if q := loc.Query(); q.Get("error") != "oauth_cancelled" || q.Get("provider") != "github" || q.Get("next") != "/posts" {
		t.Fatalf("cancelled: %s", loc)
	}
}

// Two unlinks racing for the last two providers of a password-less user: exactly one wins.
func TestRacingUnlinksKeepOneWayToSignIn(t *testing.T) {
	r := newSocialRig(t, envOpts{})
	c := r.githubSignUp(t, 3101, "ray", "ray@example.com")
	r.connectGoogle(t, c, "g-3101")
	statuses := make(chan int, 2)
	for _, p := range []string{"github", "google"} {
		go func() { statuses <- c.do("DELETE", identitiesPath+"/"+p, nil).status }()
	}
	got := []int{<-statuses, <-statuses}
	if got[0]+got[1] != 204+409 || got[0] == got[1] {
		t.Fatalf("statuses %v, want one 204 and one 409", got)
	}
	if n := r.countRows(`SELECT count(*) FROM user_identities`); n != 1 {
		t.Fatalf("identities left: %d", n)
	}
}

// Switching a provider off does not hide or trap the identities already linked to it.
func TestLinkedIdentitiesSurviveADisabledProvider(t *testing.T) {
	e := newEnv(t, envOpts{}) // no sign-in provider configured
	c := e.browser()
	uid := uuid.MustParse(c.register("off@example.com")["user"].(map[string]any)["id"].(string))
	err := postgres.NewIdentities(e.app.DB).Create(context.Background(), &identity.Identity{UserID: uid, Provider: identity.GitHub, Subject: "3201", LinkedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if got := providersOf(c.must("GET", identitiesPath, nil, 200)); len(got) != 1 || got[0] != "github" {
		t.Fatalf("list: %v", got)
	}
	c.must("DELETE", identitiesPath+"/github", map[string]any{"current_password": "correct horse battery"}, 204)
	c.must("POST", identitiesPath+"/github/link", map[string]any{"current_password": "correct horse battery"}, 404)
}

// A linked identity whose provider is switched off cannot sign anyone in, so it does not count as the way that remains.
func TestADisabledProviderIsNotAWayToSignIn(t *testing.T) {
	r := newGitHubOnlyRig(t, envOpts{})
	c := r.githubSignUp(t, 3301, "una", "una@example.com")
	var uid uuid.UUID
	if err := r.e.app.DB.Pool.QueryRow(context.Background(), `SELECT user_id FROM user_identities WHERE subject = '3301'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	err := postgres.NewIdentities(r.e.app.DB).Create(context.Background(), &identity.Identity{UserID: uid, Provider: identity.Google, Subject: "g-3301", LinkedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	c.must("DELETE", identitiesPath+"/github", nil, 409)
	c.must("DELETE", identitiesPath+"/google", nil, 204)
	if got := providersOf(c.must("GET", identitiesPath, nil, 200)); len(got) != 1 || got[0] != "github" {
		t.Fatalf("identities left: %v", got)
	}
}

// The data export lists the provider accounts a user signs in with, and says whether a password is set.
func TestExportListsSignInMethods(t *testing.T) {
	r := newSocialRig(t, envOpts{startWorker: true})
	c := r.githubSignUp(t, 3501, "wes", "wes@example.com")
	uid := c.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"].(string)
	id := c.must("POST", "/api/v1/account/exports", nil, 202)["id"].(string)
	c.waitExport(id, "ready")
	files := r.e.zipOf(uid, id)

	var methods []map[string]any
	if err := json.Unmarshal(files["sign_in_methods.json"], &methods); err != nil || len(methods) != 1 {
		t.Fatalf("sign_in_methods.json: %v %s", err, files["sign_in_methods.json"])
	}
	if m := methods[0]; m["provider"] != "github" || m["provider_account_id"] != "3501" || m["email"] != "wes@example.com" || m["linked_at"] == nil {
		t.Fatalf("method: %v", m)
	}
	// Another user's archive holds none of it.
	other := r.githubSignUp(t, 3502, "xan", "xan@example.com")
	oid := other.must("GET", "/api/v1/me", nil, 200)["user"].(map[string]any)["id"].(string)
	oexp := other.must("POST", "/api/v1/account/exports", nil, 202)["id"].(string)
	other.waitExport(oexp, "ready")
	if theirs := r.e.zipOf(oid, oexp)["sign_in_methods.json"]; !bytes.Contains(theirs, []byte(`"3502"`)) || bytes.Contains(theirs, []byte("3501")) || bytes.Contains(theirs, []byte("wes@")) {
		t.Fatalf("another user's export lists wes: %s", theirs)
	}
	var profile map[string]any
	if err := json.Unmarshal(files["profile.json"], &profile); err != nil || profile["has_password"] != false {
		t.Fatalf("profile.json: %v %s", err, files["profile.json"])
	}
}
