package e2e

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/config"
)

const goodPassword = "correct horse battery"

// enforce turns on email verification the way production does: a real mail provider.
func enforce(c *config.Config) { c.MailProvider = config.MailProviderSMTP }

// waitMail blocks until the worker delivered n messages in total.
func (c *captureMailer) waitMail(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for c.count() < n && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if c.count() < n {
		t.Fatalf("expected %d mails, got %d", n, c.count())
	}
}

// last returns the newest delivered message of a template.
func (c *captureMailer) last(t *testing.T, template string) port.Message {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.got) - 1; i >= 0; i-- {
		if c.got[i].Template == template {
			return c.got[i]
		}
	}
	t.Fatalf("no %s mail delivered", template)
	return port.Message{}
}

func (c *captureMailer) templates() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, m := range c.got {
		out = append(out, m.Template+"→"+m.To)
	}
	return out
}

// token extracts the raw token from the fragment link of a mail.
func token(t *testing.T, m port.Message) string {
	t.Helper()
	_, after, ok := strings.Cut(m.Text, "http://web.test/")
	if !ok || !strings.Contains(after, "#token=") || strings.Contains(m.Text, "?token=") {
		t.Fatalf("mail must link with a #token= fragment: %q", m.Text)
	}
	_, tok, _ := strings.Cut(after, "#token=")
	return strings.Fields(tok)[0]
}

func (c *client) login(email, password string) resp {
	c.e.t.Helper()
	r := c.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": password})
	if r.status == http.StatusOK {
		c.csrf, _ = r.json(c.e.t)["csrf_token"].(string)
	}
	return r
}

func userField(t *testing.T, r resp, key string) any {
	t.Helper()
	u, _ := r.json(t)["user"].(map[string]any)
	return u[key]
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestUnverifiedOwnerIsGatedWhereMailWorks(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm, mutate: enforce})
	c := e.browser()
	c.register("gate@example.com")
	cm.waitMail(t, 1)
	if m := cm.last(t, "verify_email"); m.To != "gate@example.com" {
		t.Fatalf("verification mail to %q", m.To)
	}

	me := c.must("GET", "/api/v1/me", nil, 200)
	if me["verification_enforced"] != true || me["mail_delivery"] != "smtp" ||
		me["user"].(map[string]any)["email_verified"] != false || me["user"].(map[string]any)["plan"] != "free" {
		t.Fatalf("/me: %v", me)
	}

	forbidden := func(method, path string, body any) {
		t.Helper()
		r := c.do(method, path, body)
		if r.status != http.StatusForbidden || r.errCode(t) != "EMAIL_NOT_VERIFIED" {
			t.Errorf("%s %s: want 403 EMAIL_NOT_VERIFIED, got %d %s", method, path, r.status, r.body)
		}
	}
	someID := uuid.NewString()
	forbidden("GET", "/api/v1/social/mock/connect?format=json", nil)
	forbidden("POST", "/api/v1/social/telegram/connect", nil)
	forbidden("POST", "/api/v1/developer/api-keys", map[string]any{"name": "k", "scopes": []string{"posts:read"}})
	forbidden("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": "m"})
	forbidden("POST", "/api/v1/posts", map[string]any{"content": "x", "schedule": true, "scheduled_at": fmtTime(time.Now().Add(time.Hour))})
	forbidden("POST", "/api/v1/posts/"+someID+"/schedule", map[string]any{"scheduled_at": fmtTime(time.Now().Add(time.Hour))})
	forbidden("POST", "/api/v1/posts/"+someID+"/publish", map[string]any{"confirm": true})
	forbidden("POST", "/api/v1/posts/"+someID+"/retry", map[string]any{})
	// Drafts and reading stay available.
	c.must("POST", "/api/v1/posts", map[string]any{"content": "a draft"}, 201)
	c.must("GET", "/api/v1/posts", nil, 200)

	// Verify with the mailed link; afterwards everything above works.
	raw := token(t, cm.last(t, "verify_email"))
	out := c.must("POST", "/api/v1/auth/verify-email", map[string]any{"token": raw}, 200)
	if out["email_verified"] != true {
		t.Fatalf("verify body: %v", out)
	}
	if r := c.do("POST", "/api/v1/auth/verify-email", map[string]any{"token": raw}); r.status != 400 || r.errCode(t) != "VALIDATION_ERROR" {
		t.Fatalf("reused link: %d %s", r.status, r.body)
	}
	if got := userField(t, c.do("GET", "/api/v1/me", nil), "email_verified"); got != true {
		t.Fatalf("email_verified after verify: %v", got)
	}
	c.connectMock()
	c.createKey("agent", "posts:read")
	if r := c.do("POST", "/api/v1/auth/verify-email/resend", nil); r.status != http.StatusConflict {
		t.Fatalf("resend when verified: %d %s", r.status, r.body)
	}
	if got := auditActions(t, c)["user.email_verified"]; got != 1 {
		t.Fatalf("audit user.email_verified x%d", got)
	}
}

