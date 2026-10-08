package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/socialos/backend/internal/config"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/transport/middleware"
)

// privateProxies is what TRUST_PROXY=true gives without TRUSTED_PROXIES: loopback and the private ranges.
var privateProxies = middleware.TrustedProxies(config.DefaultTrustedProxies())

func cidrs(t *testing.T, in ...string) middleware.TrustedProxies {
	t.Helper()
	var out middleware.TrustedProxies
	for _, s := range in {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

func requestFrom(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestClientIP(t *testing.T) {
	const (
		client = "203.0.113.9" // the real caller in most cases
		forged = "6.6.6.6"
	)
	type tc struct {
		trusted middleware.TrustedProxies // nil means privateProxies
		remote  string
		xff     []string // one element per header line
		want    string
	}
	cases := map[string]tc{
		// The peer is not a trusted proxy: it is the client and the headers are noise.
		"untrusted peer, no header":                          {remote: "192.0.2.5:4000", want: "192.0.2.5"},
		"untrusted peer forges a client":                     {remote: "192.0.2.5:4000", xff: []string{forged}, want: "192.0.2.5"},
		"untrusted peer forges a chain":                      {remote: "192.0.2.5:4000", xff: []string{forged + ", 10.0.0.1"}, want: "192.0.2.5"},
		"untrusted peer forges a private address":            {remote: "192.0.2.5:4000", xff: []string{"10.0.0.1"}, want: "192.0.2.5"},
		"untrusted peer, several header lines":               {remote: "192.0.2.5:4000", xff: []string{forged, client}, want: "192.0.2.5"},
		"untrusted IPv6 peer":                                {remote: "[2001:db8::1]:80", xff: []string{forged}, want: "2001:db8::1"},
		"untrusted peer is normalised":                       {remote: "[::ffff:192.0.2.5]:80", xff: []string{forged}, want: "192.0.2.5"},
		"peer without a port":                                {remote: "192.0.2.9", want: "192.0.2.9"},
		"bare IPv6 peer without a port":                      {remote: "2001:db8::9", want: "2001:db8::9"},
		"peer that is not an IP stays as it is":              {remote: "@", xff: []string{forged}, want: "@"},
		"empty peer stays empty":                             {remote: "", xff: []string{forged}, want: ""},
		"empty trust set trusts nobody, even loopback":       {trusted: middleware.TrustedProxies{}, remote: "127.0.0.1:1", xff: []string{forged}, want: "127.0.0.1"},
		"custom set: private peer outside it is not trusted": {trusted: cidrs(t, "203.0.113.0/24"), remote: "10.0.0.1:1", xff: []string{forged}, want: "10.0.0.1"},

		// A trusted proxy reports who it saw.
		"trusted peer, no header":                     {remote: "10.0.0.1:4000", want: "10.0.0.1"},
		"trusted peer, empty header":                  {remote: "10.0.0.1:4000", xff: []string{""}, want: "10.0.0.1"},
		"one hop":                                     {remote: "10.0.0.1:4000", xff: []string{client}, want: client},
		"loopback proxy":                              {remote: "127.0.0.1:4000", xff: []string{client}, want: client},
		"IPv6 loopback proxy":                         {remote: "[::1]:4000", xff: []string{client}, want: client},
		"IPv6 ULA proxy (docker/VPC)":                 {remote: "[fd00:1::5]:4000", xff: []string{client}, want: client},
		"IPv4-mapped trusted peer":                    {remote: "[::ffff:10.0.0.1]:4000", xff: []string{client}, want: client},
		"docker bridge proxy":                         {remote: "172.18.0.5:4000", xff: []string{client}, want: client},
		"192.168 proxy":                               {remote: "192.168.1.2:4000", xff: []string{client}, want: client},
		"172.32 is public, not a proxy":               {remote: "172.32.0.1:4000", xff: []string{forged}, want: "172.32.0.1"},
		"11.0.0.1 is public, not a proxy":             {remote: "11.0.0.1:4000", xff: []string{forged}, want: "11.0.0.1"},
		"fe80:: link-local is not trusted by default": {remote: "[fe80::1]:4000", xff: []string{forged}, want: "fe80::1"},

		// The leftmost entry is attacker-controlled; only what the trusted proxies appended counts.
		"spoofed leftmost, proxy appended the client": {remote: "10.0.0.1:1", xff: []string{forged + ", " + client}, want: client},
		"spoofed leftmost, two proxies":               {remote: "10.0.0.1:1", xff: []string{forged + ", " + client + ", 10.0.0.2"}, want: client},
		"spoofed leftmost, three proxies":             {remote: "10.0.0.1:1", xff: []string{forged + ", " + client + ", 10.0.0.2, 172.18.0.3"}, want: client},
		"append chain (CDN then proxy)":               {remote: "10.0.0.1:1", xff: []string{client + ", 198.51.100.77, 10.0.0.2"}, want: "198.51.100.77"},
		"forged private entry on the left":            {remote: "10.0.0.1:1", xff: []string{"10.0.0.99, " + client}, want: client},
		"forged loopback entry on the left":           {remote: "10.0.0.1:1", xff: []string{"127.0.0.1, " + client}, want: client},
		"forged entry cannot win by being trusted":    {remote: "10.0.0.1:1", xff: []string{client + ", 10.9.9.9, 10.8.8.8"}, want: client},

		// Every hop is a proxy: nothing to learn, so the peer it is.
		"all hops trusted":                  {remote: "10.0.0.1:1", xff: []string{"10.0.0.5, 192.168.1.1"}, want: "10.0.0.1"},
		"all hops trusted, loopback client": {remote: "10.0.0.1:1", xff: []string{"127.0.0.1"}, want: "10.0.0.1"},
		"all hops trusted IPv6":             {remote: "[fd00::1]:1", xff: []string{"fd00::2, ::1"}, want: "fd00::1"},

		// Several X-Forwarded-For lines are one list; the last line is the nearest proxy's.
		"two lines, forged first":     {remote: "10.0.0.1:1", xff: []string{forged, client + ", 10.0.0.2"}, want: client},
		"two lines, client first":     {remote: "10.0.0.1:1", xff: []string{client, "10.0.0.2"}, want: client},
		"two lines, last one decides": {remote: "10.0.0.1:1", xff: []string{"10.0.0.2", "198.51.100.4"}, want: "198.51.100.4"},
		"three lines, trusted tail":   {remote: "10.0.0.1:1", xff: []string{forged, client, "10.0.0.2", "172.16.0.9"}, want: client},
		"blank last line is skipped":  {remote: "10.0.0.1:1", xff: []string{forged + ", " + client, ""}, want: client},

		// IPv6, brackets, ports, zones.
		"IPv6 client":                         {remote: "10.0.0.1:1", xff: []string{"2001:db8::1"}, want: "2001:db8::1"},
		"IPv6 client is normalised":           {remote: "10.0.0.1:1", xff: []string{"2001:DB8:0:0:0:0:0:1"}, want: "2001:db8::1"},
		"IPv6 client in brackets":             {remote: "10.0.0.1:1", xff: []string{"[2001:db8::7]"}, want: "2001:db8::7"},
		"IPv6 client in brackets with port":   {remote: "10.0.0.1:1", xff: []string{"[2001:db8::7]:5555"}, want: "2001:db8::7"},
		"IPv4 client with port":               {remote: "10.0.0.1:1", xff: []string{client + ":4711"}, want: client},
		"IPv4-mapped client":                  {remote: "10.0.0.1:1", xff: []string{"::ffff:" + client}, want: client},
		"IPv4-mapped proxy hop is skipped":    {remote: "10.0.0.1:1", xff: []string{client + ", ::ffff:10.0.0.7"}, want: client},
		"bracketed IPv6 proxy hop is skipped": {remote: "10.0.0.1:1", xff: []string{client + ", [fd00::2]:99"}, want: client},
		"IPv6 chain, forged left":             {remote: "[fd00::1]:1", xff: []string{forged + ", 2001:db8::5, fd00::2"}, want: "2001:db8::5"},
		"zone is dropped":                     {remote: "10.0.0.1:1", xff: []string{"fe80::1%eth0"}, want: "fe80::1"},
		"spaces and tabs around entries":      {remote: "10.0.0.1:1", xff: []string{"  " + forged + " ,\t" + client + "\t, 10.0.0.2 "}, want: client},

		// Garbage never panics, never wins and never hides the real hop.
		"garbage on the left":             {remote: "10.0.0.1:1", xff: []string{"not-an-ip, " + client}, want: client},
		"garbage on the right is skipped": {remote: "10.0.0.1:1", xff: []string{client + ", not-an-ip"}, want: client},
		"garbage between hops":            {remote: "10.0.0.1:1", xff: []string{client + ", ???, 10.0.0.2"}, want: client},
		"only garbage":                    {remote: "10.0.0.1:1", xff: []string{"not-an-ip"}, want: "10.0.0.1"},
		"unknown and obfuscated tokens":   {remote: "10.0.0.1:1", xff: []string{"unknown, _hidden, " + client}, want: client},
		"empty entries":                   {remote: "10.0.0.1:1", xff: []string{", ," + client + ",,"}, want: client},
		"only commas":                     {remote: "10.0.0.1:1", xff: []string{",,,"}, want: "10.0.0.1"},
		"truncated IPv4":                  {remote: "10.0.0.1:1", xff: []string{"1.2.3, " + client}, want: client},
		"out-of-range octet":              {remote: "10.0.0.1:1", xff: []string{client + ", 999.1.1.1"}, want: client},
		"CIDR is not an address":          {remote: "10.0.0.1:1", xff: []string{client + ", 1.1.1.0/24"}, want: client},
		"unclosed bracket":                {remote: "10.0.0.1:1", xff: []string{client + ", [2001:db8::1"}, want: client},
		"empty brackets":                  {remote: "10.0.0.1:1", xff: []string{client + ", []"}, want: client},
		"bad port":                        {remote: "10.0.0.1:1", xff: []string{client + ", 1.1.1.1:99999"}, want: client},
		"hostname":                        {remote: "10.0.0.1:1", xff: []string{client + ", evil.example.com"}, want: client},
		"quoted address (RFC 7239 style)": {remote: "10.0.0.1:1", xff: []string{client + `, "1.1.1.1"`}, want: client},
		"semicolon list":                  {remote: "10.0.0.1:1", xff: []string{"for=1.1.1.1;by=2.2.2.2"}, want: "10.0.0.1"},
		"NUL and control bytes":           {remote: "10.0.0.1:1", xff: []string{"\x00\x01, " + client + ", \x7f"}, want: client},
		"very many empty entries":         {remote: "10.0.0.1:1", xff: []string{client + strings.Repeat(",", 200000)}, want: client},

		// A custom set replaces the defaults.
		"custom set: its hop is skipped":      {trusted: cidrs(t, "203.0.113.0/24"), remote: "203.0.113.5:1", xff: []string{"198.51.100.1, 203.0.113.9"}, want: "198.51.100.1"},
		"custom set: private hop is a client": {trusted: cidrs(t, "203.0.113.0/24"), remote: "203.0.113.5:1", xff: []string{"10.1.1.1"}, want: "10.1.1.1"},
		"custom single host":                  {trusted: cidrs(t, "192.0.2.7/32"), remote: "192.0.2.7:1", xff: []string{forged + ", " + client}, want: client},
		"custom single host, neighbour":       {trusted: cidrs(t, "192.0.2.7/32"), remote: "192.0.2.8:1", xff: []string{client}, want: "192.0.2.8"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			trusted := c.trusted
			if trusted == nil {
				trusted = privateProxies
			}
			if got := middleware.ClientIP(requestFrom(c.remote, c.xff...), trusted); got != c.want {
				t.Errorf("ClientIP(remote=%q, xff=%q) = %q, want %q", c.remote, c.xff, got, c.want)
			}
		})
	}
}

// What TRUST_PROXY=false means: the header is never read, whoever the peer is.
func TestClientIPWithoutTrustedProxiesIgnoresTheHeader(t *testing.T) {
	for _, remote := range []string{"10.0.0.1:1", "127.0.0.1:1", "[::1]:1", "192.0.2.1:1"} {
		r := requestFrom(remote, "203.0.113.9", "198.51.100.1, 10.0.0.2")
		host := remote[:strings.LastIndex(remote, ":")]
		host = strings.Trim(host, "[]")
		if got := middleware.ClientIP(r, nil); got != host {
			t.Errorf("remote %s: got %s, want %s", remote, got, host)
		}
	}
}

// The limiter must be keyed by the address ClientIP resolves: a client that rotates the forged part of
// X-Forwarded-For stays in one bucket, and two real clients behind the same proxy get one bucket each.
func TestRateLimitKeysByTheResolvedClientIP(t *testing.T) {
	newHandler := func(trusted middleware.TrustedProxies) http.Handler {
		l := middleware.NewLimiter(0.001, 1) // one request, then 429 for ~17 minutes
		return middleware.RequestID(middleware.RateLimit(l, trusted, nil, "t:")(http.HandlerFunc(ok)))
	}
	send := func(h http.Handler, remote string, xff ...string) int {
		return do(h, "GET", "/", func(r *http.Request) {
			r.RemoteAddr = remote
			for _, v := range xff {
				r.Header.Add("X-Forwarded-For", v)
			}
		}).Code
	}

	t.Run("rotating a forged leftmost hop does not reset the bucket", func(t *testing.T) {
		h := newHandler(privateProxies)
		var codes []int
		for i := 0; i < 6; i++ {
			// The proxy appended the real address after whatever the client sent.
			codes = append(codes, send(h, "10.0.0.1:1", fmt.Sprintf("198.51.100.%d, 203.0.113.9", i+1)))
		}
		if codes[0] != 204 {
			t.Fatalf("first request must pass: %v", codes)
		}
		for i, c := range codes[1:] {
			if c != 429 {
				t.Fatalf("request %d with a fresh forged hop got %d, want 429: %v", i+2, c, codes)
			}
		}
	})

	t.Run("a long forged chain does not either", func(t *testing.T) {
		h := newHandler(privateProxies)
		send(h, "10.0.0.1:1", "1.1.1.1, 2.2.2.2, 203.0.113.9, 10.0.0.2")
		if c := send(h, "10.0.0.1:1", "3.3.3.3, 4.4.4.4, 5.5.5.5, 203.0.113.9, 10.0.0.2"); c != 429 {
			t.Fatalf("got %d, want 429", c)
		}
	})

	t.Run("different real clients behind one proxy have separate buckets", func(t *testing.T) {
		h := newHandler(privateProxies)
		for _, ip := range []string{"203.0.113.9", "203.0.113.10", "2001:db8::1"} {
			if c := send(h, "10.0.0.1:1", ip); c != 204 {
				t.Fatalf("%s: first request got %d", ip, c)
			}
			if c := send(h, "10.0.0.1:1", ip); c != 429 {
				t.Fatalf("%s: second request got %d, want 429", ip, c)
			}
		}
	})

	t.Run("the same client through different proxy hops is one bucket", func(t *testing.T) {
		h := newHandler(privateProxies)
		send(h, "10.0.0.1:1", "203.0.113.9")
		if c := send(h, "172.18.0.7:9", "203.0.113.9, 10.0.0.1"); c != 429 {
			t.Fatalf("got %d, want 429", c)
		}
		if c := send(h, "10.0.0.1:1", "::ffff:203.0.113.9"); c != 429 {
			t.Fatalf("IPv4-mapped spelling got %d, want 429", c)
		}
	})

	t.Run("an untrusted peer is limited by its own address, whatever it claims", func(t *testing.T) {
		h := newHandler(privateProxies)
		var codes []int
		for i := 0; i < 5; i++ {
			codes = append(codes, send(h, "192.0.2.1:1", fmt.Sprintf("198.51.100.%d", i+1), "10.0.0.1"))
		}
		if codes[0] != 204 || codes[1] != 429 || codes[4] != 429 {
			t.Fatalf("forged XFF from an untrusted peer reset the bucket: %v", codes)
		}
	})

	t.Run("with no trusted proxies the header is never used, even from loopback", func(t *testing.T) {
		h := newHandler(nil)
		var codes []int
		for i := 0; i < 4; i++ {
			codes = append(codes, send(h, "127.0.0.1:1", fmt.Sprintf("198.51.100.%d", i+1)))
		}
		if codes[0] != 204 || codes[1] != 429 || codes[3] != 429 {
			t.Fatalf("codes %v", codes)
		}
	})

	t.Run("authenticated callers stay keyed by actor, not by IP", func(t *testing.T) {
		h := newHandler(privateProxies)
		asUser := func(xff string) int {
			return do(h, "GET", "/", func(r *http.Request) {
				r.RemoteAddr = "10.0.0.1:1"
				r.Header.Set("X-Forwarded-For", xff)
				*r = *r.WithContext(actor.With(r.Context(), actor.Actor{Type: actor.TypeUser, ID: "user-a"}))
			}).Code
		}
		if asUser("198.51.100.1") != 204 || asUser("198.51.100.2") != 429 {
			t.Fatal("the same user must share one bucket across addresses")
		}
	})
}
