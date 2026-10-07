package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/telegram"
	"github.com/socialos/backend/internal/config"
)

const (
	chanX = int64(-1009876543210) // the channel user A owns
	chanY = int64(-1001111111111)
	chanZ = int64(-1002222222222)
	grp   = int64(-1003333333333)

	webhookSecret = "whsec_0123456789abcdef0123456789abcdef"
)

var codeRe = regexp.MustCompile(`^SOS-[A-HJKMNP-Z2-9]{8}$`)

// tgEnv is an environment whose Telegram provider talks to a fake Bot API.
func tgEnv(t *testing.T, o envOpts) (*env, *fakeTelegram) {
	t.Helper()
	bot := newFakeTelegram(t)
	tg := telegram.New(telegram.Config{BotToken: fakeBotToken, APIBaseURL: bot.srv.URL})
	o.providers = append(o.providers, tg)
	return newEnv(t, o), bot
}

// deliver hands a raw update to the application method both intake modes call.
func (e *env) deliver(t *testing.T, u fakeUpdate) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return e.app.Services.Accounts.HandleChatUpdate(ctx, "telegram", []byte(u.raw))
}

// startLink starts a Telegram link and returns its id and code.
func (c *client) startLink() (id, code string) {
	c.e.t.Helper()
	m := c.must("POST", "/api/v1/social/telegram/connect", nil, 201)
	id, _ = m["id"].(string)
	code, _ = m["code"].(string)
	if id == "" || !codeRe.MatchString(code) {
		c.e.t.Fatalf("link start: %v", m)
	}
	return id, code
}

func (c *client) linkStatus(id string) map[string]any {
	c.e.t.Helper()
	return c.must("GET", "/api/v1/social/telegram/connect/"+id, nil, 200)
}

func (c *client) waitLink(id, want string, timeout time.Duration) map[string]any {
	c.e.t.Helper()
	deadline := time.Now().Add(timeout)
	var last map[string]any
	for time.Now().Before(deadline) {
		last = c.linkStatus(id)
		if last["status"] == want {
			return last
		}
		time.Sleep(25 * time.Millisecond)
	}
	c.e.t.Fatalf("link %s did not reach %q: %v", id, want, last)
	return nil
}