func TestVerificationMailCanBeResentOncePerMinute(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm, mutate: enforce})
	c := e.browser()
	c.register("resend@example.com")
	cm.waitMail(t, 1)
	first := token(t, cm.last(t, "verify_email"))

	if r := c.do("POST", "/api/v1/auth/verify-email/resend", nil); r.status != 429 {
		t.Fatalf("resend inside the cooldown: %d %s", r.status, r.body)
	}
	if r := e.apiKeyClient("sk_live_x").do("POST", "/api/v1/auth/verify-email/resend", nil); r.status != 401 {
		t.Fatalf("anonymous resend: %d", r.status)
	}
	if r := e.browser().do("POST", "/api/v1/auth/verify-email/resend", nil); r.status != 401 {
		t.Fatalf("anonymous resend: %d", r.status)
	}
	if cm.count() != 1 || first == "" {
		t.Fatalf("mails: %v", cm.templates())
	}
}

func TestNothingIsGatedWithoutMailDelivery(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm}) // MAIL_PROVIDER=log
	c := e.browser()
	c.register("nolog@example.com")
	me := c.must("GET", "/api/v1/me", nil, 200)
	if me["verification_enforced"] != false || me["mail_delivery"] != "log" || me["user"].(map[string]any)["email_verified"] != false {
		t.Fatalf("/me: %v", me)
	}
	c.connectMock()
	c.createKey("agent", "posts:read")
	cm.waitMail(t, 1) // the link is still issued, so a developer can read it in the log
}

func TestPasswordResetEndToEnd(t *testing.T) {
	cm, logs := &captureMailer{}, &lockedBuf{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm, mutate: enforce,
		logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	b1 := e.browser()
	b1.register("reset@example.com")
	b2 := e.browser()
	b2.login("reset@example.com", goodPassword)
	b1.must("GET", "/api/v1/me", nil, 200)
	b2.must("GET", "/api/v1/me", nil, 200)
	cm.waitMail(t, 1)

	anon := e.browser()
	r := anon.do("POST", "/api/v1/auth/password/forgot", map[string]any{"email": "Reset@Example.com"})
	if r.status != 202 || r.json(t)["status"] != "accepted" || r.json(t)["delivery"] != "smtp" {
		t.Fatalf("forgot: %d %s", r.status, r.body)
	}
	cm.waitMail(t, 2)
	reset := cm.last(t, "reset_password")
	raw := token(t, reset)
	if !strings.HasPrefix(reset.Text[strings.Index(reset.Text, "http"):], "http://web.test/reset-password#token=") {
		t.Fatalf("reset link: %q", reset.Text)
	}

	// A weak password is rejected without burning the link.
	if r := anon.do("POST", "/api/v1/auth/password/reset", map[string]any{"token": raw, "password": "short"}); r.status != 400 {
		t.Fatalf("weak password: %d %s", r.status, r.body)
	}
	if r := anon.do("POST", "/api/v1/auth/password/reset", map[string]any{"token": raw + "x", "password": "a brand new password"}); r.status != 400 {
		t.Fatalf("wrong token: %d %s", r.status, r.body)
	}
	if r := anon.do("POST", "/api/v1/auth/password/reset", map[string]any{"token": raw, "password": "a brand new password"}); r.status != 204 {
		t.Fatalf("reset: %d %s", r.status, r.body)
	}
	cm.waitMail(t, 3)
	if cm.last(t, "password_changed").To != "reset@example.com" {
		t.Fatal("no password-changed notice")
	}

	// Every session is gone; only the new password works; the link is spent.
	for i, b := range []*client{b1, b2} {
		if r := b.do("GET", "/api/v1/me", nil); r.status != 401 {
			t.Fatalf("session %d survived the reset: %d", i+1, r.status)
		}
	}
	if r := e.browser().login("reset@example.com", goodPassword); r.status != 401 {
		t.Fatalf("old password still works: %d", r.status)
	}
	fresh := e.browser()
	if r := fresh.login("reset@example.com", "a brand new password"); r.status != 200 {
		t.Fatalf("new password: %d %s", r.status, r.body)
	}
	if r := anon.do("POST", "/api/v1/auth/password/reset", map[string]any{"token": raw, "password": "yet another password"}); r.status != 400 {
		t.Fatalf("reused reset link: %d", r.status)
	}
	// The link proved control of the address.
	if got := userField(t, fresh.do("GET", "/api/v1/me", nil), "email_verified"); got != true {
		t.Fatalf("email_verified after reset: %v", got)
	}

	// Secrets never reach the audit log or the process log.
	audit := string(fresh.do("GET", "/api/v1/audit-logs?limit=100", nil).body)
	if !strings.Contains(audit, "user.password_reset") {
		t.Fatalf("no reset audit entry: %s", audit)
	}
	for _, secret := range []string{raw, token(t, cm.last(t, "verify_email")), "a brand new password", goodPassword} {
		if strings.Contains(audit, secret) || strings.Contains(logs.String(), secret) {
			t.Fatalf("a secret leaked into the audit log or process log")
		}
	}
}

func TestForgotPasswordLooksTheSameForEveryAddress(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm})
	e.browser().register("known@example.com")
	cm.waitMail(t, 1)
	anon := e.browser()
	forgot := func(email string) (resp, time.Duration) {
		start := time.Now()
		r := anon.do("POST", "/api/v1/auth/password/forgot", map[string]any{"email": email})
		return r, time.Since(start)
	}
	known, kd := forgot("known@example.com")
	unknown, ud := forgot("nobody@example.com")
	malformed, _ := forgot("not-an-email")
	empty, _ := forgot("")
	for name, r := range map[string]resp{"unknown": unknown, "malformed": malformed, "empty": empty} {
		if r.status != known.status || !bytes.Equal(r.body, known.body) {
			t.Errorf("%s differs from known: %d %s vs %d %s", name, r.status, r.body, known.status, known.body)
		}
	}
	if known.status != 202 || string(bytes.TrimSpace(known.body)) != `{"delivery":"log","status":"accepted"}` {
		t.Fatalf("body: %d %s", known.status, known.body)
	}
	// Both only enqueue; neither waits for SMTP. A generous bound keeps this stable on busy CI.
	if d := kd - ud; d > 250*time.Millisecond || d < -250*time.Millisecond {
		t.Errorf("response times differ too much: known %v unknown %v", kd, ud)
	}
	cm.waitMail(t, 2)
	time.Sleep(200 * time.Millisecond)
	if got := cm.templates(); !slices.Equal(got, []string{"verify_email→known@example.com", "reset_password→known@example.com"}) {
		t.Fatalf("mails sent: %v", got)
	}
}

