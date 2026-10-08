package config

import (
	"net/netip"
	"strconv"
	"strings"
	"testing"
)

func prefixes(t *testing.T, in []netip.Prefix) string {
	t.Helper()
	s := make([]string, len(in))
	for i, p := range in {
		s[i] = p.String()
	}
	return strings.Join(s, ",")
}

func TestTrustedProxiesResolution(t *testing.T) {
	defaults := "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"
	for name, tc := range map[string]struct {
		trust bool
		raw   string // TRUSTED_PROXIES; "-" leaves it unset
		want  string
	}{
		"proxy off: nobody is trusted":        {false, "-", ""},
		"proxy off ignores a valid list":      {false, "10.0.0.0/8", ""},
		"proxy on, unset: loopback + private": {true, "-", defaults},
		"proxy on, blank: loopback + private": {true, "  ,  ", defaults},
		"explicit list replaces the defaults": {true, "203.0.113.0/24", "203.0.113.0/24"},
		"spaces, bare IPs and IPv6":           {true, " 192.0.2.7 , 2001:db8::/32 ,fe80::1", "192.0.2.7/32,2001:db8::/32,fe80::1/128"},
		"host bits are masked":                {true, "10.1.2.3/8", "10.0.0.0/8"},
		"zone on a bare address is dropped":   {true, "fe80::1%eth0", "fe80::1/128"},
		"mixed IPv4 and IPv6":                 {true, "10.0.0.0/8,::1", "10.0.0.0/8,::1/128"},
	} {
		t.Run(name, func(t *testing.T) {
			validEnv(t)
			t.Setenv("TRUST_PROXY", strconv.FormatBool(tc.trust))
			if tc.raw != "-" {
				t.Setenv("TRUSTED_PROXIES", tc.raw)
			}
			c, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := prefixes(t, c.TrustedProxies); got != tc.want {
				t.Fatalf("TrustedProxies = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrustedProxiesFailFast(t *testing.T) {
	for name, raw := range map[string]string{
		"not an address":          "proxy.internal",
		"bad prefix length":       "10.0.0.0/33",
		"bad IPv6 prefix length":  "fc00::/129",
		"one bad entry in a list": "10.0.0.0/8,300.1.1.1/8",
		"trailing slash":          "10.0.0.0/",
		"range instead of a CIDR": "10.0.0.1-10.0.0.9",
		"IPv4-mapped IPv6 prefix": "::ffff:10.0.0.0/104",
		"semicolon, not a comma":  "10.0.0.0/8;192.168.0.0/16",
		"url":                     "http://10.0.0.1",
		"IPv4 wildcard":           "0.0.0.0/0",
		"IPv6 wildcard":           "10.0.0.0/8,::/0",
	} {
		for _, trust := range []string{"true", "false"} { // a typo must not wait until someone flips TRUST_PROXY
			t.Run(name+"/TRUST_PROXY="+trust, func(t *testing.T) {
				validEnv(t)
				t.Setenv("TRUST_PROXY", trust)
				t.Setenv("TRUSTED_PROXIES", raw)
				c, err := Load()
				if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
					t.Fatalf("want a TRUSTED_PROXIES error, got %v", err)
				}
				if c == nil || len(c.TrustedProxies) != 0 {
					t.Fatalf("a rejected list must not leave a trust set behind: %+v", c)
				}
			})
		}
	}
}

func TestTrustedProxiesErrorIsReportedWithOtherProblems(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("TRUSTED_PROXIES", "nope")
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL", "ENCRYPTION_KEY", "TRUSTED_PROXIES"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s: %v", want, err)
		}
	}
}
