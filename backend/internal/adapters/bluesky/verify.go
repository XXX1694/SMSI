package bluesky

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/socialos/backend/internal/adapters/provider"
)

var handlePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// Verify logs in with the app password and returns the DID and handle as
// non-secret metadata. The secret is the app password; no JWT is stored.
func (a *Adapter) Verify(ctx context.Context, fields map[string]string) (provider.Profile, string, error) {
	ident, err := normalizeIdentifier(fields["handle"])
	if err != nil {
		return provider.Profile{}, "", err
	}
	password := strings.TrimSpace(fields["app_password"])
	if password == "" {
		return provider.Profile{}, "", permanent("MISSING_PASSWORD", "Bluesky needs an app password")
	}
	base, custom, err := a.baseFor(fields["pds"])
	if err != nil {
		return provider.Profile{}, "", err
	}
	// A throwaway cache key: Verify must not seed the cache of an account that does not exist yet.
	s, err := a.create(ctx, target{base: base, identifier: ident, password: password})
	if err != nil {
		return provider.Profile{}, "", err
	}
	handle := s.handle
	if handle == "" {
		handle = ident
	}
	meta := map[string]any{"handle": handle, "did": s.did}
	if custom {
		meta["pds"] = base
	}
	return provider.Profile{ID: s.did, Username: handle, DisplayName: handle, Metadata: meta}, password, nil
}

// normalizeIdentifier accepts a handle (with or without @) or a DID.
func normalizeIdentifier(raw string) (string, error) {
	s := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
	switch {
	case strings.HasPrefix(s, "did:") && len(s) <= 2048 && !strings.ContainsAny(s, " \t\r\n"):
		return s, nil
	case len(s) <= 253 && handlePattern.MatchString(s):
		return s, nil
	}
	return "", permanent("BAD_HANDLE", "Enter your full Bluesky handle, for example name.bsky.social")
}

// baseFor returns the service origin. A custom PDS must be a plain https origin;
// the SSRF-safe client then vets the address it resolves to.
func (a *Adapter) baseFor(raw string) (base string, custom bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return a.pds, false, nil
	}
	u, perr := url.Parse(raw)
	if perr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false, permanent("BAD_PDS", "The server must be an https address without a path, for example https://pds.example.com")
	}
	base = "https://" + strings.ToLower(u.Host)
	return base, base != a.pds, nil
}
