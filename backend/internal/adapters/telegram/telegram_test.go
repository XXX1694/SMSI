package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
)

const testToken = "123456:SECRET-TOKEN"

type fakeBot struct {
	mu       sync.Mutex
	srv      *httptest.Server
	calls    []string
	captions []string
	files    []string
	status   string // member status of the bot
	rateOnce bool
}

func newFakeBot(t *testing.T) *fakeBot {
	f := &fakeBot{status: "administrator"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func reply(w http.ResponseWriter, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func fail(w http.ResponseWriter, code int, desc string, extra map[string]any) {
	w.WriteHeader(code)
	body := map[string]any{"ok": false, "error_code": code, "description": desc}
	for k, v := range extra {
		body[k] = v
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeBot) handle(w http.ResponseWriter, r *http.Request) {
	prefix := "/bot" + testToken + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		fail(w, 401, "Unauthorized", nil)
		return
	}
	method := strings.TrimPrefix(r.URL.Path, prefix)
	f.mu.Lock()
	f.calls = append(f.calls, method)
	f.mu.Unlock()
	params := map[string]any{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(1 << 20)
		for k, v := range r.MultipartForm.Value {
			params[k] = v[0]
		}
		for k, fhs := range r.MultipartForm.File {
			fh, _ := fhs[0].Open()
			b, _ := io.ReadAll(fh)
			f.mu.Lock()
			f.files = append(f.files, k+"="+string(b))
			f.mu.Unlock()
		}
	} else {
		_ = json.NewDecoder(r.Body).Decode(&params)
	}
	switch method {
	case "getMe":
		reply(w, map[string]any{"id": 42, "is_bot": true, "username": "socialos_bot"})
	case "getChat":
		if params["chat_id"] != "@mychannel" {
			fail(w, 400, "Bad Request: chat not found", nil)
			return
		}
		reply(w, map[string]any{"id": -1001234567890, "type": "channel", "title": "My Channel", "username": "mychannel"})
	case "getChatMember":
		reply(w, map[string]any{"status": f.status, "can_post_messages": true})
	case "sendMessage":
		if f.rateOnce {
			f.rateOnce = false
			fail(w, 429, "Too Many Requests: retry after 3", map[string]any{"parameters": map[string]any{"retry_after": 3}})
			return
		}
		reply(w, map[string]any{"message_id": 10})
	case "sendPhoto", "sendVideo":
		f.mu.Lock()
		f.captions = append(f.captions, params["caption"].(string))
		f.mu.Unlock()
		reply(w, map[string]any{"message_id": 11})
	case "sendMediaGroup":
		reply(w, []map[string]any{{"message_id": 20}, {"message_id": 21}})
	case "deleteMessage", "deleteMessages":
		reply(w, true)
	default:
		fail(w, 404, "Not Found", nil)
	}
}

func (f *fakeBot) adapter(token string) *Adapter {
	return New(Config{BotToken: token, APIBaseURL: f.srv.URL})
}

func media(kind, mime, data string) provider.MediaFile {
	return provider.MediaFile{Kind: kind, MimeType: mime, Size: int64(len(data)), Name: "f", Open: func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(data)), nil
	}}
}

func TestVerifyChat(t *testing.T) {
	f := newFakeBot(t)
	a := f.adapter(testToken)
	prof, err := a.VerifyChat(context.Background(), "https://t.me/mychannel")
	if err != nil || prof.ID != "-1001234567890" || prof.Username != "mychannel" || prof.Metadata["token_ref"] != TokenRef {
		t.Fatalf("verify: %+v %v", prof, err)
	}
	if _, ok := prof.Metadata["bot_token"]; ok {
		t.Fatal("token must not be stored")
	}
	if _, err := a.VerifyChat(context.Background(), "@otherchan"); err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("expected chat not found, got %v", err)
	}
	f.status = "member"
	if _, err := a.VerifyChat(context.Background(), "@mychannel"); err == nil {
		t.Fatal("non-admin in channel must be rejected")
	}
	if _, err := a.VerifyChat(context.Background(), "not a chat!"); err == nil {
		t.Fatal("invalid chat accepted")
	}
}

