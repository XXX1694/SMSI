// Package linkedin is the real LinkedIn adapter.
//
// Flow: OAuth 2.0 authorization code (+state, optional PKCE) → OpenID Connect
// /v2/userinfo for the member id (sub) → versioned REST API /rest/posts as
// urn:li:person:{sub} with the w_member_social scope. Images are uploaded via
// /rest/images?action=initializeUpload.
//
// Documented limits (MVP):
//   - Member posts only. Organization (company page) posting requires the
//     Marketing Developer Platform / Community Management API approval.
//   - commentary ≤ 3000 characters; up to 20 images (multiImage), no video.
//   - Refresh tokens are only issued to apps LinkedIn has approved for
//     programmatic refresh; otherwise access tokens live 60 days and the
//     account is marked expired when they lapse.
//   - No idempotency key support and no lookup → unknown outcomes need review.
package linkedin

import (
	"net/http"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// Name is the provider identifier.
const Name = "linkedin"

// Config configures the adapter. Zero URLs fall back to production endpoints.
type Config struct {
	ClientID     string
	ClientSecret string
	APIVersion   string // LinkedIn-Version header, YYYYMM
	UsePKCE      bool
	Scopes       []string
	AuthURL      string
	TokenURL     string
	APIBaseURL   string
	HTTPClient   *http.Client
}

// Adapter implements provider.Provider, OAuth and Publisher.
type Adapter struct {
	cfg  Config
	http *http.Client
}

// New creates the adapter with defaults applied.
func New(cfg Config) *Adapter {
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://www.linkedin.com/oauth/v2/authorization"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://www.linkedin.com/oauth/v2/accessToken"
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.linkedin.com"
	}
	cfg.APIBaseURL = strings.TrimRight(cfg.APIBaseURL, "/")
	if cfg.APIVersion == "" {
		cfg.APIVersion = "202606"
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email", "w_member_social"}
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Adapter{cfg: cfg, http: client}
}

func (a *Adapter) Name() string        { return Name }
func (a *Adapter) DisplayName() string { return "LinkedIn" }
func (a *Adapter) Supported() bool     { return true }
func (a *Adapter) Configured() bool    { return a.cfg.ClientID != "" && a.cfg.ClientSecret != "" }

// MaxTextLength is LinkedIn's commentary limit.
const MaxTextLength = 3000

// MaxImages is the multiImage limit.
const MaxImages = 20

// Capabilities of the LinkedIn adapter.
func (a *Adapter) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		CanPublishText:   true,
		CanPublishImage:  true,
		CanDelete:        true,
		MaxTextLength:    MaxTextLength,
		MaxMediaCount:    MaxImages,
		RequiresApproval: true,
		ConnectMethod:    provider.ConnectOAuth,
		Notes: "Personal profile posting via 'Share on LinkedIn' (w_member_social). Video not supported in this release. " +
			"Company pages require Marketing Developer Platform approval. No native scheduling; SocialOS schedules.",
	}
}