func (c *client) accounts() []map[string]any {
	c.e.t.Helper()
	var out []map[string]any
	for _, it := range c.must("GET", "/api/v1/social/accounts", nil, 200)["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func (c *client) telegramAccounts() (out []map[string]any) {
	for _, a := range c.accounts() {
		if a["provider"] == "telegram" {
			out = append(out, a)
		}
	}
	return out
}

func accountIDs(accs []map[string]any) string {
	var ids []string
	for _, a := range accs {
		ids = append(ids, fmt.Sprint(a["provider_account_id"]))
	}
	return strings.Join(ids, ",")
}

// ---------------------------------------------------------------- ownership proof

// The bug being fixed: with one platform bot, anybody who knew a channel's
// @username could connect it to their own account. Now only the user whose code
// was posted in the channel gets it.
func TestTelegramLinkConnectsTheChannelToTheCodesOwnerOnly(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	a, b := e.browser(), e.browser()
	a.register("alice-tg@example.com")
	b.register("bob-tg@example.com")

	// Session only: an API key can neither start nor poll a link.
	k := e.apiKeyClient(a.createKey("tg key", "social:read", "posts:read", "posts:write", "posts:schedule", "posts:publish"))
	if r := k.do("POST", "/api/v1/social/telegram/connect", nil); r.status != 403 || r.errCode(t) != "FORBIDDEN" {
		t.Errorf("api key started a link: %d %s", r.status, r.body)
	}

	r := a.do("POST", "/api/v1/social/telegram/connect", nil)
	if r.status != 201 {
		t.Fatalf("start: %d %s", r.status, r.body)
	}
	start := r.json(t)
	linkID, code := start["id"].(string), start["code"].(string)
	if !codeRe.MatchString(code) || start["bot_username"] != "socialos_bot" {
		t.Fatalf("start response: %v", start)
	}
	if ins, _ := start["instructions"].(string); !strings.Contains(ins, code) || !strings.Contains(ins, "@socialos_bot") {
		t.Fatalf("instructions must name the bot and the code: %q", ins)
	}
	exp, err := time.Parse(time.RFC3339, start["expires_at"].(string))
	if err != nil || time.Until(exp) < 14*time.Minute || time.Until(exp) > 15*time.Minute+5*time.Second {
		t.Fatalf("expires_at %v (%v): want about 15 minutes ahead", start["expires_at"], err)
	}
	if k := k.do("GET", "/api/v1/social/telegram/connect/"+linkID, nil); k.status != 403 {
		t.Errorf("api key polled a link: %d", k.status)
	}

	// Only the hash is stored.
	var plain, hashed int
	ctx := context.Background()
	_ = e.app.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM telegram_link_codes WHERE code_hash = $1`, code).Scan(&plain)
	_ = e.app.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM telegram_link_codes WHERE id = $1 AND length(code_hash) = 64`, linkID).Scan(&hashed)
	if plain != 0 || hashed != 1 {
		t.Fatalf("the code must be stored as a hash only (plain=%d hashed=%d)", plain, hashed)
	}

	// Pending, and invisible to everybody else.
	if st := a.linkStatus(linkID); st["status"] != "pending" || st["account"] != nil {
		t.Fatalf("status: %v", st)
	}
	if r := b.do("GET", "/api/v1/social/telegram/connect/"+linkID, nil); r.status != 404 || r.errCode(t) != "NOT_FOUND" {
		t.Fatalf("bob polling alice's link must be 404, got %d %s", r.status, r.body)
	}
	if r := e.browser().do("GET", "/api/v1/social/telegram/connect/"+linkID, nil); r.status != 401 {
		t.Fatalf("anonymous polling: %d", r.status)
	}
	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if r := a.do("GET", "/api/v1/social/telegram/connect/"+id, nil); r.status != 404 {
			t.Errorf("link %s: %d", id, r.status)
		}
	}

	// Bob cannot connect X: no code, a made-up code, and the old direct request all fail.
	bobID, _ := b.startLink()
	for _, text := range []string{"hello", "SOS-AAAAAAAA", "@e2e_channel", "t.me/e2e_channel"} {
		if err := e.deliver(t, bot.channelPost(chanX, text)); err != nil {
			t.Fatal(err)
		}
	}
	// ...the legacy request that used to connect any channel by name now only mints a code.
	legacy := b.do("POST", "/api/v1/social/telegram/connect", map[string]any{"chat": "@e2e_channel"})
	if legacy.status != 201 || legacy.json(t)["code"] == nil {
		t.Fatalf("legacy chat parameter: %d %s", legacy.status, legacy.body)
	}
	if n := len(b.accounts()); n != 0 {
		t.Fatalf("bob connected %d accounts without proof: %v", n, b.accounts())
	}
	if bot.wasDeleted(chanX) {
		t.Fatal("nothing was linked, nothing may be deleted")
	}

	// Alice posts her code in X: spaces and lower case are tolerated.
	post := bot.channelPost(chanX, "  \n"+strings.ToLower(code)+" \n")
	if err := e.deliver(t, post); err != nil {
		t.Fatal(err)
	}
	st := a.linkStatus(linkID)
	acc, _ := st["account"].(map[string]any)
	if st["status"] != "connected" || acc == nil || acc["provider"] != "telegram" || acc["username"] != "e2e_channel" ||
		acc["display_name"] != "E2E Channel" || acc["status"] != "active" || acc["provider_account_id"] != strconv.FormatInt(chanX, 10) {
		t.Fatalf("connected status: %v", st)
	}
	if got := accountIDs(a.telegramAccounts()); got != strconv.FormatInt(chanX, 10) {
		t.Fatalf("alice accounts: %q", got)
	}
	if n := len(b.accounts()); n != 0 {
		t.Fatalf("bob must still have no accounts, has %d", n)
	}
	if r := b.do("GET", "/api/v1/social/accounts/"+acc["id"].(string), nil); r.status != 404 {
		t.Fatalf("bob reading alice's account: %d", r.status)
	}
	if st := b.linkStatus(bobID); st["status"] != "pending" {
		t.Fatalf("bob's own link stays pending: %v", st)
	}
	if !bot.wasDeleted(chanX) {
		t.Fatal("the message carrying the code must be deleted")
	}

	// Audit: written for alice, attributed to the system actor "telegram", without the code.
	var found bool
	for _, it := range a.must("GET", "/api/v1/audit-logs?limit=50", nil, 200)["items"].([]any) {
		m := it.(map[string]any)
		if strings.Contains(fmt.Sprint(m), code) {
			t.Fatalf("audit log leaks the code: %v", m)
		}
		if m["action"] == "social_account.connected" && m["actor_type"] == "system" && m["actor_id"] == "telegram" && m["actor_label"] == "telegram" {
			found = true
			meta := m["metadata"].(map[string]any)
			if meta["provider"] != "telegram" || meta["username"] != "e2e_channel" {
				t.Errorf("audit metadata: %v", meta)
			}
		}
	}
	if !found {
		t.Fatal("no social_account.connected audit entry by system/telegram")
	}
	for _, it := range b.must("GET", "/api/v1/audit-logs?limit=50", nil, 200)["items"].([]any) {
		if it.(map[string]any)["action"] == "social_account.connected" {
			t.Fatal("bob has a connect audit entry")
		}
	}

	// Posting the same code again (re-delivery or a replay) changes nothing.
	if err := e.deliver(t, post); err != nil {
		t.Fatal(err)
	}
	if err := e.deliver(t, bot.channelPost(chanX, code)); err != nil {
		t.Fatal(err)
	}
	if n := len(a.telegramAccounts()); n != 1 {
		t.Fatalf("replay duplicated the account: %d", n)
	}
	var audits int
	_ = e.app.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'social_account.connected'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("connect audited %d times", audits)
	}
}