func TestChangePasswordKeepsOnlyTheCurrentSession(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm})
	cur := e.browser()
	cur.register("change@example.com")
	other := e.browser()
	other.login("change@example.com", goodPassword)

	bad := cur.do("POST", "/api/v1/auth/password/change", map[string]any{"current_password": "wrong", "new_password": "a brand new password"})
	if bad.status != 400 || bad.errCode(t) != "VALIDATION_ERROR" {
		t.Fatalf("wrong current password: %d %s", bad.status, bad.body)
	}
	other.must("GET", "/api/v1/me", nil, 200) // a failed attempt revokes nothing
	if r := e.apiKeyClient("sk_live_x").do("POST", "/api/v1/auth/password/change", map[string]any{}); r.status != 401 {
		t.Fatalf("anonymous change: %d", r.status)
	}
	csrf := cur.csrf
	cur.csrf = ""
	if r := cur.do("POST", "/api/v1/auth/password/change", map[string]any{"current_password": goodPassword, "new_password": "a brand new password"}); r.status != 403 {
		t.Fatalf("change without CSRF: %d", r.status)
	}
	cur.csrf = csrf
	cur.must("POST", "/api/v1/auth/password/change", map[string]any{"current_password": goodPassword, "new_password": "a brand new password"}, 204)

	cur.must("GET", "/api/v1/me", nil, 200)
	if r := other.do("GET", "/api/v1/me", nil); r.status != 401 {
		t.Fatalf("other session survived: %d", r.status)
	}
	if r := e.browser().login("change@example.com", goodPassword); r.status != 401 {
		t.Fatalf("old password works: %d", r.status)
	}
	if r := e.browser().login("change@example.com", "a brand new password"); r.status != 200 {
		t.Fatalf("new password: %d", r.status)
	}
	cm.waitMail(t, 2)
	if cm.last(t, "password_changed").To != "change@example.com" {
		t.Fatal("no notice")
	}
	if got := auditActions(t, cur)["user.password_changed"]; got != 1 {
		t.Fatalf("audit x%d", got)
	}
}

func TestMailEndpointsAreRateLimited(t *testing.T) {
	e := newEnv(t, envOpts{realMailLimit: true})
	anon := e.browser()
	var statuses []int
	for i := 0; i < 5; i++ {
		statuses = append(statuses, anon.do("POST", "/api/v1/auth/password/forgot", map[string]any{"email": fmt.Sprintf("u%d@example.com", i)}).status)
	}
	if !slices.Equal(statuses, []int{202, 202, 202, 429, 429}) {
		t.Fatalf("statuses %v", statuses)
	}
}
