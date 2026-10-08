package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedProxies is the set of reverse-proxy networks whose X-Forwarded-For hops are believed (config
// TRUSTED_PROXIES). The zero value trusts nobody: the TCP peer is then always the client.
type TrustedProxies []netip.Prefix

func (t TrustedProxies) contains(a netip.Addr) bool {
	for _, p := range t {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ClientIP returns the address of the caller.
//
// A client controls every X-Forwarded-For entry it sends, and each proxy on the way appends the peer it saw to the
// right. So the header is only evidence from the right: the entries a trusted proxy appended are trustworthy, the ones
// further left are not. ClientIP therefore starts at the TCP peer (RemoteAddr); if that is not a trusted proxy it is
// the client and the headers are ignored. Otherwise it walks X-Forwarded-For from right to left, skips hops that are
// themselves trusted proxies and returns the first other valid address. When every hop is trusted, or the header is
// missing, the peer is returned. Malformed entries are skipped.
func ClientIP(r *http.Request, trusted TrustedProxies) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host // not an IP (unix socket, test double): nothing to compare, nothing to trust
	}
	peer = normalize(peer)
	if !trusted.contains(peer) {
		return peer.String()
	}
	lines := r.Header.Values("X-Forwarded-For") // several header lines are one list, the last line is the nearest proxy's
	for i := len(lines) - 1; i >= 0; i-- {
		rest := lines[i]
		for rest != "" {
			hop := rest
			if j := strings.LastIndexByte(rest, ','); j >= 0 {
				hop, rest = rest[j+1:], rest[:j]
			} else {
				rest = ""
			}
			if a, ok := parseHop(hop); ok && !trusted.contains(a) {
				return a.String()
			}
		}
	}
	return peer.String()
}

// parseHop reads one X-Forwarded-For entry: an IP, optionally with a port, IPv6 optionally in brackets.
func parseHop(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(s); err == nil {
		return normalize(a), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil { // 192.0.2.1:4711, [2001:db8::1]:4711
		return normalize(ap.Addr()), true
	}
	if n := len(s); n > 2 && s[0] == '[' && s[n-1] == ']' { // [2001:db8::1]
		if a, err := netip.ParseAddr(s[1 : n-1]); err == nil {
			return normalize(a), true
		}
	}
	return netip.Addr{}, false
}

// normalize gives every spelling of an address one form, so that the same client always lands in the same
// rate-limit bucket and ::ffff:10.0.0.1 matches 10.0.0.0/8.
func normalize(a netip.Addr) netip.Addr { return a.Unmap().WithZone("") }
