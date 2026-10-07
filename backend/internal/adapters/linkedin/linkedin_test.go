package linkedin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/socialos/backend/internal/adapters/provider"
)

type fakeLinkedIn struct {
	mu        sync.Mutex
	srv       *httptest.Server
	posts     []postBody
	uploads   int
	failPosts int // respond 503 this many times
	tokenForm url.Values
}

func newFake(t *testing.T) *fakeLinkedIn {
	f := &fakeLinkedIn{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/v2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.tokenForm = r.PostForm
		f.mu.Unlock()
		switch {
		case r.PostForm.Get("code") == "bad" || r.PostForm.Get("refresh_token") == "revoked":
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"The token is invalid"}`))
		default:
			_, _ = w.Write([]byte(`{"access_token":"at-123","expires_in":5184000,"refresh_token":"rt-1","refresh_token_expires_in":31536000,"scope":"openid,profile,email,w_member_social"}`))
		}
	})
	mux.HandleFunc("GET /v2/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-123" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"Invalid access token","status":401}`))
			return
		}
		_, _ = w.Write([]byte(`{"sub":"abc123","name":"Ada Lovelace","email":"ada@example.com","picture":"https://img/x.png"}`))
	})
	mux.HandleFunc("POST /rest/images", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "initializeUpload" || r.Header.Get("LinkedIn-Version") == "" {
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"uploadUrl": f.srv.URL + "/upload/img1", "image": "urn:li:image:img1"}})
	})
	mux.HandleFunc("PUT /upload/img1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "PNGDATA" {
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.uploads++
		f.mu.Unlock()
		w.WriteHeader(201)
	})
	mux.HandleFunc("POST /rest/posts", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failPosts > 0 {
			f.failPosts--
			w.WriteHeader(503)
			return
		}
		if r.Header.Get("X-Restli-Protocol-Version") != "2.0.0" || r.Header.Get("Authorization") != "Bearer at-123" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"bad auth"}`))
			return
		}
		var pb postBody
		_ = json.NewDecoder(r.Body).Decode(&pb)
		f.posts = append(f.posts, pb)
		w.Header().Set("x-restli-id", "urn:li:share:777")
		w.WriteHeader(201)
	})
	mux.HandleFunc("DELETE /rest/posts/{urn}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("urn") != "urn:li:share:777" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(204)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLinkedIn) adapter() *Adapter {
	return New(Config{ClientID: "cid", ClientSecret: "secret", UsePKCE: true,
		AuthURL: f.srv.URL + "/oauth/v2/authorization", TokenURL: f.srv.URL + "/oauth/v2/accessToken", APIBaseURL: f.srv.URL})
}

func TestAuthorizeURL(t *testing.T) {
	a := New(Config{ClientID: "cid", ClientSecret: "s", UsePKCE: true})
	u, _ := url.Parse(a.AuthorizeURL(provider.AuthorizeParams{State: "st", CodeChallenge: "cc", RedirectURI: "https://app/cb"}))
	q := u.Query()
	if u.Host != "www.linkedin.com" || q.Get("state") != "st" || q.Get("client_id") != "cid" ||
		q.Get("code_challenge") != "cc" || q.Get("code_challenge_method") != "S256" || !strings.Contains(q.Get("scope"), "w_member_social") {
		t.Fatalf("bad url %s", u)
	}
	if !a.Configured() || New(Config{}).Configured() {
		t.Fatal("configured wrong")
	}
}

func TestExchangeProfileAndRefresh(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	ctx := context.Background()
	tok, err := a.Exchange(ctx, "good", "verifier", "https://app/cb")
	if err != nil || tok.AccessToken != "at-123" || tok.ExpiresAt == nil || tok.RefreshExpiresAt == nil || len(tok.Scopes) != 4 {
		t.Fatalf("exchange: %+v %v", tok, err)
	}
	if f.tokenForm.Get("code_verifier") != "verifier" || f.tokenForm.Get("client_secret") != "secret" {
		t.Fatal("token form missing fields")
	}
	if _, err := a.Exchange(ctx, "bad", "v", "x"); provider.Classify(err) != provider.KindPermanent {
		t.Fatalf("bad code should be permanent: %v", err)
	}
	prof, err := a.Profile(ctx, "at-123")
	if err != nil || prof.ID != "abc123" || prof.DisplayName != "Ada Lovelace" {
		t.Fatalf("profile %+v %v", prof, err)
	}
	if _, err := a.Profile(ctx, "wrong"); provider.Classify(err) != provider.KindAuth {
		t.Fatalf("expected auth error, got %v", err)
	}
	if _, err := a.Refresh(ctx, "rt-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Refresh(ctx, "revoked"); provider.Classify(err) != provider.KindAuth {
		t.Fatalf("revoked refresh should be auth: %v", err)
	}
	if _, err := a.Refresh(ctx, ""); provider.Classify(err) != provider.KindAuth {
		t.Fatal("empty refresh should be auth")
	}
}

func TestPublishTextAndImage(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	img := provider.MediaFile{Kind: "image", MimeType: "image/png", Size: 7, Open: func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("PNGDATA")), nil
	}}
	res, err := a.Publish(context.Background(), provider.PublishRequest{
		IdempotencyKey: "k1", AccessToken: "at-123", Text: "Hello (world) #golang",
		Account: provider.AccountRef{ProviderAccountID: "abc123"}, Media: []provider.MediaFile{img},
	})
	if err != nil || res.ExternalID != "urn:li:share:777" || !strings.Contains(res.URL, "urn:li:share:777") {
		t.Fatalf("publish %+v %v", res, err)
	}
	if f.uploads != 1 || len(f.posts) != 1 {
		t.Fatalf("uploads=%d posts=%d", f.uploads, len(f.posts))
	}
	p := f.posts[0]
	if p.Author != "urn:li:person:abc123" || p.Commentary != `Hello \(world\) {hashtag|\#|golang}` || p.Visibility != "PUBLIC" {
		t.Fatalf("bad body %+v", p)
	}
	if p.Content["media"].(map[string]any)["id"] != "urn:li:image:img1" {
		t.Fatalf("media missing %+v", p.Content)
	}
}

