// Package safehttp is the SSRF-safe HTTP client for adapters that call a host the
// user supplied. It lives beside the provider port, not in it, because it does I/O.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// Limits of the SSRF-safe client (see D-010).
const (
	// MaxResponseBytes caps every response body an adapter reads.
	MaxResponseBytes = 1 << 20
	maxRedirects     = 3
)

// Sentinel errors of the safe client. They hold no URL or credential.
var (
	ErrBlockedAddress   = errors.New("destination address is not allowed")
	ErrInsecureScheme   = errors.New("only https is allowed")
	ErrBlockedHost      = errors.New("host is not allowed")
	ErrRedirectNotAllow = errors.New("redirect to another host is not allowed")
	ErrResponseTooLarge = errors.New("response is too large")
)

// SafeClientConfig tunes NewSafeClient. The zero value is the production setting.
type SafeClientConfig struct {
	// Timeout bounds one whole request including the body (default 15 s).
	Timeout time.Duration
	// AllowedHosts, when set, restricts requests and redirects to these exact
	// host names. Adapters with a fixed API host (discord.com) set it; adapters
	// whose host the user supplies leave it empty and rely on the address guard.
	AllowedHosts []string
	// Lookup resolves a host name. Nil uses the system resolver. Tests inject a
	// custom resolver here to simulate DNS rebinding.
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)

	blocked func(netip.Addr) bool // tests only: allow loopback test servers
}

// NewSafeClient returns an http.Client for fetching URLs a user supplied (a
// Mastodon instance, a webhook). It
//   - speaks https only and ignores proxy environment variables,
//   - resolves the host itself, refuses to connect when any answer is a
//     loopback, private, link-local, CGNAT, multicast, unspecified or otherwise
//     reserved address (IPv4, IPv6 and IPv4-mapped forms) and dials the vetted
//     IP directly, so a second DNS answer cannot change the destination,
//   - re-checks the connected address in the dialer Control hook,
//   - follows redirects only to the same host, at most three times,
//   - fails a response body that is larger than MaxResponseBytes.
func NewSafeClient(cfg SafeClientConfig) *http.Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.blocked == nil {
		cfg.blocked = IsBlockedAddr
	}
	if cfg.Lookup == nil {
		cfg.Lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil || cfg.blocked(ap.Addr()) {
			return ErrBlockedAddress
		}
		return nil
	}}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           cfg.dialContext(dialer),
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Timeout:       cfg.Timeout,
		Transport:     &guardTransport{next: tr, cfg: cfg},
		CheckRedirect: cfg.checkRedirect,
	}
}

func (cfg SafeClientConfig) dialContext(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := cfg.resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		var last error
		for _, ip := range ips {
			c, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return c, nil
			}
			last = err
		}
		return nil, last
	}
}

// resolve returns the addresses of host, or ErrBlockedAddress when any of them
// is not allowed (a mixed answer is a rebinding signal, not a choice to make).
func (cfg SafeClientConfig) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		if cfg.blocked(ip) {
			return nil, ErrBlockedAddress
		}
		return []netip.Addr{ip}, nil
	}
	ips, err := cfg.Lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	for _, ip := range ips {
		if cfg.blocked(ip) {
			return nil, ErrBlockedAddress
		}
	}
	return ips, nil
}

func (cfg SafeClientConfig) hostAllowed(host string) bool {
	if len(cfg.AllowedHosts) == 0 {
		return true
	}
	for _, h := range cfg.AllowedHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}

func (cfg SafeClientConfig) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return fmt.Errorf("stopped after %d redirects: %w", maxRedirects, ErrRedirectNotAllow)
	}
	if req.URL.Scheme != "https" {
		return ErrInsecureScheme
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return ErrRedirectNotAllow
	}
	return nil
}

// guardTransport enforces the scheme and host rules before any dial and caps response bodies.
type guardTransport struct {
	next http.RoundTripper
	cfg  SafeClientConfig
}

func (g *guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, ErrInsecureScheme
	}
	if !g.cfg.hostAllowed(req.URL.Hostname()) {
		return nil, ErrBlockedHost
	}
	resp, err := g.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.ContentLength > MaxResponseBytes {
		_ = resp.Body.Close()
		return nil, ErrResponseTooLarge
	}
	resp.Body = &cappedBody{rc: resp.Body, left: MaxResponseBytes}
	return resp, nil
}

// cappedBody returns ErrResponseTooLarge instead of silently truncating, so a
// cut-off JSON document is never mistaken for a complete answer.
type cappedBody struct {
	rc   io.ReadCloser
	left int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	if b.left < 0 {
		return 0, ErrResponseTooLarge
	}
	if int64(len(p)) > b.left+1 {
		p = p[:b.left+1]
	}
	n, err := b.rc.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		return n - 1, ErrResponseTooLarge // the last byte is the one past the cap
	}
	return n, err
}

func (b *cappedBody) Close() error { return b.rc.Close() }
