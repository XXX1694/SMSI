package redirect

import (
	"strings"
	"testing"
)

func TestSafePath(t *testing.T) {
	const fb = "/fallback"
	for in, want := range map[string]string{
		"":                       fb,
		"/dashboard":             "/dashboard",
		"/a/b?c=d#e":             "/a/b?c=d#e",
		"accounts":               fb, // not absolute
		"https://evil.example/":  fb,
		"http://evil.example":    fb,
		"//evil.example":         fb, // protocol-relative
		"///evil.example":        fb,
		"/\\evil.example":        fb, // browsers treat \ as /
		"\\\\evil.example":       fb,
		"javascript:alert(1)":    fb,
		"/ok\r\nSet-Cookie: a=b": fb, // header injection
		"/ok\nLocation: x":       fb,
		"/tab\there":             fb,
		"/next?u=https://x.test": fb, // embedded scheme, rejected conservatively
		"data:text/html,x":       fb,
	} {
		if got := SafePath(in, fb); got != want {
			t.Errorf("SafePath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SafePath("/"+strings.Repeat("a", MaxLen-1), fb); got == fb {
		t.Error("512 characters are allowed")
	}
	if got := SafePath("/"+strings.Repeat("a", MaxLen), fb); got != fb {
		t.Error("over 512 characters must be rejected")
	}
}