func TestPublishErrors(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	ctx := context.Background()
	acct := provider.AccountRef{ProviderAccountID: "abc123"}
	f.failPosts = 1
	_, err := a.Publish(ctx, provider.PublishRequest{AccessToken: "at-123", Text: "hi", Account: acct})
	if provider.Classify(err) != provider.KindRetryable {
		t.Fatalf("503 should be retryable: %v", err)
	}
	_, err = a.Publish(ctx, provider.PublishRequest{AccessToken: "expired", Text: "hi", Account: acct})
	if provider.Classify(err) != provider.KindAuth || strings.Contains(err.Error(), "expired\"") {
		t.Fatalf("401 should be auth: %v", err)
	}
	_, err = a.Publish(ctx, provider.PublishRequest{AccessToken: "at-123", Text: strings.Repeat("a", 3001), Account: acct})
	if provider.Classify(err) != provider.KindPermanent {
		t.Fatal("too long should be permanent")
	}
	_, err = a.Publish(ctx, provider.PublishRequest{AccessToken: "at-123", Text: "v", Account: acct, Media: []provider.MediaFile{{Kind: "video"}}})
	if provider.Classify(err) != provider.KindUnsupported {
		t.Fatal("video should be unsupported")
	}
}

func TestDelete(t *testing.T) {
	f := newFake(t)
	a := f.adapter()
	if err := a.Delete(context.Background(), provider.DeleteRequest{AccessToken: "at-123", ExternalID: "urn:li:share:777"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Delete(context.Background(), provider.DeleteRequest{AccessToken: "at-123", ExternalID: "urn:li:share:gone"}); err != nil {
		t.Fatal("404 should be treated as deleted")
	}
}

func TestFormatCommentary(t *testing.T) {
	cases := map[string]string{
		"plain":          "plain",
		"a_b *c* @d":     `a\_b \*c\* \@d`,
		"#tag and x#y":   `{hashtag|\#|tag} and x\#y`,
		"# alone":        `\# alone`,
		"Привет #мир":    `Привет {hashtag|\#|мир}`,
		`back\slash [x]`: `back\\slash \[x\]`,
		"<html>{json}|~": `\<html\>\{json\}\|\~`,
	}
	for in, want := range cases {
		if got := FormatCommentary(in); got != want {
			t.Errorf("FormatCommentary(%q)=%q want %q", in, got, want)
		}
	}
}
