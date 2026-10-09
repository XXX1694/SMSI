package accounts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/audit"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/linkcode"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// ---- fakes -------------------------------------------------------------------

type memRepo struct {
	mu   sync.Mutex
	accs map[uuid.UUID]*socialaccount.Account
}

func (r *memRepo) Upsert(_ context.Context, a *socialaccount.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.accs {
		if x.UserID == a.UserID && x.Provider == a.Provider && x.ProviderAccountID == a.ProviderAccountID {
			a.ID = x.ID
			*x = *a
			return nil
		}
	}
	a.ID = uuid.New()
	cp := *a
	r.accs[a.ID] = &cp
	return nil
}
func (r *memRepo) List(context.Context, uuid.UUID) ([]socialaccount.Account, error) { return nil, nil }
func (r *memRepo) Get(_ context.Context, userID, id uuid.UUID) (*socialaccount.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.accs[id]; ok && a.UserID == userID {
		cp := *a
		return &cp, nil
	}
	return nil, errs.NotFoundf("social account")
}
func (r *memRepo) SetStatus(_ context.Context, _ uuid.UUID, id uuid.UUID, st socialaccount.Status) error {
	if a, ok := r.accs[id]; ok {
		a.Status = st
	}
	return nil
}
func (r *memRepo) SaveCredentials(context.Context, uuid.UUID, EncryptedCredentials) error { return nil }
func (r *memRepo) GetCredentials(context.Context, uuid.UUID) (*EncryptedCredentials, error) {
	return nil, errs.NotFoundf("credentials")
}
func (r *memRepo) DeleteCredentials(context.Context, uuid.UUID) error { return nil }

type memLinks struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*linkcode.Code
	finds  int
	markEr error
}

func (l *memLinks) Create(_ context.Context, c *linkcode.Code, maxActive int, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	active := 0
	for _, x := range l.byID {
		if x.UserID == c.UserID && x.Usable(now) {
			active++
		}
	}
	if active >= maxActive { // the real repo retires the oldest; the fake just records the call
		for _, x := range l.byID {
			if x.UserID == c.UserID && x.Usable(now) {
				x.ExpiresAt = now
				break
			}
		}
	}
	c.ID, c.CreatedAt = uuid.New(), now
	cp := *c
	l.byID[c.ID] = &cp
	return nil
}
func (l *memLinks) Get(_ context.Context, userID, id uuid.UUID) (*linkcode.Code, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.byID[id]; ok && c.UserID == userID {
		cp := *c
		return &cp, nil
	}
	return nil, errs.NotFoundf("link")
}
func (l *memLinks) FindByHash(_ context.Context, hash string) (*linkcode.Code, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.finds++
	for _, c := range l.byID {
		if c.Hash == hash {
			cp := *c
			return &cp, nil
		}
	}
	return nil, errs.NotFoundf("link code")
}
func (l *memLinks) MarkUsed(_ context.Context, id uuid.UUID, now time.Time, chatID string, accID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.markEr != nil {
		return l.markEr
	}
	c := l.byID[id]
	if c == nil || !c.Usable(now) {
		return errs.NotFoundf("link code")
	}
	c.UsedAt, c.ChatID, c.SocialAccountID = &now, chatID, &accID
	return nil
}

// memTx serialises transactions and discards the audit rows of a failed one,
// like the database does (the lost-race loser rolls back its audit entry).
type memTx struct {
	mu    sync.Mutex
	audit *auditRec
}

type inTxKey struct{}

func (t *memTx) InTx(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(inTxKey{}) != nil { // nested calls join the outer transaction
		return fn(ctx)
	}
	ctx = context.WithValue(ctx, inTxKey{}, true)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.audit.mu.Lock()
	n := len(t.audit.entries)
	t.audit.mu.Unlock()
	err := fn(ctx)
	if err != nil {
		t.audit.mu.Lock()
		t.audit.entries = t.audit.entries[:n]
		t.audit.mu.Unlock()
	}
	return err
}

type auditRec struct {
	mu      sync.Mutex
	entries []auditEntry
}
type auditEntry struct {
	a      actor.Actor
	action string
	meta   map[string]any
}

