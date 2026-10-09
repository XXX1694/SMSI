// Package bluesky is the Bluesky (AT Protocol) adapter, connected with an app password.
//
// Flow: the user pastes a handle and an app password. Verify calls
// com.atproto.server.createSession and stores the DID and handle as metadata; the
// vault stores only the app password. Sessions (accessJwt/refreshJwt) live in
// process memory, keyed by account, and are never persisted or logged.
//
// Limits (checked against docs.bsky.app and the atproto lexicons on 2026-10-09):
//   - text: 300 graphemes and 3000 UTF-8 bytes [V]. The core counts runes, which is
//     never lower than graphemes, so the core check is stricter than Bluesky's.
//   - up to 4 images of at most 2,000,000 bytes each [V]; alt text is not carried by
//     the core media model yet, so alt is sent empty.
//   - createSession: 30 per 5 minutes and 300 per day per account [V]; content
//     writes: 5000 points per hour, create costs 3, delete 1 [V].
//   - App passwords are discouraged for new apps; OAuth is the long-term path [V].
package bluesky

import (
	"net/http"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/safehttp"
)

// Name is the provider identifier.
const Name = "bluesky"

// Limits of the network.
const (
	MaxGraphemes  = 300
	maxTextBytes  = 3000
	MaxImages     = 4
	MaxImageBytes = 2_000_000
	defaultPDS    = "https://bsky.social"
	postType      = "app.bsky.feed.post"
)

// Config configures the adapter. Zero values are the production setting.
type Config struct {
	// PDSURL is the default service (tests point it at a fake).
	PDSURL string
	// HTTPClient replaces every client (tests only). Production uses safehttp.
	HTTPClient *http.Client
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Adapter implements provider.Provider, TokenConnector, Publisher and Lookuper.
type Adapter struct {
	cfg      Config
	pds      string
	fixed    *http.Client // bsky.social only
	custom   *http.Client // user-supplied PDS: SSRF guard on any address
	sessions *sessionCache
}

// New creates the adapter with defaults applied.
func New(cfg Config) *Adapter {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	pds := strings.TrimRight(cfg.PDSURL, "/")
	if pds == "" {
		pds = defaultPDS
	}
	a := &Adapter{cfg: cfg, pds: pds, sessions: newSessionCache()}
	if cfg.HTTPClient != nil {
		a.fixed, a.custom = cfg.HTTPClient, cfg.HTTPClient
		return a
	}
	a.fixed = safehttp.NewSafeClient(safehttp.SafeClientConfig{Timeout: 60 * time.Second, AllowedHosts: []string{"bsky.social"}})
	a.custom = safehttp.NewSafeClient(safehttp.SafeClientConfig{Timeout: 60 * time.Second})
	return a
}

func (a *Adapter) Name() string        { return Name }
func (a *Adapter) DisplayName() string { return "Bluesky" }
func (a *Adapter) Supported() bool     { return true }
func (a *Adapter) Configured() bool    { return true }

// Capabilities of the Bluesky adapter.
func (a *Adapter) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		CanPublishText:  true,
		CanPublishImage: true,
		CanDelete:       true,
		MaxTextLength:   MaxGraphemes,
		MaxMediaCount:   MaxImages,
		MaxImageBytes:   MaxImageBytes,
		ConnectMethod:   provider.ConnectToken,
		ConnectFields: []provider.ConnectField{
			{Name: "handle", Label: "Handle", Help: "Your Bluesky handle, for example name.bsky.social.", Placeholder: "name.bsky.social", Kind: provider.FieldText, Required: true},
			{Name: "app_password", Label: "App password", Help: "Create one in Settings > Privacy and security > App passwords. Never use your main password.", Placeholder: "xxxx-xxxx-xxxx-xxxx", Kind: provider.FieldSecret, Secret: true, Required: true},
			{Name: "pds", Label: "Server (optional)", Help: "Only if you host your own PDS. Leave empty for bsky.social.", Placeholder: "https://bsky.social", Kind: provider.FieldURL},
		},
		Notes: "Text up to 300 graphemes (SocialOS checks 300 characters, which is stricter for emoji), up to 4 images of 2 MB each without alt text, " +
			"links and hashtags become clickable, delete supported. Mentions are not linked. Uses an app password; Bluesky prefers OAuth for new apps, " +
			"so OAuth connect is planned. No native scheduling; SocialOS schedules.",
	}
}
