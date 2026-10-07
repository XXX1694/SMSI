package telegram

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

func TestParseUpdate(t *testing.T) {
	a := New(Config{BotToken: testToken})
	const code = "SOS-7KQ2M9XA"
	chPost := func(chat string) string {
		return `{"update_id":1,"channel_post":{"message_id":7,"sender_chat":` + chat + `,"chat":` + chat + `,"date":1,"text":"` + code + `"}}`
	}
	channel := `{"id":-1001234567890,"type":"channel","title":"My Channel","username":"mychannel"}`
	supergroup := `{"id":-1009999999999,"type":"supergroup","title":"G"}`
	group := `{"id":-555,"type":"group","title":"Old group"}`
	member := `{"id":77,"is_bot":false,"first_name":"Ann"}`

	cases := []struct {
		name string
		raw  string
		want *provider.ChatMessage
	}{
		{"channel post", chPost(channel),
			&provider.ChatMessage{ChatID: "-1001234567890", MessageID: 7, Text: code, SenderTrusted: true}},
		{"supergroup message from a member",
			`{"update_id":2,"message":{"message_id":9,"from":` + member + `,"chat":` + supergroup + `,"text":"` + code + `"}}`,
			&provider.ChatMessage{ChatID: "-1009999999999", MessageID: 9, Text: code, SenderID: "77"}},
		{"basic group message from a member",
			`{"update_id":3,"message":{"message_id":2,"from":` + member + `,"chat":` + group + `,"text":"` + code + `"}}`,
			&provider.ChatMessage{ChatID: "-555", MessageID: 2, Text: code, SenderID: "77"}},
		{"anonymous group admin (sender_chat is the group)",
			`{"update_id":4,"message":{"message_id":3,"from":{"id":1087968824,"is_bot":true,"username":"GroupAnonymousBot"},"sender_chat":` + supergroup + `,"chat":` + supergroup + `,"text":"` + code + `"}}`,
			&provider.ChatMessage{ChatID: "-1009999999999", MessageID: 3, Text: code, SenderTrusted: true}},

		// Everything below must be ignored.
		{"edited channel post", `{"update_id":5,"edited_channel_post":{"message_id":7,"chat":` + channel + `,"text":"` + code + `"}}`, nil},
		{"edited group message", `{"update_id":6,"edited_message":{"message_id":7,"from":` + member + `,"chat":` + supergroup + `,"text":"` + code + `"}}`, nil},
		{"private chat", `{"update_id":7,"message":{"message_id":1,"from":` + member + `,"chat":{"id":77,"type":"private"},"text":"` + code + `"}}`, nil},
		{"message in a channel-typed chat", `{"update_id":8,"message":{"message_id":1,"from":` + member + `,"chat":` + channel + `,"text":"` + code + `"}}`, nil},
		{"channel post in a group-typed chat", `{"update_id":9,"channel_post":{"message_id":1,"chat":` + supergroup + `,"text":"` + code + `"}}`, nil},
		{"group message from a bot", `{"update_id":10,"message":{"message_id":1,"from":{"id":5,"is_bot":true},"chat":` + supergroup + `,"text":"` + code + `"}}`, nil},
		{"group message without a sender", `{"update_id":11,"message":{"message_id":1,"chat":` + supergroup + `,"text":"` + code + `"}}`, nil},
		{"group message sent on behalf of another channel",
			`{"update_id":12,"message":{"message_id":1,"from":` + member + `,"sender_chat":` + channel + `,"chat":` + supergroup + `,"text":"` + code + `"}}`, nil},
		{"automatic forward from the linked channel",
			`{"update_id":13,"message":{"message_id":1,"from":{"id":777000,"is_bot":false},"sender_chat":` + channel + `,"chat":` + supergroup + `,"is_automatic_forward":true,"text":"` + code + `"}}`, nil},
		{"channel post without text (photo)", `{"update_id":14,"channel_post":{"message_id":1,"chat":` + channel + `,"photo":[{"file_id":"x"}],"caption":"` + code + `"}}`, nil},
		{"my_chat_member", `{"update_id":15,"my_chat_member":{"chat":` + channel + `,"from":` + member + `,"date":1,"old_chat_member":{"status":"left"},"new_chat_member":{"status":"administrator"}}}`, nil},
		{"callback query", `{"update_id":16,"callback_query":{"id":"1","from":` + member + `,"data":"x"}}`, nil},
		{"empty object", `{}`, nil},
	}
	for _, tc := range cases {
		got, err := a.ParseUpdate([]byte(tc.raw))
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got  %+v\n want %+v", tc.name, got, tc.want)
		}
	}
	for _, bad := range []string{``, `not json`, `[1,2]`, `{"update_id":"x","message":5}`} {
		if got, err := a.ParseUpdate([]byte(bad)); err == nil || got != nil {
			t.Errorf("malformed %q: got %+v, %v; want an error", bad, got, err)
		}
	}
}

