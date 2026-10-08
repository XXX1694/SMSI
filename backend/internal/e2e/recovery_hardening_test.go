package e2e

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/application/port"
)

// failMailer records every attempt and always fails, like a relay that is down for good.
type failMailer struct {
	mu  sync.Mutex
	got []port.Message
}

func (f *failMailer) Send(_ context.Context, m port.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, m)
	return errors.New("smtp relay down")
}

func (f *failMailer) attempts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.got)
}

// When the last retry fails, the task is archived with its rendered body. The link in it must be dead.
func TestUndeliveredMailLeavesNoLiveLink(t *testing.T) {
	fm := &failMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: fm, mutate: enforce,
		retryDelay: func(int, error) time.Duration { return 20 * time.Millisecond }})
	c := e.browser()
	c.register("undelivered@example.com")

	deadline := time.Now().Add(20 * time.Second)
	for fm.attempts() < 6 && time.Now().Before(deadline) { // the first try plus 5 retries
		time.Sleep(50 * time.Millisecond)
	}
	if fm.attempts() != 6 {
		t.Fatalf("attempts: %d", fm.attempts())
	}
	time.Sleep(300 * time.Millisecond) // the final attempt retires the token before it returns
	raw := token(t, fm.got[0])
	if r := c.do("POST", "/api/v1/auth/verify-email", map[string]any{"token": raw}); r.status != http.StatusBadRequest {
		t.Fatalf("the archived mail still holds a live link: %d %s", r.status, r.body)
	}
	if got := userField(t, c.do("GET", "/api/v1/me", nil), "email_verified"); got != false {
		t.Fatalf("email_verified: %v", got)
	}
}

func TestPasswordChangeRevokesKeysOnlyWhenAsked(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm})
	c := e.browser()
	c.register("keys-pw@example.com")
	key := e.apiKeyClient(c.createKey("agent", "posts:read"))
	mcp := e.apiKeyClient(c.must("POST", "/api/v1/developer/mcp-connections", map[string]any{"name": "Claude"}, 201)["key"].(string))
	key.must("GET", "/api/v1/posts", nil, 200)

	c.must("POST", "/api/v1/auth/password/change", map[string]any{"current_password": goodPassword, "new_password": "second password 22"}, 204)
	key.must("GET", "/api/v1/posts", nil, 200)
	mcp.must("GET", "/api/v1/posts", nil, 200)
	cm.waitMail(t, 2)
	if m := cm.last(t, "password_changed"); !contains(m.Text, "were not revoked") || !contains(m.Text, "http://web.test/developer") {
		t.Fatalf("notice must say the keys survived and link to /developer: %q", m.Text)
	}

	c.must("POST", "/api/v1/auth/password/change", map[string]any{"current_password": "second password 22", "new_password": "third password 333", "revoke_keys": true}, 204)
	for name, k := range map[string]*client{"api key": key, "mcp connection": mcp} {
		if r := k.do("GET", "/api/v1/posts", nil); r.status != 401 {
			t.Errorf("%s survived revoke_keys: %d", name, r.status)
		}
	}
	list := c.must("GET", "/api/v1/developer/mcp-connections", nil, 200)["items"].([]any)
	if list[0].(map[string]any)["revoked_at"] == nil {
		t.Fatalf("connection not marked revoked: %v", list[0])
	}
	cm.waitMail(t, 3)
	if m := cm.last(t, "password_changed"); !contains(m.Text, "were revoked too") {
		t.Fatalf("notice: %q", m.Text)
	}
}

func TestPasswordResetRevokesKeysOnlyWhenAsked(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		cm := &captureMailer{}
		e := newEnv(t, envOpts{startWorker: true, mailer: cm})
		c := e.browser()
		c.register("reset-keys@example.com")
		key := e.apiKeyClient(c.createKey("agent", "posts:read"))
		cm.waitMail(t, 1)
		e.browser().must("POST", "/api/v1/auth/password/forgot", map[string]any{"email": "reset-keys@example.com"}, 202)
		cm.waitMail(t, 2)
		raw := token(t, cm.last(t, "reset_password"))
		e.browser().must("POST", "/api/v1/auth/password/reset", map[string]any{"token": raw, "password": "a brand new password", "revoke_keys": revoke}, 204)
		want := 200
		if revoke {
			want = 401
		}
		if r := key.do("GET", "/api/v1/posts", nil); r.status != want {
			t.Fatalf("revoke_keys=%v: key status %d, want %d", revoke, r.status, want)
		}
		if got := auditActions(t, e.loginAs("reset-keys@example.com", "a brand new password"))["user.password_reset"]; got != 1 {
			t.Fatalf("audit x%d", got)
		}
	}
}

// Verification switched on after a user already had scheduled work: editing a scheduled post is gated too.
func TestEditingAScheduledPostNeedsAVerifiedOwner(t *testing.T) {
	cm := &captureMailer{}
	e := newEnv(t, envOpts{startWorker: true, mailer: cm, mutate: enforce})
	c := e.browser()
	c.register("edit@example.com")
	cm.waitMail(t, 1)
	c.must("POST", "/api/v1/auth/verify-email", map[string]any{"token": token(t, cm.last(t, "verify_email"))}, 200)
	acc := c.connectMock()
	p := c.must("POST", "/api/v1/posts", map[string]any{"content": "later", "social_account_ids": []string{acc},
		"schedule": true, "scheduled_at": fmtTime(time.Now().Add(48 * time.Hour))}, 201)
	id := p["id"].(string)
	c.must("PATCH", "/api/v1/posts/"+id, map[string]any{"content": "edited while verified"}, 200)

	if _, err := e.app.DB.Pool.Exec(context.Background(), `UPDATE users SET email_verified_at = NULL WHERE email = 'edit@example.com'`); err != nil {
		t.Fatal(err)
	}
	if r := c.do("PATCH", "/api/v1/posts/"+id, map[string]any{"content": "edited unverified"}); r.status != 403 || r.errCode(t) != "EMAIL_NOT_VERIFIED" {
		t.Fatalf("scheduled post edited by an unverified owner: %d %s", r.status, r.body)
	}
	// Drafts stay editable.
	d := c.must("POST", "/api/v1/posts", map[string]any{"content": "draft"}, 201)
	c.must("PATCH", "/api/v1/posts/"+d["id"].(string), map[string]any{"content": "draft edited"}, 200)
}