func (r *auditRec) Record(_ context.Context, a actor.Actor, action, _, _ string, meta map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, auditEntry{a, action, meta})
	return nil
}

type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fixedClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fixedClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// fakeChat is a configurable ChatLinker. ParseUpdate treats raw as "<chatID>|<messageID>|<senderID or *>|<text>".
type fakeChat struct {
	mu        sync.Mutex
	configure bool
	admins    map[string]bool // senderID -> admin of every chat
	verifyErr error
	adminErr  error
	deleteErr error
	deleted   []string
	verified  []string
	adminChks []string
}

func (f *fakeChat) Name() string                                { return "telegram" }
func (f *fakeChat) DisplayName() string                         { return "Telegram" }
func (f *fakeChat) Supported() bool                             { return true }
func (f *fakeChat) Configured() bool                            { return f.configure }
func (f *fakeChat) Capabilities() provider.Capabilities         { return provider.Capabilities{} }
func (f *fakeChat) BotUsername(context.Context) (string, error) { return "socialos_bot", nil }
func (f *fakeChat) LinkInstructions(bot, code string, ttl time.Duration) string {
	return "add @" + bot + " and post " + code
}
func (f *fakeChat) VerifyChat(_ context.Context, chat string) (provider.Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = append(f.verified, chat)
	if f.verifyErr != nil {
		return provider.Profile{}, f.verifyErr
	}
	return provider.Profile{ID: chat, Username: "chan" + chat, DisplayName: "Channel " + chat, Metadata: map[string]any{"chat_id": chat}}, nil
}
func (f *fakeChat) IsChatAdmin(_ context.Context, chatID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adminChks = append(f.adminChks, chatID+"/"+userID)
	return f.admins[userID], f.adminErr
}
func (f *fakeChat) DeleteMessage(_ context.Context, chatID string, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, chatID)
	return f.deleteErr
}
func (f *fakeChat) ParseUpdate(raw []byte) (*provider.ChatMessage, error) {
	parts := strings.SplitN(string(raw), "|", 4)
	if len(parts) != 4 {
		return nil, errors.New("malformed")
	}
	if parts[0] == "-" {
		return nil, nil
	}
	m := &provider.ChatMessage{ChatID: parts[0], MessageID: 11, Text: parts[3]}
	if parts[2] == "*" {
		m.SenderTrusted = true
	} else {
		m.SenderID = parts[2]
	}
	return m, nil
}

type rig struct {
	svc   *Service
	chat  *fakeChat
	links *memLinks
	repo  *memRepo
	audit *auditRec
	clock *fixedClock
}

func newRig(t *testing.T) *rig {
	t.Helper()
	chat := &fakeChat{configure: true, admins: map[string]bool{}}
	r := &rig{chat: chat, links: &memLinks{byID: map[uuid.UUID]*linkcode.Code{}}, repo: &memRepo{accs: map[uuid.UUID]*socialaccount.Account{}},
		audit: &auditRec{}, clock: &fixedClock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}}
	r.svc = NewService(Deps{Repo: r.repo, Links: r.links, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Registry: provider.NewRegistry(chat), Tx: &memTx{audit: r.audit}, Audit: r.audit, Clock: r.clock})
	return r
}

func session(user uuid.UUID) actor.Actor {
	return actor.Actor{UserID: user, Type: actor.TypeUser, ID: user.String(), SessionID: uuid.New(), EmailVerified: true}
}

