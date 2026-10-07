package accounts

import (
	"strings"
	"testing"
)

func TestSafeRedirectPath(t *testing.T) {
	for in, want := range map[string]string{
		"":                       DefaultRedirect,
		"/":                      "/",
		"/accounts":              "/accounts",
		"/calendar?week=2":       "/calendar?week=2",
		"/posts/123#top":         "/posts/123#top",
		"/a/b/c":                 "/a/b/c",
		"accounts":               DefaultRedirect, // not absolute
		"https://evil.example/":  DefaultRedirect,
		"http://evil.example":    DefaultRedirect,
		"//evil.example":         DefaultRedirect, // protocol-relative
		"///evil.example":        DefaultRedirect,
		"/\\evil.example":        DefaultRedirect, // browsers treat \ as /
		"\\\\evil.example":       DefaultRedirect,
		"javascript:alert(1)":    DefaultRedirect,
		"/ok\r\nSet-Cookie: a=b": DefaultRedirect, // header injection
		"/ok\nLocation: x":       DefaultRedirect,
		"/tab\there":             DefaultRedirect,
		"/next?u=https://x.test": DefaultRedirect, // embedded scheme, rejected conservatively
		"data:text/html,x":       DefaultRedirect,
	} {
		if got := SafeRedirectPath(in); got != want {
			t.Errorf("SafeRedirectPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SafeRedirectPath("/" + strings.Repeat("a", 511)); got == DefaultRedirect {
		t.Error("512 characters are allowed")
	}
	if got := SafeRedirectPath("/" + strings.Repeat("a", 512)); got != DefaultRedirect {
		t.Error("over 512 characters must be rejected")
	}
}

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