func TestTelegramLinkExpiredReusedAndRetiredCodesDoNotConnect(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	bot.addChannel(chanY, "other_channel", "Other")
	a := e.browser()
	a.register("alice-exp@example.com")
	ctx := context.Background()

	// Expired.
	id, code := a.startLink()
	if _, err := e.app.DB.Pool.Exec(ctx, `UPDATE telegram_link_codes SET expires_at = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if st := a.linkStatus(id); st["status"] != "expired" {
		t.Fatalf("status of an expired link: %v", st)
	}
	if err := e.deliver(t, bot.channelPost(chanX, code)); err != nil {
		t.Fatal(err)
	}
	if n := len(a.telegramAccounts()); n != 0 || bot.wasDeleted(chanX) {
		t.Fatal("an expired code connected a channel")
	}
	if bot.count("getChat") != 0 {
		t.Fatal("an expired code must not even trigger platform calls")
	}

	// Reused: connect X once, then use the same code for Y and again for X.
	id2, code2 := a.startLink()
	if err := e.deliver(t, bot.channelPost(chanX, code2)); err != nil {
		t.Fatal(err)
	}
	a.waitLink(id2, "connected", 5*time.Second)
	if err := e.deliver(t, bot.channelPost(chanY, code2)); err != nil {
		t.Fatal(err)
	}
	if got := accountIDs(a.telegramAccounts()); got != strconv.FormatInt(chanX, 10) {
		t.Fatalf("a used code connected another channel: %q", got)
	}
	if bot.wasDeleted(chanY) {
		t.Fatal("a reused code's message must be left alone")
	}
	var rows int
	_ = e.app.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM telegram_link_codes WHERE id = $1 AND used_at IS NOT NULL AND chat_id = $2 AND social_account_id IS NOT NULL`,
		id2, strconv.FormatInt(chanX, 10)).Scan(&rows)
	if rows != 1 {
		t.Fatal("the code row must record when, where and to which account it was used")
	}

	// At most three active codes: the oldest is retired by the fourth.
	var ids, codes []string
	for i := 0; i < 4; i++ {
		id, code := a.startLink()
		ids, codes = append(ids, id), append(codes, code)
	}
	want := []string{"expired", "pending", "pending", "pending"}
	for i, id := range ids {
		if got := a.linkStatus(id)["status"]; got != want[i] {
			t.Errorf("link %d: status %v, want %s", i, got, want[i])
		}
	}
	if err := e.deliver(t, bot.channelPost(chanY, codes[0])); err != nil {
		t.Fatal(err)
	}
	if got := accountIDs(a.telegramAccounts()); got != strconv.FormatInt(chanX, 10) {
		t.Fatalf("a retired code connected a channel: %q", got)
	}
	if err := e.deliver(t, bot.channelPost(chanY, codes[3])); err != nil {
		t.Fatal(err)
	}
	if n := len(a.telegramAccounts()); n != 2 {
		t.Fatalf("the newest code must work: %d accounts", n)
	}
}

