package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/providertest"
)

const (
	hookID    = "112233445566778899"
	hookToken = "tok_SECRET-abcdefghijklmnopqrstuvwxyz0123456789" // gitleaks:allow (fake test value)
	hookURL   = "https://discord.com/api/webhooks/" + hookID + "/" + hookToken
	hookPath  = "/api/webhooks/" + hookID + "/" + hookToken
)

type seen struct {
	method, path, query, ctype string
	body                       []byte
}

// fake is a Discord stand-in. handler decides the answer; every request is recorded.
type fake struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []seen
}

func newFake(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, n int)) *fake {
	t.Helper()
	f := &fake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Content-Type"), b})
		n := len(f.reqs)
		f.mu.Unlock()
		handler(w, r, n)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) adapter(timeout time.Duration) *Adapter {
	c := f.srv.Client()
	c.Timeout = timeout
	return New(Config{APIBaseURL: f.srv.URL, HTTPClient: c})
}

func (f *fake) last() seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

func reply(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func webhookInfo(w http.ResponseWriter) {
	// Discord includes the token in GET answers; the adapter must not keep it.
	reply(w, 200, fmt.Sprintf(`{"id":%q,"type":1,"name":"Release bot","channel_id":"777","guild_id":"555","token":%q}`, hookID, hookToken))
}

func okFake(t *testing.T) *fake {
	return newFake(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		switch r.Method {
		case http.MethodGet:
			webhookInfo(w)
		case http.MethodPost:
			reply(w, 200, `{"id":"999000111","channel_id":"777"}`)
		default:
			w.WriteHeader(204)
		}
	})
}

func publishReq(text string, media ...provider.MediaFile) provider.PublishRequest {
	return provider.PublishRequest{Text: text, Media: media, AccessToken: hookURL,
		Account: provider.AccountRef{Metadata: map[string]any{"guild_id": "555", "channel_id": "777"}}}
}

func image(name string, data string) provider.MediaFile {
	return provider.MediaFile{Kind: "image", MimeType: "image/png", Size: int64(len(data)), Name: name,
		Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(data)), nil }}
}

func noSecret(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	for _, s := range []string{err.Error(), provider.SafeMessage(err), fmt.Sprintf("%+v", err)} {
		if strings.Contains(s, hookToken) || strings.Contains(s, hookPath) {
			t.Fatalf("error leaks the webhook: %s", s)
		}
	}
}

func TestContract(t *testing.T) { providertest.Contract(t, New(Config{})) }

func TestCapabilitiesAreHonest(t *testing.T) {
	c := New(Config{}).Capabilities()
	if c.ConnectMethod != provider.ConnectToken || c.MaxTextLength != 2000 || !c.CanDelete || c.RequiresTitle || c.CanPublishVideo || c.SafeToRetryAfterUnknown {
		t.Fatalf("capabilities: %+v", c)
	}
	if len(c.ConnectFields) != 1 || c.ConnectFields[0].Name != "webhook_url" || !c.ConnectFields[0].Secret {
		t.Fatalf("webhook_url must be a Secret field: %+v", c.ConnectFields)
	}
	if blob, _ := json.Marshal(c); !strings.Contains(string(blob), `"secret":true`) {
		t.Fatalf("the secret flag is missing from the capabilities JSON: %s", blob)
	}
}

func TestVerifyReturnsOnlyNonSecretMetadata(t *testing.T) {
	f := okFake(t)
	prof, secret, err := f.adapter(5*time.Second).Verify(context.Background(), map[string]string{"webhook_url": hookURL + "?wait=true"})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.last(); got.method != "GET" || got.path != hookPath || got.query != "" {
		t.Fatalf("request: %+v", got)
	}
	if prof.ID != hookID || prof.Username != "Release bot" || prof.Metadata["channel_id"] != "777" || prof.Metadata["guild_id"] != "555" || prof.Metadata["webhook_id"] != hookID {
		t.Fatalf("profile: %+v", prof)
	}
	blob, _ := json.Marshal(prof)
	if strings.Contains(string(blob), hookToken) {
		t.Fatalf("profile leaks the token: %s", blob)
	}
	if secret != hookURL {
		t.Fatalf("the credential must be the canonical webhook URL, got %q", secret)
	}
}

