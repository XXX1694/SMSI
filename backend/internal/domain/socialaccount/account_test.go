package socialaccount

import (
	"testing"
	"time"
)

func TestNeedsRefresh(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	for name, tc := range map[string]struct {
		c    Credentials
		want bool
	}{
		"no expiry means a static token": {Credentials{}, false},
		"far in the future":              {Credentials{ExpiresAt: at(2 * time.Hour)}, false},
		"inside the window":              {Credentials{ExpiresAt: at(4 * time.Minute)}, true},
		"exactly at the window edge":     {Credentials{ExpiresAt: at(5 * time.Minute)}, false},
		"already expired":                {Credentials{ExpiresAt: at(-time.Second)}, true},
	} {
		if got := tc.c.NeedsRefresh(now, 5*time.Minute); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestCanRefresh(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	for name, tc := range map[string]struct {
		c    Credentials
		want bool
	}{
		"no refresh token":             {Credentials{}, false},
		"token without expiry":         {Credentials{RefreshToken: "r"}, true},
		"token not yet expired":        {Credentials{RefreshToken: "r", RefreshExpiresAt: &future}, true},
		"refresh token expired":        {Credentials{RefreshToken: "r", RefreshExpiresAt: &past}, false},
		"expiry set but token missing": {Credentials{RefreshExpiresAt: &future}, false},
	} {
		if got := tc.c.CanRefresh(now); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestPublishable(t *testing.T) {
	for st, want := range map[Status]bool{StatusActive: true, StatusExpired: false, StatusRevoked: false, StatusError: false, Status(""): false} {
		if got := (&Account{Status: st}).Publishable(); got != want {
			t.Errorf("%q: %v", st, got)
		}
	}
}
