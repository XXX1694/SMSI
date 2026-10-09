package safehttp

import "net/netip"

// blockedPrefixes are destinations a user-supplied URL must never reach, beyond
// what netip's own predicates cover. Sources: IANA special-purpose registries.
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // carrier-grade NAT
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"240.0.0.0/4",     // reserved, includes the broadcast address
	"2001::/32",       // Teredo
	"2001:db8::/32",   // documentation
	"100::/64",        // discard-only
	"fec0::/10",       // deprecated site-local (RFC 3879), still routable on some internal networks
	"::/96",           // deprecated IPv4-compatible (::7f00:1 would reach 127.0.0.1 on some stacks); also ::, ::1
)

// Prefixes that embed an IPv4 address: the embedded address is judged instead.
var (
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")
	// Local-use NAT64 (RFC 8215): operator translators that may use any RFC 6052
	// layout (/48../96), so the embedded IPv4 cannot be read reliably. The whole
	// range is blocked.
	nat64LocalPrefix = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFour        = netip.MustParsePrefix("2002::/16")
)

func mustPrefixes(in ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(in))
	for i, s := range in {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// IsBlockedAddr reports whether ip is a loopback, private, link-local (which
// includes the 169.254.169.254 metadata address), CGNAT, multicast,
// unspecified or otherwise reserved address. IPv4-mapped, NAT64 and 6to4 forms
// are judged by the IPv4 address they carry. An invalid address is blocked.
func IsBlockedAddr(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	ip = ip.WithZone("").Unmap()
	switch {
	case ip.IsUnspecified(), ip.IsLoopback(), ip.IsPrivate(), ip.IsMulticast(),
		ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(), ip.IsInterfaceLocalMulticast():
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	if ip.Is6() {
		b := ip.As16()
		switch {
		case nat64Prefix.Contains(ip):
			return IsBlockedAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		case nat64LocalPrefix.Contains(ip):
			return true
		case sixToFour.Contains(ip):
			return IsBlockedAddr(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	return false
}
