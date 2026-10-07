// Package provider is the port every social network adapter implements.
// It has no I/O dependencies; adapters live in sibling packages.
package provider

import (
	"context"
	"io"
	"time"
)

// Connect methods.
const (
	ConnectOAuth    = "oauth"
	ConnectTelegram = "telegram_chat"
	ConnectNone     = "none"
)

// Capabilities describe what a provider can do (returned by GET /social/providers).
type Capabilities struct {
	CanPublishText   bool   `json:"can_publish_text"`
	CanPublishImage  bool   `json:"can_publish_image"`
	CanPublishVideo  bool   `json:"can_publish_video"`
	CanSchedule      bool   `json:"can_schedule"`
	CanDelete        bool   `json:"can_delete"`
	CanAnalytics     bool   `json:"can_analytics"`
	MaxTextLength    int    `json:"max_text_length"`
	MaxCaptionLength int    `json:"max_caption_length,omitempty"`
	MaxMediaCount    int    `json:"max_media_count"`
	RequiresApproval bool   `json:"requires_approval"`
	Notes            string `json:"notes"`
	// SafeToRetryAfterUnknown: the provider de-duplicates by idempotency key,
	// so re-sending after an unknown outcome cannot create a duplicate.
	SafeToRetryAfterUnknown bool   `json:"safe_to_retry_after_unknown"`
	ConnectMethod           string `json:"connect_method"`
}

// Provider is the base contract of every registered network.
type Provider interface {
	Name() string
	DisplayName() string
	Capabilities() Capabilities
	// Configured reports whether credentials/config needed for live use exist.
	Configured() bool
	// Supported is false for clearly-labelled stubs.
	Supported() bool
}

// AuthorizeParams are inputs to build the provider consent URL.
type AuthorizeParams struct {
	State         string
	CodeChallenge string
	RedirectURI   string
	// LoginHint optionally pre-selects an account at the provider (OAuth
	// login_hint). Providers that do not support it ignore it.
	LoginHint string
}

// Token is an OAuth token set.
type Token struct {
	AccessToken      string
	RefreshToken     string
	ExpiresAt        *time.Time
	RefreshExpiresAt *time.Time
	Scopes           []string
}

// Profile identifies the connected account. ID is the provider account id.
type Profile struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   string
	Metadata    map[string]any
}

// OAuth is implemented by providers using the authorization-code flow.
type OAuth interface {
	AuthorizeURL(p AuthorizeParams) string
	Exchange(ctx context.Context, code, verifier, redirectURI string) (Token, error)
	Profile(ctx context.Context, accessToken string) (Profile, error)
	Refresh(ctx context.Context, refreshToken string) (Token, error)
}

// ChatVerifier is implemented by non-OAuth chat providers (Telegram).
type ChatVerifier interface {
	VerifyChat(ctx context.Context, chat string) (Profile, error)
}

// AccountRef is the non-secret account data an adapter needs.
type AccountRef struct {
	ID                string
	ProviderAccountID string
	Username          string
	Metadata          map[string]any
}

// MediaFile is one attachment; Open streams the bytes from storage.
type MediaFile struct {
	Kind     string // "image" | "video"
	MimeType string
	Size     int64
	Name     string
	Open     func(ctx context.Context) (io.ReadCloser, error)
}

// PublishRequest is the input of Publish.
type PublishRequest struct {
	IdempotencyKey string
	Account        AccountRef
	AccessToken    string
	Text           string
	Media          []MediaFile
}

// PublishResult identifies the created post.
type PublishResult struct {
	ExternalID string
	URL        string
	Metadata   map[string]any
}

// DeleteRequest identifies a post to remove.
type DeleteRequest struct {
	Account     AccountRef
	AccessToken string
	ExternalID  string
}

// Publisher is implemented by providers that can post.
type Publisher interface {
	Publish(ctx context.Context, req PublishRequest) (PublishResult, error)
	Delete(ctx context.Context, req DeleteRequest) error
}

// Lookuper finds a previously published post by idempotency key, used to
// resolve attempts whose outcome is unknown.
type Lookuper interface {
	Lookup(ctx context.Context, req PublishRequest) (PublishResult, bool, error)
}