func TestBadTokenIsRedacted(t *testing.T) {
	f := newFakeBot(t)
	a := f.adapter("999:WRONG-SECRET")
	_, err := a.VerifyChat(context.Background(), "@mychannel")
	if err == nil || strings.Contains(err.Error(), "WRONG-SECRET") {
		t.Fatalf("expected redacted error, got %v", err)
	}
	unreachable := New(Config{BotToken: "777:LEAKME", APIBaseURL: "http://127.0.0.1:1"})
	_, err = unreachable.Publish(context.Background(), provider.PublishRequest{Text: "x", Account: provider.AccountRef{ProviderAccountID: "1"}})
	if err == nil || strings.Contains(err.Error(), "LEAKME") {
		t.Fatalf("token leaked in transport error: %v", err)
	}
	if provider.Classify(err) != provider.KindRetryable {
		t.Fatalf("connection refused should be retryable: %v", err)
	}
	if _, err := New(Config{}).Publish(context.Background(), provider.PublishRequest{Text: "x"}); provider.Classify(err) != provider.KindPermanent {
		t.Fatal("missing token should be permanent")
	}
}

func TestPublishVariants(t *testing.T) {
	f := newFakeBot(t)
	a := f.adapter(testToken)
	ctx := context.Background()
	acc := provider.AccountRef{ProviderAccountID: "-1001234567890", Username: "mychannel", Metadata: map[string]any{"chat_id": "-1001234567890"}}

	res, err := a.Publish(ctx, provider.PublishRequest{Text: "hello", Account: acc})
	if err != nil || res.ExternalID != "-1001234567890:10" || res.URL != "https://t.me/mychannel/10" {
		t.Fatalf("text: %+v %v", res, err)
	}
	res, err = a.Publish(ctx, provider.PublishRequest{Text: "pic", Account: acc, Media: []provider.MediaFile{media("image", "image/png", "IMG")}})
	if err != nil || res.ExternalID != "-1001234567890:11" || f.captions[0] != "pic" || f.files[0] != "photo=IMG" {
		t.Fatalf("photo: %+v %v %v", res, err, f.files)
	}
	res, err = a.Publish(ctx, provider.PublishRequest{Text: "album", Account: acc, Media: []provider.MediaFile{
		media("image", "image/jpeg", "A"), media("video", "video/mp4", "B")}})
	if err != nil || res.ExternalID != "-1001234567890:20,21" {
		t.Fatalf("album: %+v %v", res, err)
	}
	if err := a.Delete(ctx, provider.DeleteRequest{ExternalID: res.ExternalID}); err != nil {
		t.Fatal(err)
	}
	if err := a.Delete(ctx, provider.DeleteRequest{ExternalID: "-100:10"}); err != nil {
		t.Fatal(err)
	}
	if f.calls[len(f.calls)-2] != "deleteMessages" || f.calls[len(f.calls)-1] != "deleteMessage" {
		t.Fatalf("delete calls %v", f.calls)
	}
}

func TestPublishErrorsAndLimits(t *testing.T) {
	f := newFakeBot(t)
	a := f.adapter(testToken)
	ctx := context.Background()
	acc := provider.AccountRef{ProviderAccountID: "-100"}
	f.rateOnce = true
	_, err := a.Publish(ctx, provider.PublishRequest{Text: "hi", Account: acc})
	if provider.Classify(err) != provider.KindRetryable || provider.RetryAfterOf(err).Seconds() != 3 {
		t.Fatalf("429: %v", err)
	}
	cases := []provider.PublishRequest{
		{Text: "", Account: acc},
		{Text: strings.Repeat("a", 4097), Account: acc},
		{Text: strings.Repeat("a", 1025), Account: acc, Media: []provider.MediaFile{media("image", "image/png", "x")}},
		{Text: "x", Account: acc, Media: []provider.MediaFile{{Kind: "video", Size: 51 << 20}}},
	}
	for i, c := range cases {
		if _, err := a.Publish(ctx, c); provider.Classify(err) != provider.KindPermanent {
			t.Errorf("case %d: expected permanent, got %v", i, err)
		}
	}
}

func TestNormalizeChatAndURLs(t *testing.T) {
	for in, want := range map[string]string{"@chan_name": "@chan_name", "chan_name": "@chan_name", "t.me/chan_name/": "@chan_name", "-100123": "-100123"} {
		if got, ok := NormalizeChat(in); !ok || got != want {
			t.Errorf("NormalizeChat(%q)=%q", in, got)
		}
	}
	if messageURL("", "-1009876", 5) != "https://t.me/c/9876/5" || messageURL("", "123", 5) != "" {
		t.Fatal("message url wrong")
	}
	if _, _, err := parseExternalID("garbage"); err == nil {
		t.Fatal("bad id accepted")
	}
}