func TestVerifyRejectsBadURLsWithoutCallingOut(t *testing.T) {
	f := okFake(t)
	a := f.adapter(time.Second)
	for name, raw := range map[string]string{
		"other host":  "https://evil.example.com/api/webhooks/" + hookID + "/" + hookToken,
		"lookalike":   "https://discord.com.evil.example/api/webhooks/" + hookID + "/" + hookToken,
		"http":        "http://discord.com/api/webhooks/" + hookID + "/" + hookToken,
		"userinfo":    "https://discord.com@evil.example/api/webhooks/" + hookID + "/" + hookToken,
		"no token":    "https://discord.com/api/webhooks/" + hookID,
		"bad id":      "https://discord.com/api/webhooks/abc/" + hookToken,
		"extra path":  "https://discord.com/api/webhooks/" + hookID + "/" + hookToken + "/slack",
		"empty":       "",
		"port":        "https://discord.com:8443/api/webhooks/" + hookID + "/" + hookToken,
		"traversal":   "https://discord.com/api/webhooks/" + hookID + "/..%2f..%2fusers",
		"wrong route": "https://discord.com/api/users/" + hookID + "/" + hookToken,
	} {
		_, _, err := a.Verify(context.Background(), map[string]string{"webhook_url": raw})
		if provider.Classify(err) != provider.KindPermanent {
			t.Errorf("%s: want a permanent error, got %v", name, err)
		}
		if err != nil && raw != "" && strings.Contains(err.Error(), raw) {
			t.Errorf("%s: the error repeats the input", name)
		}
	}
	if len(f.reqs) != 0 {
		t.Fatalf("no request may be made for a bad URL, got %d", len(f.reqs))
	}
}

func TestParseAcceptsKnownSpellings(t *testing.T) {
	for _, raw := range []string{
		hookURL,
		"https://discordapp.com/api/webhooks/" + hookID + "/" + hookToken,
		"https://DISCORD.com/api/v10/webhooks/" + hookID + "/" + hookToken + "/?thread_id=1#x",
		" " + hookURL + " ",
	} {
		w, err := parseWebhookURL(raw)
		if err != nil || w.id != hookID || w.token != hookToken {
			t.Errorf("%q: %+v %v", raw, w, err)
		}
	}
}

func TestVerifyRefusesAnAnswerForAnotherWebhook(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, _ *http.Request, _ int) { reply(w, 200, `{"id":"1","name":"x"}`) })
	_, _, err := f.adapter(time.Second).Verify(context.Background(), map[string]string{"webhook_url": hookURL})
	if provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("got %v", err)
	}
}

func TestPublishTextShape(t *testing.T) {
	f := okFake(t)
	res, err := f.adapter(5*time.Second).Publish(context.Background(), publishReq("Hello @everyone"))
	if err != nil {
		t.Fatal(err)
	}
	got := f.last()
	if got.method != "POST" || got.path != hookPath || got.query != "wait=true" || got.ctype != "application/json" {
		t.Fatalf("request: %+v", got)
	}
	var body map[string]any
	_ = json.Unmarshal(got.body, &body)
	if body["content"] != "Hello @everyone" {
		t.Fatalf("body: %s", got.body)
	}
	if am, _ := body["allowed_mentions"].(map[string]any); am == nil || len(am["parse"].([]any)) != 0 {
		t.Fatalf("mentions must not ping: %s", got.body)
	}
	if res.ExternalID != "999000111" || res.URL != "https://discord.com/channels/555/777/999000111" {
		t.Fatalf("result: %+v", res)
	}
}