func TestUpdateID(t *testing.T) {
	if id, err := UpdateID([]byte(`{"update_id":123456789012,"message":{}}`)); err != nil || id != 123456789012 {
		t.Fatalf("%d %v", id, err)
	}
	for _, bad := range []string{`{}`, `nope`, `{"update_id":null}`} {
		if _, err := UpdateID([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSecretMatches(t *testing.T) {
	const secret = "s3cret_token-0123456789"
	if !SecretMatches(secret, secret) {
		t.Fatal("the right secret must match")
	}
	for _, got := range []string{"", "wrong", secret + "x", secret[:len(secret)-1], strings.ToUpper(secret), " " + secret} {
		if SecretMatches(got, secret) {
			t.Errorf("%q must not match", got)
		}
	}
	if SecretMatches("", "") || SecretMatches("anything", "") {
		t.Fatal("an unconfigured (empty) secret must never match, not even an empty header")
	}
	for s, want := range map[string]bool{"abc_DEF-123": true, "": false, "has space": false, "semi;colon": false, strings.Repeat("a", 256): true, strings.Repeat("a", 257): false} {
		if ValidWebhookSecret(s) != want {
			t.Errorf("ValidWebhookSecret(%q) != %v", s, want)
		}
	}
}

func TestIsChatAdminAndDeleteMessage(t *testing.T) {
	f := newFakeBot(t)
	f.members[1] = "creator"
	f.members[2] = "administrator"
	f.members[3] = "restricted"
	f.members[4] = "left"
	a := f.adapter(testToken)
	ctx := context.Background()
	for uid, want := range map[string]bool{"1": true, "2": true, "3": false, "4": false, "99": false} {
		got, err := a.IsChatAdmin(ctx, "-1001234567890", uid)
		if err != nil || got != want {
			t.Errorf("user %s: %v %v, want %v", uid, got, err, want)
		}
	}
	if _, err := a.IsChatAdmin(ctx, "@name", "1"); err == nil {
		t.Error("a non-numeric chat id must be rejected")
	}
	if _, err := a.IsChatAdmin(ctx, "-100", "abc"); err == nil {
		t.Error("a non-numeric user id must be rejected")
	}

	if err := a.DeleteMessage(ctx, "-1001234567890", 55); err != nil {
		t.Fatal(err)
	}
	got := f.bodies["deleteMessage"]
	if len(got) != 1 || got[0]["chat_id"] != float64(-1001234567890) || got[0]["message_id"] != float64(55) {
		t.Fatalf("deleteMessage body %v", got)
	}
	if err := a.DeleteMessage(ctx, "nope", 1); err == nil {
		t.Error("a bad chat id must be rejected")
	}
}

func TestBotUsernameIsCached(t *testing.T) {
	f := newFakeBot(t)
	a := f.adapter(testToken)
	for i := 0; i < 3; i++ {
		if got, err := a.BotUsername(context.Background()); err != nil || got != "socialos_bot" {
			t.Fatalf("%q %v", got, err)
		}
	}
	if f.meCalls != 1 {
		t.Fatalf("getMe called %d times, want 1 (cached)", f.meCalls)
	}
	if _, err := New(Config{}).BotUsername(context.Background()); provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("an unconfigured bot must fail permanently: %v", err)
	}
	// A failure is not cached.
	bad := f.adapter("999:WRONG")
	if _, err := bad.BotUsername(context.Background()); err == nil {
		t.Fatal("wrong token accepted")
	}
}

func TestGetUpdatesRequestAndResult(t *testing.T) {
	f := newFakeBot(t)
	f.updates = []string{`{"update_id":10}`, `{"update_id":11}`}
	a := f.adapter(testToken)
	raws, err := a.GetUpdates(context.Background(), 10, 25*time.Second)
	if err != nil || len(raws) != 2 {
		t.Fatalf("%v %v", raws, err)
	}
	if id, _ := UpdateID(raws[1]); id != 11 {
		t.Fatalf("second update id %d", id)
	}
	body := f.bodies["getUpdates"][0]
	if body["offset"] != float64(10) || body["timeout"] != float64(25) || body["limit"] != float64(maxUpdateBatch) {
		t.Fatalf("getUpdates params %v", body)
	}
	if !reflect.DeepEqual(body["allowed_updates"], []any{"channel_post", "message", "my_chat_member"}) {
		t.Fatalf("allowed_updates %v", body["allowed_updates"])
	}
	f.failPoll = []int{409}
	_, err = a.GetUpdates(context.Background(), 0, time.Second)
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.HTTPStatus != 409 || provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("409 must surface as a permanent provider error with its status: %v", err)
	}
	f.failPoll = []int{502}
	if _, err = a.GetUpdates(context.Background(), 0, time.Second); provider.Classify(err) != provider.KindRetryable {
		t.Fatalf("5xx must be retryable: %v", err)
	}
}

func TestWebhookManagement(t *testing.T) {
	f := newFakeBot(t)
	a := f.adapter(testToken)
	ctx := context.Background()
	if err := a.SetWebhook(ctx, "https://api.example.com/api/v1/webhooks/telegram", "sec_ret-1", true); err != nil {
		t.Fatal(err)
	}
	b := f.bodies["setWebhook"][0]
	if b["url"] != "https://api.example.com/api/v1/webhooks/telegram" || b["secret_token"] != "sec_ret-1" || b["drop_pending_updates"] != true ||
		!reflect.DeepEqual(b["allowed_updates"], []any{"channel_post", "message", "my_chat_member"}) {
		t.Fatalf("setWebhook body %v", b)
	}
	for _, bad := range []struct{ url, secret string }{
		{"http://api.example.com/hook", "ok"}, {"api.example.com/hook", "ok"}, {"https://", "ok"}, {"https://a.example/h", ""}, {"https://a.example/h", "bad secret!"},
	} {
		if err := a.SetWebhook(ctx, bad.url, bad.secret, false); err == nil {
			t.Errorf("SetWebhook(%q, %q) accepted", bad.url, bad.secret)
		}
	}
	if len(f.bodies["setWebhook"]) != 1 {
		t.Fatal("invalid input must not reach Telegram")
	}
	if err := a.DeleteWebhook(ctx, false); err != nil {
		t.Fatal(err)
	}
	info, err := a.GetWebhookInfo(ctx)
	if err != nil || info.URL != "https://api.example.com/hook" || info.PendingUpdateCount != 3 {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestLinkInstructionsMentionBotAndCode(t *testing.T) {
	a := New(Config{})
	got := a.LinkInstructions("socialos_bot", "SOS-7KQ2M9XA", 15*time.Minute)
	for _, want := range []string{"@socialos_bot", "SOS-7KQ2M9XA", "15 minutes", "Post messages"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions %q lack %q", got, want)
		}
	}
}
