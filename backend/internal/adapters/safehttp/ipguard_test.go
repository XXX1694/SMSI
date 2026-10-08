package safehttp

import (
	"net/netip"
	"testing"
)

func TestIsBlockedAddr(t *testing.T) {
	for _, tc := range []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true}, {"127.255.255.254", true}, {"::1", true},
		{"10.0.0.1", true}, {"172.16.0.1", true}, {"172.31.255.255", true}, {"192.168.1.1", true}, {"fd00::1", true}, {"fc00::1", true},
		{"169.254.169.254", true}, {"169.254.0.1", true}, {"fe80::1", true}, {"fe80::1%eth0", true},
		{"100.64.0.1", true}, {"100.127.255.255", true},
		{"224.0.0.1", true}, {"239.255.255.250", true}, {"ff02::1", true}, {"ff0e::1", true},
		{"0.0.0.0", true}, {"::", true}, {"0.1.2.3", true},
		{"255.255.255.255", true}, {"240.0.0.1", true}, {"198.18.0.1", true},
		{"::ffff:127.0.0.1", true}, {"::ffff:10.0.0.1", true}, {"::ffff:169.254.169.254", true}, {"::ffff:100.64.0.1", true},
		{"64:ff9b::7f00:1", true}, {"64:ff9b::a9fe:a9fe", true}, {"2002:7f00:1::1", true}, {"2002:a00:1::1", true},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", true},
		// Public addresses stay reachable.
		{"8.8.8.8", false}, {"1.1.1.1", false}, {"172.32.0.1", false}, {"100.63.255.255", false}, {"100.128.0.1", false},
		{"2606:4700:4700::1111", false}, {"::ffff:8.8.8.8", false}, {"64:ff9b::808:808", false}, {"2002:808:808::1", false},
	} {
		if got := IsBlockedAddr(netip.MustParseAddr(tc.ip)); got != tc.blocked {
			t.Errorf("IsBlockedAddr(%s) = %v, want %v", tc.ip, got, tc.blocked)
		}
	}
	if !IsBlockedAddr(netip.Addr{}) {
		t.Error("an invalid address must be blocked")
	}
}