func TestTelegramLinkNeedsTheBotToHavePostRights(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	bot.addChannel(chanZ, "z_channel", "Z")
	a := e.browser()
	a.register("alice-rights@example.com")
	id, code := a.startLink()
	post := bot.channelPost(chanZ, code)

	// Member only; admin without "Post messages"; kicked.
	for name, set := range map[string]func(){
		"member":          func() { bot.setBot(chanZ, "member", false) },
		"admin, no post":  func() { bot.setBot(chanZ, "administrator", false) },
		"bot not in chat": func() { bot.setBot(chanZ, "left", false) },
	} {
		set()
		if err := e.deliver(t, post); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len(a.telegramAccounts()); n != 0 {
			t.Fatalf("%s: connected a channel the bot cannot post to", name)
		}
		if st := a.linkStatus(id); st["status"] != "pending" {
			t.Fatalf("%s: the code must not be burned, status %v", name, st)
		}
		if bot.wasDeleted(chanZ) {
			t.Fatalf("%s: message deleted without linking", name)
		}
	}
	// The user fixes the rights and posts the code again: it still works.
	bot.setBot(chanZ, "administrator", true)
	if err := e.deliver(t, post); err != nil {
		t.Fatal(err)
	}
	if st := a.linkStatus(id); st["status"] != "connected" {
		t.Fatalf("after fixing the rights: %v", st)
	}
}

func TestTelegramLinkInGroupsRequiresAnAdministratorSender(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	g := bot.addGroup(grp, "Team chat")
	g.Admins[8] = "administrator"
	g.Admins[9] = "creator"
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	a := e.browser()
	a.register("alice-grp@example.com")

	id, code := a.startLink()
	// A random member of somebody's group cannot attach the group to their own account.
	if err := e.deliver(t, bot.groupMessage(grp, 7, code)); err != nil {
		t.Fatal(err)
	}
	// Neither can a channel posting into the discussion group, nor an edit.
	if err := e.deliver(t, bot.groupMessageAsChannel(grp, chanX, code)); err != nil {
		t.Fatal(err)
	}
	if err := e.deliver(t, bot.edited(chanX, code)); err != nil {
		t.Fatal(err)
	}
	if st := a.linkStatus(id); st["status"] != "pending" || len(a.telegramAccounts()) != 0 || bot.wasDeleted(grp) {
		t.Fatalf("a non-admin linked the group: %v", st)
	}
	// An admin can, in the open...
	if err := e.deliver(t, bot.groupMessage(grp, 8, code)); err != nil {
		t.Fatal(err)
	}
	st := a.waitLink(id, "connected", 5*time.Second)
	if acc := st["account"].(map[string]any); acc["display_name"] != "Team chat" || acc["provider_account_id"] != strconv.FormatInt(grp, 10) {
		t.Fatalf("group account: %v", st)
	}
	if !bot.wasDeleted(grp) {
		t.Fatal("the code message must be deleted from the group")
	}
	// ...and an anonymous admin (posting as the group) can too.
	b := e.browser()
	b.register("bob-grp@example.com")
	bot.addGroup(int64(-1004444444444), "Anon admins")
	id2, code2 := b.startLink()
	if err := e.deliver(t, bot.anonymousAdminMessage(-1004444444444, code2)); err != nil {
		t.Fatal(err)
	}
	b.waitLink(id2, "connected", 5*time.Second)
}

