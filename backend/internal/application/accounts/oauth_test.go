package accounts

import (
	"testing"
)

func TestCallbackURL(t *testing.T) {
	for base, want := range map[string]string{
		"https://api.example.com":  "https://api.example.com/api/v1/social/linkedin/callback",
		"https://api.example.com/": "https://api.example.com/api/v1/social/linkedin/callback",
		"http://localhost:8080///": "http://localhost:8080/api/v1/social/linkedin/callback",
	} {
		s := &Service{redirectBaseURL: base}
		if got := s.CallbackURL("linkedin"); got != want {
			t.Errorf("%q: %q, want %q", base, got, want)
		}
	}
}