func TestPublishWithImagesIsMultipart(t *testing.T) {
	f := okFake(t)
	_, err := f.adapter(5*time.Second).Publish(context.Background(), publishReq("Look", image("../a b.png", "PNGDATA1"), image("c.png", "PNGDATA2")))
	if err != nil {
		t.Fatal(err)
	}
	got := f.last()
	mt, params, err := mime.ParseMediaType(got.ctype)
	if err != nil || mt != "multipart/form-data" {
		t.Fatalf("content type %q", got.ctype)
	}
	mr := multipart.NewReader(bytes.NewReader(got.body), params["boundary"])
	parts := map[string]string{}
	files := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p)
		parts[p.FormName()] = string(b)
		files[p.FormName()] = p.FileName()
	}
	var pj payload
	if err := json.Unmarshal([]byte(parts["payload_json"]), &pj); err != nil || pj.Content != "Look" || len(pj.Attachments) != 2 {
		t.Fatalf("payload_json: %q %v", parts["payload_json"], err)
	}
	if parts["files[0]"] != "PNGDATA1" || parts["files[1]"] != "PNGDATA2" {
		t.Fatalf("files: %v", parts)
	}
	if files["files[0]"] != "1-a b.png" || files["files[1]"] != "2-c.png" || pj.Attachments[0].Filename != "1-a b.png" {
		t.Fatalf("file names must be safe base names: %v %+v", files, pj.Attachments)
	}
}

func TestPublishValidatesBeforeSending(t *testing.T) {
	f := okFake(t)
	a := f.adapter(time.Second)
	big := image("big.png", "x")
	big.Size = MaxFileBytes + 1
	video := image("v.mp4", "x")
	video.Kind = "video"
	many := make([]provider.MediaFile, MaxMedia+1)
	for i := range many {
		many[i] = image("a.png", "x")
	}
	for name, tc := range map[string]struct {
		req  provider.PublishRequest
		want provider.Kind
	}{
		"too long":  {publishReq(strings.Repeat("я", 2001)), provider.KindPermanent},
		"empty":     {publishReq("  "), provider.KindPermanent},
		"too big":   {publishReq("x", big), provider.KindPermanent},
		"too many":  {publishReq("x", many...), provider.KindPermanent},
		"video":     {publishReq("x", video), provider.KindUnsupported},
		"bad token": {provider.PublishRequest{Text: "x", AccessToken: "nonsense"}, provider.KindAuth},
	} {
		_, err := a.Publish(context.Background(), tc.req)
		if provider.Classify(err) != tc.want {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := a.Publish(context.Background(), publishReq(strings.Repeat("я", 2000))); err != nil {
		t.Errorf("2000 characters must pass: %v", err)
	}
	if len(f.reqs) != 1 {
		t.Fatalf("only the valid post may reach Discord, got %d requests", len(f.reqs))
	}
}

func TestErrorMapping(t *testing.T) {
	type tc struct {
		status  int
		body    string
		headers map[string]string
		want    provider.Kind
		retry   time.Duration
	}
	for name, c := range map[string]tc{
		"401":                 {401, `{"message":"401: Unauthorized","code":0}`, nil, provider.KindAuth, 0},
		"404 unknown webhook": {404, `{"message":"Unknown Webhook","code":10015}`, nil, provider.KindAuth, 0},
		"404 other":           {404, `{"message":"Not Found","code":0}`, nil, provider.KindPermanent, 0},
		"400":                 {400, `{"message":"Invalid Form Body","code":50035}`, nil, provider.KindPermanent, 0},
		"403":                 {403, `{"message":"Missing Permissions","code":50013}`, nil, provider.KindPermanent, 0},
		"429 body":            {429, `{"message":"You are being rate limited.","retry_after":1.5,"global":false}`, nil, provider.KindRetryable, 1500 * time.Millisecond},
		"429 reset-after":     {429, `{}`, map[string]string{"X-RateLimit-Reset-After": "2.25"}, provider.KindRetryable, 2250 * time.Millisecond},
		"429 retry-after":     {429, `not json`, map[string]string{"Retry-After": "3"}, provider.KindRetryable, 3 * time.Second},
		"429 nothing":         {429, ``, nil, provider.KindRetryable, time.Second},
		"500":                 {500, `oops`, nil, provider.KindRetryable, 0},
		"503":                 {503, ``, nil, provider.KindRetryable, 0},
	} {
		f := newFake(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
			for k, v := range c.headers {
				w.Header().Set(k, v)
			}
			reply(w, c.status, c.body)
		})
		_, perr := f.adapter(5*time.Second).Publish(context.Background(), publishReq("x"))
		_, _, verr := f.adapter(5*time.Second).Verify(context.Background(), map[string]string{"webhook_url": hookURL})
		for _, err := range []error{perr, verr} {
			if provider.Classify(err) != c.want || provider.RetryAfterOf(err) != c.retry {
				t.Errorf("%s: kind %v retry %v (%v)", name, provider.Classify(err), provider.RetryAfterOf(err), err)
			}
			noSecret(t, err)
		}
	}
}

func TestAnErrorMessageNeverRepeatsAnEchoedToken(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		reply(w, 400, fmt.Sprintf(`{"message":"bad %s here","code":50035}`, hookToken))
	})
	_, err := f.adapter(time.Second).Publish(context.Background(), publishReq("x"))
	noSecret(t, err)
	if err == nil || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("got %v", err)
	}
}