func TestTelegramLinkTransientFailureIsRetriedByRedelivery(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	a := e.browser()
	a.register("alice-retry@example.com")
	id, code := a.startLink()
	post := bot.channelPost(chanX, code)
	bot.mu.Lock()
	bot.failGetChat = 1
	bot.mu.Unlock()
	if err := e.deliver(t, post); err == nil {
		t.Fatal("a platform outage must be reported to the intake so it can retry")
	}
	if a.linkStatus(id)["status"] != "pending" {
		t.Fatal("a failed attempt must leave the code usable")
	}
	if err := e.deliver(t, post); err != nil {
		t.Fatal(err)
	}
	a.waitLink(id, "connected", 5*time.Second)
}

func TestTelegramLinkStartIsRateLimited(t *testing.T) {
	e, _ := tgEnv(t, envOpts{mutate: func(c *config.Config) { c.AuthRateRPS, c.AuthRateBurst = 0.001, 3 }})
	a := e.browser()
	a.register("alice-rl@example.com") // takes one token of the auth bucket; the link bucket is separate
	var codes []int
	for i := 0; i < 5; i++ {
		r := a.do("POST", "/api/v1/social/telegram/connect", nil)
		codes = append(codes, r.status)
		if r.status == 429 && (r.errCode(t) != "RATE_LIMITED" || r.header.Get("Retry-After") == "") {
			t.Fatalf("429 envelope: %s", r.body)
		}
	}
	if fmt.Sprint(codes) != "[201 201 201 429 429]" {
		t.Fatalf("link starts: %v", codes)
	}
	// Polling is not throttled by the same bucket.
	if r := a.do("GET", "/api/v1/social/telegram/connect/00000000-0000-0000-0000-000000000000", nil); r.status != 404 {
		t.Fatalf("status endpoint: %d", r.status)
	}
}

