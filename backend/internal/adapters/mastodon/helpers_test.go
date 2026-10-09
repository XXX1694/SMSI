package mastodon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// testToken is a fake credential; it must never show up in errors or profiles.
const testToken = "tok-fake-0123456789abcdef" // gitleaks:allow (fake test value)

// instanceHost is a name the httptest certificate is valid for.
const instanceHost = "example.com"

type seen struct {
	Method, Path string
	Header       http.Header
	Body         string
}

// fake is a Mastodon server over TLS. Routes are "METHOD /path".
type fake struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	reqs   []seen
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{t: t, routes: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, seen{r.Method, r.URL.Path, r.Header.Clone(), string(b)})
		h := f.routes[r.Method+" "+r.URL.Path]
		f.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(b)))
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) on(route string, h http.HandlerFunc) { f.routes[route] = h }

func (f *fake) requests(route string) []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []seen
	for _, r := range f.reqs {
		if r.Method+" "+r.Path == route {
			out = append(out, r)
		}
	}
	return out
}

// client trusts the test certificate and dials the test server whatever the
// host name is. It replaces the production SSRF-safe client, which (rightly)
// refuses loopback; the guard itself is tested in safehttp and in TestDefaultClientBlocksPrivateInstances.
func (f *fake) client(timeout time.Duration) *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(f.srv.Certificate())
	addr := f.srv.Listener.Addr().String()
	return &http.Client{Timeout: timeout, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
}

func (f *fake) adapter() *Adapter {
	return New(Config{HTTPClient: f.client(5 * time.Second), MediaPollInterval: time.Millisecond, MediaProcessingTimeout: 2 * time.Second,
		Now: func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }})
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// standardRoutes makes the instance answer whoami and the v2 instance document.
func (f *fake) standardRoutes() {
	f.on("GET /api/v1/accounts/verify_credentials", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			writeJSON(w, 401, `{"error":"The access token is invalid"}`)
			return
		}
		writeJSON(w, 200, `{"id":"1234","username":"alice","display_name":"Alice","avatar":"https://example.com/a.png"}`)
	})
	f.on("GET /api/v2/instance", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"configuration":{"statuses":{"max_characters":800,"max_media_attachments":3},"media_attachments":{"image_size_limit":8388608}}}`)
	})
}

func account() provider.AccountRef {
	return provider.AccountRef{ID: "acc-1", ProviderAccountID: instanceHost + ":1234", Metadata: map[string]any{"host": instanceHost}}
}

func publishReq(text string, media ...provider.MediaFile) provider.PublishRequest {
	return provider.PublishRequest{IdempotencyKey: "target-uuid-1", Account: account(), AccessToken: testToken, Text: text, Media: media}
}

func imageFile(data string) provider.MediaFile {
	return provider.MediaFile{Kind: "image", MimeType: "image/png", Size: int64(len(data)), Name: "pic.png",
		Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(data)), nil }}
}
