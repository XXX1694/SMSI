// Package discord is the Discord adapter. A user connects a channel by pasting
// an incoming-webhook URL (channel settings > Integrations > Webhooks). The whole
// URL is the credential: it is stored encrypted, never returned, logged or put
// into the profile. Only the webhook id, channel id, guild id and name are metadata.
//
// Limits: content <= 2000 chars, <= 10 attachments, 10 MiB per file (the
// documented default is higher; we stay below every figure seen, see
// docs/PLATFORMS.md). Discord has no idempotency key and no way to list a
// webhook's messages, so a timeout after send is an unknown outcome that goes to
// needs_review instead of being retried into a duplicate.
package discord

import (
	"net/http"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/adapters/safehttp"
)

// Name is the provider identifier.
const Name = "discord"

// Limits of the adapter.
const (
	MaxTextLength = 2000
	MaxMedia      = 10
	MaxFileBytes  = 10 << 20
)

// allowedHosts are the only hosts a webhook may live on (and the only hosts the client dials).
var allowedHosts = []string{"discord.com", "discordapp.com"}

// Config configures the adapter. The zero value is the production setting.
type Config struct {
	// APIBaseURL replaces https://<webhook host> in tests.
	APIBaseURL string
	// HTTPClient replaces the fixed-host safe client in tests.
	HTTPClient *http.Client
}

// Adapter implements provider.Provider, TokenConnector and Publisher.
type Adapter struct {
	base string
	http *http.Client
}

// New creates the adapter.
func New(cfg Config) *Adapter {
	client := cfg.HTTPClient
	if client == nil {
		// Uploads of ten images need more than the 15 s default.
		client = safehttp.NewSafeClient(safehttp.SafeClientConfig{Timeout: 60 * time.Second, AllowedHosts: allowedHosts})
	}
	return &Adapter{base: strings.TrimRight(cfg.APIBaseURL, "/"), http: client}
}

func (a *Adapter) Name() string        { return Name }
func (a *Adapter) DisplayName() string { return "Discord" }
func (a *Adapter) Supported() bool     { return true }
func (a *Adapter) Configured() bool    { return true }

// Capabilities of the Discord adapter.
func (a *Adapter) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		CanPublishText: true, CanPublishImage: true, CanDelete: true,
		MaxTextLength: MaxTextLength, MaxMediaCount: MaxMedia, MaxImageBytes: MaxFileBytes,
		ConnectMethod: provider.ConnectToken,
		ConnectFields: []provider.ConnectField{{
			Name: "webhook_url", Label: "Webhook URL", Kind: provider.FieldURL, Secret: true, Required: true,
			Placeholder: "https://discord.com/api/webhooks/...",
			Help:        "Channel settings > Integrations > Webhooks > New Webhook > Copy Webhook URL. The URL is a password: anyone who has it can post in that channel.",
		}},
		Notes: "Posts into one channel through its webhook, as the webhook's name. Text up to 2000 characters and up to 10 images " +
			"(10 MB each); mentions are not pinged. No titles, video, threads or scheduling on Discord's side. Deleting the webhook in " +
			"Discord disconnects the account. If a send times out, the post goes to review because Discord offers no way to check whether it arrived.",
	}
}