func TestTelegramLinkRequiresACsrfTokenAndAConfiguredBot(t *testing.T) {
	e, _ := tgEnv(t, envOpts{})
	a := e.browser()
	a.register("alice-csrf@example.com")
	saved := a.csrf
	a.csrf = ""
	if r := a.do("POST", "/api/v1/social/telegram/connect", nil); r.status != 403 {
		t.Fatalf("without CSRF token: %d %s", r.status, r.body)
	}
	a.csrf = saved

	// Without a bot token the provider is not configured: 501, nothing stored.
	plain := newEnv(t, envOpts{})
	p := plain.browser()
	p.register("alice-nobot@example.com")
	if r := p.do("POST", "/api/v1/social/telegram/connect", nil); r.status != 501 || r.errCode(t) != "PROVIDER_NOT_AVAILABLE" {
		t.Fatalf("unconfigured telegram: %d %s", r.status, r.body)
	}
	var n int
	_ = plain.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM telegram_link_codes`).Scan(&n)
	if n != 0 {
		t.Fatal("no code may be created while the bot is not configured")
	}
}

// ---------------------------------------------------------------- webhook intake

func webhookEnv(t *testing.T) (*env, *fakeTelegram) {
	return tgEnv(t, envOpts{mutate: func(c *config.Config) {
		c.TelegramToken, c.TelegramUpdatesMode, c.TelegramWebhookSecret = fakeBotToken, config.TelegramModeWebhook, webhookSecret
	}})
}

func (e *env) webhook(t *testing.T, secret *string, body []byte) resp {
	t.Helper()
	req, err := http.NewRequest("POST", e.srv.URL+"/api/v1/webhooks/telegram", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != nil {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", *secret)
	}
	// A fresh client: Telegram sends no cookie and no CSRF token.
	return (&client{e: e, http: &http.Client{Timeout: 30 * time.Second}}).send(req)
}

func TestTelegramWebhookVerifiesTheSecretAndConnectsEndToEnd(t *testing.T) {
	e, bot := webhookEnv(t)
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	bot.addChannel(chanY, "other_channel", "Other")
	a := e.browser()
	a.register("alice-wh@example.com")
	id, code := a.startLink()
	update := []byte(bot.channelPost(chanX, code).raw)

	str := func(s string) *string { return &s }
	for name, secret := range map[string]*string{"missing": nil, "empty": str(""), "wrong": str("nope"), "prefix": str(webhookSecret[:10]),
		"suffix": str(webhookSecret + "x"), "case": str(strings.ToUpper(webhookSecret))} {
		r := e.webhook(t, secret, update)
		if r.status != 401 || r.errCode(t) != "UNAUTHENTICATED" {
			t.Errorf("%s secret: %d %s", name, r.status, r.body)
		}
	}
	if a.linkStatus(id)["status"] != "pending" || bot.count("getChat") != 0 {
		t.Fatal("an unauthenticated delivery must have no effect at all")
	}

	secret := webhookSecret
	// Verified but unusable bodies are acknowledged and ignored; Telegram must not retry them.
	huge := append([]byte(`{"update_id":1,"channel_post":{"text":"`+code+`"}}`), bytes.Repeat([]byte(" "), 300<<10)...)
	for name, body := range map[string][]byte{"garbage": []byte("not json"), "empty": nil, "oversized": huge,
		"irrelevant": []byte(bot.edited(chanX, code).raw)} {
		r := e.webhook(t, &secret, body)
		if r.status != 200 || string(bytes.TrimSpace(r.body)) != `{"ok":true}` {
			t.Errorf("%s body: %d %s", name, r.status, r.body)
		}
	}
	if a.linkStatus(id)["status"] != "pending" {
		t.Fatal("garbage, oversized and irrelevant bodies must not connect anything")
	}

	// The real thing.
	if r := e.webhook(t, &secret, update); r.status != 200 || string(bytes.TrimSpace(r.body)) != `{"ok":true}` {
		t.Fatalf("webhook: %d %s", r.status, r.body)
	}
	st := a.waitLink(id, "connected", 5*time.Second)
	if acc := st["account"].(map[string]any); acc["username"] != "e2e_channel" {
		t.Fatalf("account: %v", st)
	}
	if !bot.wasDeleted(chanX) {
		t.Fatal("code message not deleted")
	}
	// A wrong code is acknowledged exactly like a right one: nothing leaks to the sender.
	if r := e.webhook(t, &secret, []byte(bot.channelPost(chanY, "SOS-AAAAAAAA").raw)); r.status != 200 {
		t.Fatalf("unknown code: %d", r.status)
	}
	// GET on the route is not a thing.
	if r := e.browser().do("GET", "/api/v1/webhooks/telegram", nil); r.status != 404 {
		t.Fatalf("GET webhook: %d", r.status)
	}
}

func TestTelegramWebhookRouteDoesNotExistInPollingMode(t *testing.T) {
	e, bot := tgEnv(t, envOpts{}) // default mode: polling
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	a := e.browser()
	a.register("alice-nowh@example.com")
	id, code := a.startLink()
	secret := webhookSecret
	for _, s := range []*string{nil, &secret, new(string)} {
		if r := e.webhook(t, s, []byte(bot.channelPost(chanX, code).raw)); r.status != 404 {
			t.Fatalf("webhook in polling mode: %d %s", r.status, r.body)
		}
	}
	if a.linkStatus(id)["status"] != "pending" {
		t.Fatal("connected through a disabled webhook")
	}
}

// ---------------------------------------------------------------- polling intake

func startPoller(t *testing.T, e *env, o telegram.PollerOptions) context.CancelFunc {
	t.Helper()
	p := e.app.NewTelegramPoller(o)
	if p == nil {
		t.Fatal("no poller: telegram is not configured")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("poller did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

var fastPoll = telegram.PollerOptions{Timeout: time.Second, LockTTL: 2 * time.Second, RetryMin: 5 * time.Millisecond,
	RetryMax: 40 * time.Millisecond, StandbyInterval: 20 * time.Millisecond}

func TestTelegramPollingConnectsAdvancesOffsetAndRecoversFromErrors(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	bot.addChannel(chanY, "other_channel", "Other")
	a, b := e.browser(), e.browser()
	a.register("alice-poll@example.com")
	b.register("bob-poll@example.com")

	// The Bot API misbehaves first: a 502, a 429 and a conflict, then a failing getChat on the first attempt.
	bot.mu.Lock()
	bot.failPoll = []int{502, 429, 409}
	bot.failGetChat = 2
	bot.mu.Unlock()

	idA, codeA := a.startLink()
	idB, codeB := b.startLink()
	chat := bot.channelPost(chanX, "weather is nice") // unrelated chatter before the code
	mine := bot.channelPost(chanX, codeA)
	theirs := bot.channelPost(chanY, codeB)
	bot.enqueue(chat, mine, theirs)
	stop := startPoller(t, e, fastPoll)

	a.waitLink(idA, "connected", 10*time.Second)
	b.waitLink(idB, "connected", 10*time.Second)
	if got := accountIDs(a.telegramAccounts()); got != strconv.FormatInt(chanX, 10) {
		t.Fatalf("alice: %q", got)
	}
	if got := accountIDs(b.telegramAccounts()); got != strconv.FormatInt(chanY, 10) {
		t.Fatalf("bob: %q", got)
	}
	if bot.count("getChat") < 3 {
		t.Fatalf("the transient getChat failure must have been retried, getChat called %d times", bot.count("getChat"))
	}

	// The offset moves past everything handled, both towards Telegram and in Redis.
	waitUntil(t, "offset confirmed", func() bool { return bot.lastOffset() == theirs.id+1 })
	key := "socialos:telegram:" + e.app.Cfg.QueueName + ":offset"
	got, err := e.app.Redis.Client.Get(context.Background(), key).Int64()
	if err != nil || got != theirs.id+1 {
		t.Fatalf("stored offset %d (%v), want %d", got, err, theirs.id+1)
	}
	// Handled updates are not seen again: each connect is audited once.
	var audits int
	_ = e.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action = 'social_account.connected'`).Scan(&audits)
	if audits != 2 {
		t.Fatalf("connect audited %d times, want 2", audits)
	}

	// A restarted worker resumes from the stored offset instead of replaying history.
	stop()
	bot.mu.Lock()
	calls := len(bot.offsets)
	bot.mu.Unlock()
	startPoller(t, e, fastPoll)
	waitUntil(t, "second poller polling", func() bool { bot.mu.Lock(); defer bot.mu.Unlock(); return len(bot.offsets) > calls })
	bot.mu.Lock()
	first := bot.offsets[calls]
	bot.mu.Unlock()
	if first != theirs.id+1 {
		t.Fatalf("restarted poller began at offset %d, want the stored %d", first, theirs.id+1)
	}
	idA2, codeA2 := a.startLink()
	bot.enqueue(bot.channelPost(chanY, codeA2))
	a.waitLink(idA2, "connected", 10*time.Second)
}