func (r *rig) start(t *testing.T, user uuid.UUID) *LinkStart {
	t.Helper()
	ls, err := r.svc.StartChatLink(context.Background(), session(user), "telegram")
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func (r *rig) deliver(chat, sender, text string) error {
	return r.svc.HandleChatUpdate(context.Background(), "telegram", []byte(chat+"|11|"+sender+"|"+text))
}

func (r *rig) accountsOf(user uuid.UUID) (n int) {
	for _, a := range r.repo.accs {
		if a.UserID == user {
			n++
		}
	}
	return n
}

// ---- tests -------------------------------------------------------------------

func TestStartChatLinkStoresOnlyTheHash(t *testing.T) {
	r, user := newRig(t), uuid.New()
	ls := r.start(t, user)
	if !strings.HasPrefix(ls.Code, "SOS-") || ls.BotUsername != "socialos_bot" || !strings.Contains(ls.Instructions, ls.Code) {
		t.Fatalf("%+v", ls)
	}
	if want := r.clock.Now().Add(linkcode.TTL); !ls.ExpiresAt.Equal(want) {
		t.Fatalf("expires %v want %v", ls.ExpiresAt, want)
	}
	rec := r.links.byID[ls.ID]
	if rec == nil || rec.UserID != user || rec.Hash != linkcode.Hash(ls.Code) || rec.Hash == ls.Code || rec.Used() {
		t.Fatalf("stored record %+v", rec)
	}
}

func TestStartChatLinkNeedsASessionAndAConfiguredProvider(t *testing.T) {
	r := newRig(t)
	key := actor.Actor{UserID: uuid.New(), Type: actor.TypeAPIKey, ID: "k"}
	if _, err := r.svc.StartChatLink(context.Background(), key, "telegram"); !errs.Is(err, errs.Forbidden) {
		t.Fatalf("an API key must not start a link: %v", err)
	}
	if _, err := r.svc.StartChatLink(context.Background(), actor.Actor{}, "telegram"); !errs.Is(err, errs.Unauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
	r.chat.configure = false
	if _, err := r.svc.StartChatLink(context.Background(), session(uuid.New()), "telegram"); !errs.Is(err, errs.ProviderNotAvailable) {
		t.Fatalf("unconfigured: %v", err)
	}
	r.chat.configure = true
	if _, err := r.svc.StartChatLink(context.Background(), session(uuid.New()), "nope"); !errs.Is(err, errs.ProviderNotAvailable) {
		t.Fatalf("unknown provider: %v", err)
	}
	if len(r.links.byID) != 0 {
		t.Fatal("failed attempts must not create codes")
	}
}

func TestRedeemConnectsTheChatToTheCodesOwnerOnly(t *testing.T) {
	r, alice, bob := newRig(t), uuid.New(), uuid.New()
	a, b := r.start(t, alice), r.start(t, bob)

	if err := r.deliver("-100", "*", "  "+strings.ToLower(a.Code)+"\n"); err != nil {
		t.Fatal(err)
	}
	if r.accountsOf(alice) != 1 || r.accountsOf(bob) != 0 {
		t.Fatalf("alice has %d accounts, bob %d; the chat must go to alice only", r.accountsOf(alice), r.accountsOf(bob))
	}
	for _, acc := range r.repo.accs {
		if acc.UserID != alice || acc.Provider != "telegram" || acc.ProviderAccountID != "-100" || acc.Status != socialaccount.StatusActive ||
			acc.Username != "chan-100" || len(acc.Scopes) != 1 || acc.Scopes[0] != "post_messages" {
			t.Fatalf("account %+v", acc)
		}
	}
	rec := r.links.byID[a.ID]
	if !rec.Used() || rec.ChatID != "-100" || rec.SocialAccountID == nil {
		t.Fatalf("code not marked used with chat and account: %+v", rec)
	}
	if r.links.byID[b.ID].Used() {
		t.Fatal("bob's code must stay untouched")
	}
	if len(r.chat.deleted) != 1 {
		t.Fatalf("the code message must be deleted, got %v", r.chat.deleted)
	}
	if len(r.audit.entries) != 1 {
		t.Fatalf("audit entries: %+v", r.audit.entries)
	}
	e := r.audit.entries[0]
	if e.action != audit.ActionAccountConnected || e.a.UserID != alice || e.a.Type != actor.TypeSystem || e.a.ID != "telegram" || e.a.Label != "telegram" {
		t.Fatalf("audit must record a system/telegram actor for the owner: %+v", e)
	}
	for k, v := range e.meta {
		if s, ok := v.(string); ok && strings.Contains(s, a.Code) {
			t.Fatalf("audit metadata %q leaks the code", k)
		}
	}

	st, err := r.svc.ChatLinkStatus(context.Background(), session(alice), a.ID)
	if err != nil || st.Status != linkcode.StatusConnected || st.Account == nil || st.Account.ProviderAccountID != "-100" {
		t.Fatalf("status %+v %v", st, err)
	}
	if st, err := r.svc.ChatLinkStatus(context.Background(), session(bob), b.ID); err != nil || st.Status != linkcode.StatusPending || st.Account != nil {
		t.Fatalf("bob's own link is still pending: %+v %v", st, err)
	}
}

func TestChatLinkStatusIsTenantScoped(t *testing.T) {
	r, alice, bob := newRig(t), uuid.New(), uuid.New()
	a := r.start(t, alice)
	if _, err := r.svc.ChatLinkStatus(context.Background(), session(bob), a.ID); !errs.Is(err, errs.NotFound) {
		t.Fatalf("another user must get NOT_FOUND, got %v", err)
	}
	if _, err := r.svc.ChatLinkStatus(context.Background(), session(alice), uuid.New()); !errs.Is(err, errs.NotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := r.svc.ChatLinkStatus(context.Background(), actor.Actor{UserID: alice, Type: actor.TypeAPIKey}, a.ID); !errs.Is(err, errs.Forbidden) {
		t.Fatalf("API key: %v", err)
	}
	r.clock.add(linkcode.TTL)
	if st, _ := r.svc.ChatLinkStatus(context.Background(), session(alice), a.ID); st.Status != linkcode.StatusExpired {
		t.Fatalf("after the TTL the status is expired, got %q", st.Status)
	}
}

func TestExpiredAndReusedCodesDoNotConnect(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	old := r.start(t, alice)
	r.clock.add(linkcode.TTL + time.Second)
	if err := r.deliver("-100", "*", old.Code); err != nil || r.accountsOf(alice) != 0 {
		t.Fatalf("expired code: %v, %d accounts", err, r.accountsOf(alice))
	}
	if len(r.chat.verified) != 0 || len(r.chat.deleted) != 0 {
		t.Fatal("an expired code must not trigger platform calls")
	}

	fresh := r.start(t, alice)
	if err := r.deliver("-100", "*", fresh.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("first use: %v", err)
	}
	// The same code in another chat, and the same message delivered again.
	if err := r.deliver("-200", "*", fresh.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("reuse in another chat must not connect: %v, %d accounts", err, r.accountsOf(alice))
	}
	if err := r.deliver("-100", "*", fresh.Code); err != nil || r.accountsOf(alice) != 1 || len(r.audit.entries) != 1 {
		t.Fatal("re-delivery must be a silent no-op")
	}
}

func TestWrongOrUnrelatedMessagesAreSilentlyIgnored(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	typo := ls.Code[:len(ls.Code)-1] + map[bool]string{true: "A", false: "B"}[!strings.HasSuffix(ls.Code, "A")]
	for _, text := range []string{"", "hello everyone", "SOS-AAAAAAAA", typo, "please link " + ls.Code, ls.Code + " thanks", ls.Code + "\n" + ls.Code} {
		if err := r.deliver("-100", "*", text); err != nil {
			t.Errorf("%q: a non-match must not be an error, got %v", text, err)
		}
	}
	if err := r.svc.HandleChatUpdate(context.Background(), "telegram", []byte("-|0|*|"+ls.Code)); err != nil { // irrelevant update kind
		t.Error(err)
	}
	if err := r.svc.HandleChatUpdate(context.Background(), "telegram", []byte("garbage")); err != nil {
		t.Errorf("a malformed update is dropped, not retried: %v", err)
	}
	if r.accountsOf(alice) != 0 || len(r.chat.verified) != 0 || len(r.chat.deleted) != 0 || len(r.audit.entries) != 0 {
		t.Fatal("nothing may happen for non-matching messages")
	}
	// Only the two code-shaped guesses (SOS-AAAAAAAA and the typo) may reach the database; ordinary chat text must not.
	if r.links.finds != 2 {
		t.Fatalf("expected 2 database lookups, got %d", r.links.finds)
	}
}

func TestGroupMessagesRequireAnAdministratorSender(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.chat.admins["admin1"] = true

	if err := r.deliver("-500", "random-member", ls.Code); err != nil || r.accountsOf(alice) != 0 {
		t.Fatalf("a non-admin member must not link the group: %v", err)
	}
	if len(r.chat.verified) != 0 || len(r.chat.deleted) != 0 {
		t.Fatal("a non-admin's message must not even reach the bot rights check")
	}
	if r.links.byID[ls.ID].Used() {
		t.Fatal("the code must not be burned by a non-admin")
	}
	if err := r.deliver("-500", "admin1", ls.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("an admin's message links the group: %v", err)
	}
	if got := strings.Join(r.chat.adminChks, ","); got != "-500/random-member,-500/admin1" {
		t.Fatalf("admin checks %q", got)
	}
}

func TestChannelPostsSkipTheSenderCheck(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	if err := r.deliver("-100", "*", ls.Code); err != nil || r.accountsOf(alice) != 1 || len(r.chat.adminChks) != 0 {
		t.Fatalf("trusted senders are not looked up: %v %v", err, r.chat.adminChks)
	}
}

func TestBotWithoutPostRightsDoesNotConnectButTheCodeSurvives(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.chat.verifyErr = &provider.Error{Kind: provider.KindPermanent, Provider: "telegram", Code: "CHAT_INVALID", Message: "the bot is an admin but lacks the 'Post messages' right"}
	if err := r.deliver("-100", "*", ls.Code); err != nil {
		t.Fatalf("missing rights is a silent non-match, got %v", err)
	}
	if r.accountsOf(alice) != 0 || len(r.chat.deleted) != 0 || len(r.audit.entries) != 0 || r.links.byID[ls.ID].Used() {
		t.Fatal("nothing may be connected, deleted or audited, and the code must stay usable")
	}
	// The user fixes the rights and posts the same code again.
	r.chat.verifyErr = nil
	if err := r.deliver("-100", "*", ls.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("retry after fixing the rights: %v", err)
	}
}

func TestTransientPlatformFailuresAreReturnedForRetry(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.chat.verifyErr = &provider.Error{Kind: provider.KindRetryable, Provider: "telegram", HTTPStatus: 502, Message: "bad gateway"}
	if err := r.deliver("-100", "*", ls.Code); err == nil {
		t.Fatal("a transient failure must be returned so the poller retries")
	}
	r.chat.verifyErr = errors.New("dial tcp: connection refused")
	if err := r.deliver("-100", "*", ls.Code); err != nil {
		t.Fatal("an unclassifiable plain error is permanent: do not loop on it")
	}
	r.chat.verifyErr = nil
	r.chat.adminErr = &provider.Error{Kind: provider.KindRetryable, Provider: "telegram", HTTPStatus: 429, Message: "slow"}
	if err := r.deliver("-100", "member", ls.Code); err == nil {
		t.Fatal("a transient sender-check failure must be returned")
	}
	r.chat.adminErr = &provider.Error{Kind: provider.KindPermanent, Provider: "telegram", HTTPStatus: 400, Message: "user not found"}
	if err := r.deliver("-100", "member", ls.Code); err != nil {
		t.Fatal("a permanent sender-check failure is a silent non-match")
	}
	r.chat.adminErr = nil
	if r.accountsOf(alice) != 0 || r.links.byID[ls.ID].Used() {
		t.Fatal("nothing connected so far")
	}
	// Database failure on the way is transient too.
	r.links.markEr = errors.New("connection reset")
	if err := r.deliver("-100", "*", ls.Code); err == nil {
		t.Fatal("a database failure must be returned for retry")
	}
	r.links.markEr = nil
	if err := r.deliver("-100", "*", ls.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("recovery: %v", err)
	}
}

func TestRacingRedemptionsConnectOnce(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = r.deliver("-100", "*", ls.Code) }()
	}
	wg.Wait()
	if r.accountsOf(alice) != 1 || len(r.audit.entries) != 1 {
		t.Fatalf("accounts %d audit %d, want exactly one of each", r.accountsOf(alice), len(r.audit.entries))
	}
}

func TestMarkUsedLostRaceIsSilent(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.links.markEr = errs.NotFoundf("link code")
	if err := r.deliver("-100", "*", ls.Code); err != nil || len(r.chat.deleted) != 0 {
		t.Fatalf("losing the race to another delivery is silent and must not delete the message: %v", err)
	}
}

func TestDeleteFailureDoesNotUndoTheLink(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.chat.deleteErr = errors.New("not enough rights to delete")
	if err := r.deliver("-100", "*", ls.Code); err != nil || r.accountsOf(alice) != 1 || !r.links.byID[ls.ID].Used() {
		t.Fatalf("link must succeed even if the message cannot be deleted: %v", err)
	}
}

func TestReconnectingARevokedChatReactivatesItForTheSameOwner(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	first := r.start(t, alice)
	_ = r.deliver("-100", "*", first.Code)
	for _, a := range r.repo.accs {
		a.Status = socialaccount.StatusRevoked
	}
	second := r.start(t, alice)
	_ = r.deliver("-100", "*", second.Code)
	if r.accountsOf(alice) != 1 {
		t.Fatalf("one account per (user, chat), got %d", r.accountsOf(alice))
	}
	for _, a := range r.repo.accs {
		if a.Status != socialaccount.StatusActive {
			t.Fatalf("status %q", a.Status)
		}
	}
}

func TestHandleChatUpdateWithUnconfiguredProviderIsANoop(t *testing.T) {
	r := newRig(t)
	r.chat.configure = false
	if err := r.deliver("-100", "*", "SOS-AAAAAAAA"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.HandleChatUpdate(context.Background(), "linkedin", []byte("x")); err != nil {
		t.Fatal("an unknown provider must be ignored")
	}
}

func TestUnverifiedOwnerCannotStartChatLink(t *testing.T) {
	r := newRig(t)
	a := session(uuid.New())
	a.EmailVerified = false
	if _, err := r.svc.StartChatLink(context.Background(), a, "telegram"); !errs.Is(err, errs.EmailNotVerified) {
		t.Fatalf("want EMAIL_NOT_VERIFIED, got %v", err)
	}
	if len(r.links.byID) != 0 {
		t.Fatal("a code was created")
	}
}

type closedGate struct{}

func (closedGate) RequireVerifiedOwner(context.Context, uuid.UUID) error {
	return errs.New(errs.EmailNotVerified, "verify your email address to use this feature")
}

// A code started while verification was off must not connect a channel once the owner is unverified under enforcement.
func TestChatLinkCompletionChecksTheOwnersVerification(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.svc.gate = closedGate{}
	if err := r.deliver("-100", "*", ls.Code); err == nil || r.accountsOf(alice) != 0 {
		t.Fatalf("an unverified owner connected a channel: err=%v accounts=%d", err, r.accountsOf(alice))
	}
	r.svc.gate = nil
	ls = r.start(t, alice)
	if err := r.deliver("-100", "*", ls.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("without a gate the link must work: err=%v accounts=%d", err, r.accountsOf(alice))
	}
}

type fullQuota struct{ port.NoQuota }

func (fullQuota) EnforceAccount(context.Context, uuid.UUID, string, string) error {
	return errs.New(errs.QuotaExceeded, "connected accounts limit reached")
}

// A quota refusal is final for that message: returning it would make the shared poller retry (and stall) for everyone.
func TestChatLinkAtTheAccountLimitIsDroppedNotRetried(t *testing.T) {
	r, alice := newRig(t), uuid.New()
	ls := r.start(t, alice)
	r.svc.quota = fullQuota{}
	if err := r.deliver("-100", "*", ls.Code); err != nil {
		t.Fatalf("a quota refusal must not be returned to the poller: %v", err)
	}
	if r.accountsOf(alice) != 0 || r.links.byID[ls.ID].Used() {
		t.Fatal("nothing may be connected and the code must stay unused")
	}
	r.svc.quota = port.NoQuota{}
	if err := r.deliver("-100", "*", ls.Code); err != nil || r.accountsOf(alice) != 1 {
		t.Fatalf("after freeing a slot the same code works: %v", err)
	}
}