func TestTimeoutAfterSendIsUnknownAndLeaksNothing(t *testing.T) {
	release := make(chan struct{})
	f := newFake(t, func(_ http.ResponseWriter, r *http.Request, _ int) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })
	_, err := f.adapter(80*time.Millisecond).Publish(context.Background(), publishReq("x"))
	if provider.Classify(err) != provider.KindUnknown {
		t.Fatalf("got %v (%v)", err, provider.Classify(err))
	}
	noSecret(t, err)
	if New(Config{}).Capabilities().SafeToRetryAfterUnknown {
		t.Fatal("Discord has no idempotency key: re-sending would duplicate")
	}
	if _, ok := any(New(Config{})).(provider.Lookuper); ok {
		t.Fatal("Discord cannot look a message up by idempotency key")
	}
}

func TestConnectionRefusedIsRetryableAndLeaksNothing(t *testing.T) {
	f := okFake(t)
	a := f.adapter(time.Second)
	f.srv.Close()
	_, err := a.Publish(context.Background(), publishReq("x"))
	if provider.Classify(err) != provider.KindRetryable {
		t.Fatalf("got %v", err)
	}
	noSecret(t, err)
}

func TestMissingMessageIdIsUnknown(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, _ *http.Request, _ int) { reply(w, 200, `{}`) })
	_, err := f.adapter(time.Second).Publish(context.Background(), publishReq("x"))
	if provider.Classify(err) != provider.KindUnknown {
		t.Fatalf("got %v", err)
	}
}

func TestDelete(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, _ *http.Request, _ int) { w.WriteHeader(204) })
	a := f.adapter(time.Second)
	req := provider.DeleteRequest{AccessToken: hookURL, ExternalID: "999000111"}
	if err := a.Delete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := f.last(); got.method != "DELETE" || got.path != hookPath+"/messages/999000111" || got.query != "" {
		t.Fatalf("request: %+v", got)
	}
	if err := a.Delete(context.Background(), provider.DeleteRequest{AccessToken: hookURL, ExternalID: "../x"}); provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("a message id must be numeric: %v", err)
	}
}

func TestDeleteErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   provider.Kind
		ok     bool
	}{
		"already gone": {404, `{"message":"Unknown Message","code":10008}`, provider.KindPermanent, true},
		"webhook gone": {404, `{"message":"Unknown Webhook","code":10015}`, provider.KindAuth, false},
		"unauthorized": {401, `{}`, provider.KindAuth, false},
		"server":       {502, ``, provider.KindRetryable, false},
	} {
		f := newFake(t, func(w http.ResponseWriter, _ *http.Request, _ int) { reply(w, tc.status, tc.body) })
		err := f.adapter(time.Second).Delete(context.Background(), provider.DeleteRequest{AccessToken: hookURL, ExternalID: "999000111"})
		if tc.ok != (err == nil) || (err != nil && provider.Classify(err) != tc.want) {
			t.Errorf("%s: got %v", name, err)
		}
		noSecret(t, err)
	}
}

func TestDefaultClientOnlyReachesDiscordHosts(t *testing.T) {
	_, err := New(Config{}).http.Get("https://example.com/api/webhooks/" + hookID + "/" + hookToken)
	if err == nil || !strings.Contains(err.Error(), "host is not allowed") {
		t.Fatalf("got %v", err)
	}
}