func TestTelegramPollingRunsOnOneWorkerAtATimeAndFailsOver(t *testing.T) {
	e, bot := tgEnv(t, envOpts{})
	bot.addChannel(chanX, "e2e_channel", "E2E Channel")
	bot.addChannel(chanY, "other_channel", "Other")
	a := e.browser()
	a.register("alice-lock@example.com")

	// The first worker takes the lease; two more start while it holds it and must stand by.
	stopFirst := startPoller(t, e, fastPoll)
	waitUntil(t, "first poller polling", func() bool { return bot.lastOffset() >= 0 })
	startPoller(t, e, fastPoll)
	startPoller(t, e, fastPoll)

	id1, code1 := a.startLink()
	bot.enqueue(bot.channelPost(chanX, code1))
	a.waitLink(id1, "connected", 10*time.Second)
	time.Sleep(400 * time.Millisecond) // several lease renewals with three pollers alive
	bot.mu.Lock()
	maxInflight := bot.maxInflight
	bot.mu.Unlock()
	if maxInflight != 1 {
		t.Fatalf("up to %d concurrent getUpdates calls: only one worker may poll", maxInflight)
	}

	// The polling worker goes away (releasing its lease): a standby takes over and carries on.
	stopFirst()
	id2, code2 := a.startLink()
	bot.enqueue(bot.channelPost(chanY, code2))
	a.waitLink(id2, "connected", 10*time.Second)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ---------------------------------------------------------------- connected chats still publish

func TestTelegramLinkedChannelPublishes(t *testing.T) {
	e, bot := tgEnv(t, envOpts{startWorker: true})
	bot.addChannel(-1009876543210, "e2e_channel", "E2E Channel")
	c := e.browser()
	c.register("tg@example.com")

	var bodies []string
	seen := func(r resp) resp { bodies = append(bodies, string(r.body)); return r }

	p := seen(c.do("GET", "/api/v1/social/providers", nil))
	for _, it := range p.json(t)["items"].([]any) {
		if m := it.(map[string]any); m["id"] == "telegram" && (m["configured"] != true || m["status"] != "supported") {
			t.Fatalf("telegram must be configured once a bot token is present: %v", m)
		}
	}

	id, code := c.startLink()
	if err := e.deliver(t, bot.channelPost(-1009876543210, code)); err != nil {
		t.Fatal(err)
	}
	st := c.waitLink(id, "connected", 5*time.Second)
	a := st["account"].(map[string]any)
	if a["provider"] != "telegram" || a["username"] != "e2e_channel" || a["display_name"] != "E2E Channel" || a["status"] != "active" {
		t.Fatalf("telegram account: %v", a)
	}
	accID := a["id"].(string)
	// Linking the same channel again does not duplicate the account.
	id2, code2 := c.startLink()
	if err := e.deliver(t, bot.channelPost(-1009876543210, code2)); err != nil {
		t.Fatal(err)
	}
	if again := c.waitLink(id2, "connected", 5*time.Second)["account"].(map[string]any); again["id"] != accID {
		t.Fatalf("relink produced another account: %v", again)
	}
	if n := len(c.telegramAccounts()); n != 1 {
		t.Fatalf("relinking duplicated the chat: %d accounts", n)
	}

	// Publish through the real worker; the bot receives exactly the text.
	post := seen(c.do("POST", "/api/v1/posts", map[string]any{"content": "hello telegram", "social_account_ids": []string{accID}}))
	pid := post.json(t)["id"].(string)
	seen(c.do("POST", "/api/v1/posts/"+pid+"/publish", nil))
	pub := c.waitStatus(pid, "published", 20*time.Second)
	tgt := pub["targets"].([]any)[0].(map[string]any)
	if got := bot.sent(); len(got) != 1 || got[0] != "hello telegram" {
		t.Fatalf("bot received %v", got)
	}
	if tgt["external_post_id"] != "-1009876543210:101" || tgt["external_url"] != "https://t.me/e2e_channel/101" {
		t.Fatalf("external reference: %v", tgt)
	}

	// A rejected post fails with a classified, sanitized error.
	bot.mu.Lock()
	bot.failSend = true
	bot.mu.Unlock()
	bad := c.must("POST", "/api/v1/posts", map[string]any{"content": "will fail", "social_account_ids": []string{accID}}, 201)
	c.must("POST", "/api/v1/posts/"+bad["id"].(string)+"/publish", nil, 202)
	failed := c.waitStatus(bad["id"].(string), "failed", 20*time.Second)
	ft := failed["targets"].([]any)[0].(map[string]any)
	if ft["error_code"] == nil || ft["error_code"] == "" || strings.Contains(fmt.Sprint(ft), fakeBotToken) {
		t.Fatalf("failure details: %v", ft)
	}
	bodies = append(bodies, fmt.Sprint(failed), fmt.Sprint(c.must("GET", "/api/v1/posts/"+bad["id"].(string), nil, 200)))
	bodies = append(bodies, fmt.Sprint(c.must("GET", "/api/v1/audit-logs?limit=100", nil, 200)))

	// The bot token is configuration: it must be in no response and in no table.
	for _, b := range bodies {
		if strings.Contains(b, fakeBotToken) || strings.Contains(b, "AAH-SECRET") {
			t.Fatalf("bot token leaked in an API response: %.300s", b)
		}
	}
	for table, col := range map[string]string{"social_accounts": "metadata::text", "audit_logs": "metadata::text",
		"publication_attempts": "coalesce(error_message,'')", "post_targets": "coalesce(error_message,'')"} {
		var n int
		q := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s LIKE '%%AAH-SECRET%%'", table, col)
		if err := e.app.DB.Pool.QueryRow(context.Background(), q).Scan(&n); err != nil || n != 0 {
			t.Errorf("bot token stored in %s (n=%d err=%v)", table, n, err)
		}
	}
	var credRows int
	_ = e.app.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM oauth_credentials WHERE social_account_id = $1`, accID).Scan(&credRows)
	if credRows != 0 {
		t.Errorf("telegram accounts need no stored credentials, found %d", credRows)
	}
}
