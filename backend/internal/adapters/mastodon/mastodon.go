// Package mastodon is the adapter for Mastodon and API-compatible Fediverse
// servers (the v1/v2 REST API: GoToSocial, Pixelfed and similar).
//
// Flow: the user creates an application token on their own instance
// (Preferences > Development > New application, scopes write:statuses
// write:media read:accounts) and pastes the instance URL and the token.
// Verify calls accounts/verify_credentials and instance (for the limits).
// Publish uploads images to /api/v2/media (polling while the server processes
// them) and then POSTs /api/v1/statuses with an Idempotency-Key.
//
// The instance host is user-supplied, so by default every call goes through
// the SSRF-safe client (D-010): https only, no private or reserved addresses.
// Publishing is public; unlisted is a possible later option.
package mastodon

import (
	"net/http"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/safehttp"
)

// Name is the provider identifier.
const Name = "mastodon"

// Defaults of Mastodon itself; the real values are read from the instance.
const (
	DefaultMaxCharacters = 500
	DefaultMaxMedia      = 4
	// DefaultMaxImageBytes is Mastodon's default image_size_limit (16 MiB).
	DefaultMaxImageBytes = 16 << 20
)

// Config configures the adapter. The zero value is the production setting.
type Config struct {
	// HTTPClient replaces the SSRF-safe client. Tests inject one that talks to
	// an httptest TLS server; production leaves it nil.
	HTTPClient *http.Client
	// Now is the clock used to turn X-RateLimit-Reset into a delay.
	Now func() time.Time
	// MediaPollInterval is the wait between media status checks (default 1 s).
	MediaPollInterval time.Duration
	// MediaProcessingTimeout bounds the wait for one image to be processed (default 60 s).
	MediaProcessingTimeout time.Duration
}

// Adapter implements provider.Provider, TokenConnector and Publisher.
type Adapter struct {
	cfg  Config
	http *http.Client
}

// New creates the adapter with defaults applied.
func New(cfg Config) *Adapter {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.MediaPollInterval <= 0 {
		cfg.MediaPollInterval = time.Second
	}
	if cfg.MediaProcessingTimeout <= 0 {
		cfg.MediaProcessingTimeout = 60 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		// Whole-request budget: large uploads need more than the 15 s default.
		client = safehttp.NewSafeClient(safehttp.SafeClientConfig{Timeout: 60 * time.Second})
	}
	return &Adapter{cfg: cfg, http: client}
}

func (a *Adapter) Name() string        { return Name }
func (a *Adapter) DisplayName() string { return "Mastodon" }
func (a *Adapter) Supported() bool     { return true }
func (a *Adapter) Configured() bool    { return true }

// Capabilities of the adapter. The limits are Mastodon's defaults; Verify stores
// the instance's own (stricter) limits in the account metadata.
func (a *Adapter) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		CanPublishText:  true,
		CanPublishImage: true,
		CanDelete:       true,
		MaxTextLength:   DefaultMaxCharacters,
		MaxMediaCount:   DefaultMaxMedia,
		MaxImageBytes:   DefaultMaxImageBytes,
		ConnectMethod:   provider.ConnectToken,
		// The Idempotency-Key makes a repeated POST /statuses return the first
		// status, so an unknown outcome can be re-sent safely (see publish.go).
		SafeToRetryAfterUnknown: true,
		ConnectFields: []provider.ConnectField{
			{Name: "instance_url", Label: "Instance URL", Kind: provider.FieldURL, Required: true,
				Placeholder: "https://mastodon.social",
				Help:        "The https address of your server. Servers on private networks cannot be connected."},
			{Name: "access_token", Label: "Access token", Kind: provider.FieldSecret, Secret: true, Required: true,
				Help: "On your server: Preferences > Development > New application. Tick write:statuses, write:media and read:accounts, then copy \"Your access token\"."},
		},
		Notes: "Mastodon and compatible servers. Posts are public; images only (no video). Limits (characters, images, image size) " +
			"are read from your instance when you connect. Needs a token with write:statuses write:media read:accounts.",
	}
}
