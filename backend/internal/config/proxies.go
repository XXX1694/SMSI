package config

import (
	"fmt"
	"net/netip"
	"strings"
)

// DefaultTrustedProxies is what TRUST_PROXY=true trusts when TRUSTED_PROXIES is empty: loopback and the private
// ranges, i.e. a reverse proxy on the same host or on a Docker/VPC network.
func DefaultTrustedProxies() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("fc00::/7"),
	}
}

// ParseTrustedProxies parses a comma-separated list of CIDRs (a bare address means a single host). An empty list is
// valid and returns nil. The first malformed entry is an error: a typo must not silently shrink or widen the set.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := netip.ParsePrefix(part)
		if err != nil {
			a, aerr := netip.ParseAddr(part)
			if aerr != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q is not a CIDR (e.g. 10.0.0.0/8) or an IP address", part)
			}
			p = netip.PrefixFrom(a.WithZone(""), a.BitLen())
		}
		if p.Bits() == 0 {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %q would trust every address; list your proxies, or set TRUST_PROXY=false", part)
		}
		// Peers are compared unmapped (::ffff:10.0.0.1 is 10.0.0.1), so the prefixes must be too.
		if p.Addr().Is4In6() {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %q is an IPv4-mapped IPv6 prefix; write the IPv4 form", part)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// resolveTrustedProxies applies the TRUST_PROXY switch: with it off nobody is trusted (X-Forwarded-For is ignored);
// with it on, an empty TRUSTED_PROXIES means the loopback + private defaults. The list is parsed even when the switch
// is off so that a malformed value fails at startup instead of on the day someone enables the proxy.
//
// The warnings are for values that are valid but almost certainly not what the operator meant: a list that is
// ignored, or one that holds no network at all (for example ",").
func resolveTrustedProxies(trustProxy bool, raw string) (nets []netip.Prefix, warnings []string, err error) {
	parsed, err := ParseTrustedProxies(raw)
	if err != nil {
		return nil, nil, err
	}
	if raw != "" && !trustProxy {
		warnings = append(warnings, "TRUSTED_PROXIES is set but ignored because TRUST_PROXY is not true: X-Forwarded-For is never read")
	}
	if raw != "" && len(parsed) == 0 {
		warnings = append(warnings, fmt.Sprintf("TRUSTED_PROXIES %q lists no network; treating it as unset", raw))
	}
	switch {
	case !trustProxy:
		return nil, warnings, nil
	case len(parsed) == 0:
		return DefaultTrustedProxies(), warnings, nil
	}
	return parsed, warnings, nil
}
