package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fakeBotToken = "123456:AAH-SECRET-bot-token"
	fakeBotID    = 42
)

// fakeChat is one chat the fake bot knows.
type fakeChat struct {
	ID       int64
	Type     string // channel | supergroup | group
	Title    string
	Username string
	// BotStatus is the bot's membership: administrator (default) | member | left.
	BotStatus  string
	BotCanPost bool
	// Admins maps user id to status for getChatMember of humans (default "member").
	Admins map[int64]string
}

// fakeTelegram is a Telegram Bot API double: chats, getUpdates with offset
// confirmation, deleteMessage and sendMessage, with switches for failures.
type fakeTelegram struct {
	t   *testing.T
	srv *httptest.Server

	mu          sync.Mutex
	chats       map[int64]*fakeChat
	queue       []fakeUpdate // not yet confirmed by an offset
	nextUpdate  int64
	nextMessage int64
	offsets     []int64        // offset of every getUpdates call
	calls       map[string]int // per method
	deleted     []string       // "chat:message"
	texts       []string       // sendMessage texts
	failSend    bool
	failPoll    []int // HTTP status of the next getUpdates calls, one each
	failGetChat int   // number of upcoming getChat calls answered with 500
	inflight    int
	maxInflight int
}

type fakeUpdate struct {
	id  int64
	raw string
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	f := &fakeTelegram{t: t, chats: map[int64]*fakeChat{}, calls: map[string]int{}, nextUpdate: 1000, nextMessage: 500}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

// addChannel registers a channel in which the bot is an administrator with "Post messages".
func (f *fakeTelegram) addChannel(id int64, username, title string) *fakeChat {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &fakeChat{ID: id, Type: "channel", Title: title, Username: username, BotStatus: "administrator", BotCanPost: true, Admins: map[int64]string{}}
	f.chats[id] = c
	return c
}

func (f *fakeTelegram) addGroup(id int64, title string) *fakeChat {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &fakeChat{ID: id, Type: "supergroup", Title: title, BotStatus: "administrator", BotCanPost: true, Admins: map[int64]string{}}
	f.chats[id] = c
	return c
}

func (f *fakeTelegram) chat(id int64) *fakeChat {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chats[id]
}

func (f *fakeTelegram) setBot(id int64, status string, canPost bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chats[id].BotStatus, f.chats[id].BotCanPost = status, canPost
}

func (f *fakeTelegram) chatJSON(c *fakeChat) map[string]any {
	m := map[string]any{"id": c.ID, "type": c.Type, "title": c.Title}
	if c.Username != "" {
		m["username"] = c.Username
	}
	return m
}

// newUpdate returns a fresh update id and message id.
func (f *fakeTelegram) ids() (int64, int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextUpdate++
	f.nextMessage++
	return f.nextUpdate, f.nextMessage
}

func raw(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// channelPost builds the update Telegram sends when text is posted in a channel.
func (f *fakeTelegram) channelPost(chatID int64, text string) fakeUpdate {
	uid, mid := f.ids()
	c := f.chatJSON(f.chat(chatID))
	return fakeUpdate{uid, raw(map[string]any{"update_id": uid, "channel_post": map[string]any{
		"message_id": mid, "sender_chat": c, "chat": c, "date": time.Now().Unix(), "text": text}})}
}

// groupMessage is a message a (possibly non-admin) member writes in a group.
func (f *fakeTelegram) groupMessage(chatID, fromID int64, text string) fakeUpdate {
	uid, mid := f.ids()
	return fakeUpdate{uid, raw(map[string]any{"update_id": uid, "message": map[string]any{
		"message_id": mid, "from": map[string]any{"id": fromID, "is_bot": false, "first_name": "Member"},
		"chat": f.chatJSON(f.chat(chatID)), "date": time.Now().Unix(), "text": text}})}
}

// anonymousAdminMessage is what Telegram sends for an admin writing "as the group".
func (f *fakeTelegram) anonymousAdminMessage(chatID int64, text string) fakeUpdate {
	uid, mid := f.ids()
	c := f.chatJSON(f.chat(chatID))
	return fakeUpdate{uid, raw(map[string]any{"update_id": uid, "message": map[string]any{
		"message_id": mid, "from": map[string]any{"id": 1087968824, "is_bot": true, "username": "GroupAnonymousBot"},
		"sender_chat": c, "chat": c, "date": time.Now().Unix(), "text": text}})}
}

// asChannelMessage is a group message posted on behalf of another channel (or an automatic forward).
func (f *fakeTelegram) groupMessageAsChannel(chatID, otherChannelID int64, text string) fakeUpdate {
	uid, mid := f.ids()
	return fakeUpdate{uid, raw(map[string]any{"update_id": uid, "message": map[string]any{
		"message_id": mid, "from": map[string]any{"id": 777000, "is_bot": false},
		"sender_chat": f.chatJSON(f.chat(otherChannelID)), "chat": f.chatJSON(f.chat(chatID)), "date": time.Now().Unix(), "text": text}})}
}

func (f *fakeTelegram) edited(chatID int64, text string) fakeUpdate {
	uid, mid := f.ids()
	c := f.chatJSON(f.chat(chatID))
	return fakeUpdate{uid, raw(map[string]any{"update_id": uid, "edited_channel_post": map[string]any{
		"message_id": mid, "sender_chat": c, "chat": c, "date": 1, "edit_date": 2, "text": text}})}
}

// enqueue makes updates available to getUpdates until a later offset confirms them.
func (f *fakeTelegram) enqueue(us ...fakeUpdate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, us...)
}

func (f *fakeTelegram) lastOffset() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.offsets) == 0 {
		return -1
	}
	return f.offsets[len(f.offsets)-1]
}

