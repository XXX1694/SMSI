// Package redirect holds the one allow-list for where the API may send a browser after a flow it started
// (connecting a network, signing in with a provider).
package redirect

import "strings"

// MaxLen bounds a stored redirect path.
const MaxLen = 512

// SafePath returns p when it is a same-site relative path, and fallback otherwise. A redirect target that came from a
// query string must never be able to leave the web app: absolute and protocol-relative URLs, backslashes (browsers
// treat them as slashes), control characters that allow header injection, and embedded schemes are all rejected.
func SafePath(p, fallback string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") ||
		strings.ContainsAny(p, "\\\r\n\t") || strings.Contains(p, "://") || len(p) > MaxLen {
		return fallback
	}
	return p
}
