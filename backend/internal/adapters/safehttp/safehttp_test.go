package safehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testClient builds a safe client that may reach the loopback test server and
// trusts its certificate; names resolve through the supplied table.
func testClient(t *testing.T, srv *httptest.Server, names map[string]string, mutate func(*SafeClientConfig)) (*http.Client, *int32) {
	t.Helper()
	var lookups int32
	cfg := SafeClientConfig{
		Timeout: 5 * time.Second,
		Lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			atomic.AddInt32(&lookups, 1)
			ip, ok := names[host]
			if !ok {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return []netip.Addr{netip.MustParseAddr(ip)}, nil
		},
		blocked: func(netip.Addr) bool { return false },
	}
	if mutate != nil {
		mutate(&cfg)
	}
	c := NewSafeClient(cfg)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	c.Transport.(*guardTransport).next.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return c, &lookups
}

func hostURL(t *testing.T, srv *httptest.Server, host, path string) string {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	return "https://" + host + ":" + u.Port() + path
}

func get(c *http.Client, u string) (string, error) {
	resp, err := c.Get(u)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func TestSafeClientReachesAllowedHost(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	c, _ := testClient(t, srv, map[string]string{"good.example.com": "127.0.0.1"}, nil)
	if body, err := get(c, hostURL(t, srv, "good.example.com", "/")); err != nil || body != "ok" {
		t.Fatalf("got %q, %v", body, err)
	}
}

func TestSafeClientRefusesBlockedDestinations(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("server must not be reached") }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	names := map[string]string{
		"loop.test": "127.0.0.1", "priv.test": "10.1.2.3", "meta.test": "169.254.169.254", "cgnat.test": "100.64.0.9",
		"v6loop.test": "::1", "mapped.test": "::ffff:127.0.0.1", "multi.test": "224.0.0.1", "any.test": "0.0.0.0",
	}
	// Default address guard: no override of blocked.
	c, _ := testClient(t, srv, names, func(cfg *SafeClientConfig) { cfg.blocked = nil })
	for host := range names {
		if _, err := get(c, "https://"+host+":"+u.Port()+"/"); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s: want ErrBlockedAddress, got %v", host, err)
		}
	}
	for _, lit := range []string{"127.0.0.1", "[::1]", "[::ffff:10.0.0.1]", "169.254.169.254", "0.0.0.0"} {
		if _, err := get(c, "https://"+lit+":"+u.Port()+"/"); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("literal %s: want ErrBlockedAddress, got %v", lit, err)
		}
	}
}

func TestSafeClientRebindingMixedAnswerIsRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("server must not be reached") }))
	defer srv.Close()
	var calls int32
	c := NewSafeClient(SafeClientConfig{Lookup: func(context.Context, string) ([]netip.Addr, error) {
		// A rebinding resolver: a public answer first, then the internal one.
		if atomic.AddInt32(&calls, 1) == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.5")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}})
	for i := 0; i < 2; i++ {
		if _, err := get(c, hostURL(t, srv, "rebind.test", "/")); !errors.Is(err, ErrBlockedAddress) {
			t.Fatalf("attempt %d: want ErrBlockedAddress, got %v", i, err)
		}
	}
}

func TestSafeClientResolvesOncePerConnection(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	c, lookups := testClient(t, srv, map[string]string{"good.example.com": "127.0.0.1"}, nil)
	if _, err := get(c, hostURL(t, srv, "good.example.com", "/")); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(lookups); n != 1 {
		t.Fatalf("the vetted address must be dialed directly, got %d lookups", n)
	}
}

func TestSafeClientHTTPSOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("must not be reached") }))
	defer srv.Close()
	c, _ := testClient(t, httptest.NewTLSServer(http.NotFoundHandler()), map[string]string{"good.example.com": "127.0.0.1"}, nil)
	u, _ := url.Parse(srv.URL)
	if _, err := get(c, "http://good.example.com:"+u.Port()+"/"); !errors.Is(err, ErrInsecureScheme) {
		t.Fatalf("want ErrInsecureScheme, got %v", err)
	}
}

func TestSafeClientRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "https://"+r.Host+"/final", http.StatusFound)
		case "/other":
			http.Redirect(w, r, hostURL(t, srv, "evil.example.com", "/final"), http.StatusFound)
		case "/downgrade":
			http.Redirect(w, r, "http://"+r.Host+"/final", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "https://"+r.Host+"/loop", http.StatusFound)
		default:
			_, _ = w.Write([]byte("final"))
		}
	}))
	defer srv.Close()
	c, _ := testClient(t, srv, map[string]string{"good.example.com": "127.0.0.1", "evil.example.com": "127.0.0.1"}, nil)
	if body, err := get(c, hostURL(t, srv, "good.example.com", "/same")); err != nil || body != "final" {
		t.Fatalf("same-host redirect: %q, %v", body, err)
	}
	if _, err := get(c, hostURL(t, srv, "good.example.com", "/other")); !errors.Is(err, ErrRedirectNotAllow) {
		t.Fatalf("other host: want ErrRedirectNotAllow, got %v", err)
	}
	if _, err := get(c, hostURL(t, srv, "good.example.com", "/downgrade")); !errors.Is(err, ErrInsecureScheme) {
		t.Fatalf("downgrade: want ErrInsecureScheme, got %v", err)
	}
	if _, err := get(c, hostURL(t, srv, "good.example.com", "/loop")); !errors.Is(err, ErrRedirectNotAllow) {
		t.Fatalf("loop: want ErrRedirectNotAllow, got %v", err)
	}
}

func TestSafeClientResponseCap(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("a", 64<<10))
		switch r.URL.Path {
		case "/exact":
			w.Header().Set("Content-Length", "1048576")
			for i := 0; i < 16; i++ {
				_, _ = w.Write(chunk)
			}
		case "/declared":
			w.Header().Set("Content-Length", "2097152")
			_, _ = w.Write(chunk)
		default: // chunked, no declared length
			for i := 0; i < 32; i++ {
				_, _ = w.Write(chunk)
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer srv.Close()
	c, _ := testClient(t, srv, map[string]string{"good.example.com": "127.0.0.1"}, nil)
	if body, err := get(c, hostURL(t, srv, "good.example.com", "/exact")); err != nil || len(body) != MaxResponseBytes {
		t.Fatalf("exactly the cap must pass: %d bytes, %v", len(body), err)
	}
	if _, err := get(c, hostURL(t, srv, "good.example.com", "/declared")); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("declared length: want ErrResponseTooLarge, got %v", err)
	}
	if body, err := get(c, hostURL(t, srv, "good.example.com", "/stream")); !errors.Is(err, ErrResponseTooLarge) || len(body) > MaxResponseBytes {
		t.Fatalf("streamed: want ErrResponseTooLarge and at most the cap, got %d bytes, %v", len(body), err)
	}
}

func TestSafeClientAllowedHosts(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	c, _ := testClient(t, srv, map[string]string{"api.example.com": "127.0.0.1", "other.example.com": "127.0.0.1"},
		func(cfg *SafeClientConfig) { cfg.AllowedHosts = []string{"API.example.com"} })
	if _, err := get(c, hostURL(t, srv, "api.example.com", "/")); err != nil {
		t.Fatalf("allowed host: %v", err)
	}
	if _, err := get(c, hostURL(t, srv, "other.example.com", "/")); !errors.Is(err, ErrBlockedHost) {
		t.Fatalf("want ErrBlockedHost, got %v", err)
	}
}