func (f *fakeTelegram) wasDeleted(chatID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.deleted {
		if strings.HasPrefix(d, strconv.FormatInt(chatID, 10)+":") {
			return true
		}
	}
	return false
}

func (f *fakeTelegram) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...)
}

func (f *fakeTelegram) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *fakeTelegram) handle(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	ok := func(v any) { reply(200, map[string]any{"ok": true, "result": v}) }
	bad := func(code int, desc string) {
		reply(code, map[string]any{"ok": false, "error_code": code, "description": desc})
	}
	prefix := "/bot" + fakeBotToken + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		bad(401, "Unauthorized")
		return
	}
	method := strings.TrimPrefix(r.URL.Path, prefix)
	var params map[string]any
	_ = json.NewDecoder(r.Body).Decode(&params)
	f.mu.Lock()
	f.calls[method]++
	f.mu.Unlock()

	findChat := func() *fakeChat {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch v := params["chat_id"].(type) {
		case float64:
			return f.chats[int64(v)]
		case string:
			for _, c := range f.chats {
				if c.Username != "" && "@"+c.Username == v {
					return c
				}
			}
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				return f.chats[id]
			}
		}
		return nil
	}

	switch method {
	case "getMe":
		ok(map[string]any{"id": fakeBotID, "is_bot": true, "username": "socialos_bot"})
	case "getChat":
		f.mu.Lock()
		if f.failGetChat > 0 {
			f.failGetChat--
			f.mu.Unlock()
			bad(500, "Internal Server Error")
			return
		}
		f.mu.Unlock()
		c := findChat()
		if c == nil || c.BotStatus == "left" {
			bad(400, "Bad Request: chat not found")
			return
		}
		ok(f.chatJSON(c))
	case "getChatMember":
		c := findChat()
		if c == nil {
			bad(400, "Bad Request: chat not found")
			return
		}
		uid, _ := params["user_id"].(float64)
		f.mu.Lock()
		defer f.mu.Unlock()
		if int64(uid) == fakeBotID {
			m := map[string]any{"status": c.BotStatus}
			if c.BotStatus == "administrator" && c.Type == "channel" {
				m["can_post_messages"] = c.BotCanPost
			}
			ok(m)
			return
		}
		st := c.Admins[int64(uid)]
		if st == "" {
			st = "member"
		}
		ok(map[string]any{"status": st})
	case "deleteMessage":
		f.mu.Lock()
		f.deleted = append(f.deleted, fmt.Sprintf("%v:%v", int64(params["chat_id"].(float64)), int64(params["message_id"].(float64))))
		f.mu.Unlock()
		ok(true)
	case "sendMessage":
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failSend {
			bad(403, "Forbidden: bot was kicked from the channel chat")
			return
		}
		f.texts = append(f.texts, fmt.Sprint(params["text"]))
		ok(map[string]any{"message_id": 100 + len(f.texts)})
	case "getUpdates":
		f.getUpdates(params, ok, bad)
	default:
		bad(404, "Not Found")
	}
}

func (f *fakeTelegram) getUpdates(params map[string]any, ok func(any), bad func(int, string)) {
	off, _ := params["offset"].(float64)
	f.mu.Lock()
	f.offsets = append(f.offsets, int64(off))
	if len(f.failPoll) > 0 {
		code := f.failPoll[0]
		f.failPoll = f.failPoll[1:]
		f.mu.Unlock()
		bad(code, "simulated getUpdates failure")
		return
	}
	f.inflight++
	if f.inflight > f.maxInflight {
		f.maxInflight = f.inflight
	}
	// Confirm (drop) everything below the offset, like Telegram does.
	var keep []fakeUpdate
	for _, u := range f.queue {
		if u.id >= int64(off) {
			keep = append(keep, u)
		}
	}
	f.queue = keep
	pending := append([]fakeUpdate(nil), keep...)
	f.mu.Unlock()
	if len(pending) == 0 {
		time.Sleep(40 * time.Millisecond) // the long poll
	}
	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()
	out := make([]json.RawMessage, len(pending))
	for i, u := range pending {
		out[i] = json.RawMessage(u.raw)
	}
	ok(out)
}
