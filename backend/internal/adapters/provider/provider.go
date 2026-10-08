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
	// ConnectToken: the user pastes a static credential (webhook URL, app
	// password, API key) described by Capabilities.ConnectFields.
	ConnectToken = "token"
	ConnectNone  = "none"
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
	// ConnectFields describes the form of a ConnectToken provider.
	ConnectFields []ConnectField `json:"connect_fields,omitempty"`
	// MaxImageBytes is the largest single image the network accepts (0 = no known limit).
	MaxImageBytes int64 `json:"max_image_bytes,omitempty"`
	// RequiresTitle is true for article networks that reject posts without a title.
	RequiresTitle bool `json:"requires_title,omitempty"`
}

// Field kinds of a ConnectField.
const (
	FieldText   = "text"
	FieldSecret = "secret"
	FieldURL    = "url"
)

// ConnectField is one input of a token connect form.
type ConnectField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// Kind is FieldText, FieldSecret (rendered as a password input) or FieldURL (https only).
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
	// Secret marks a value that is a credential whatever its Kind (a webhook
	// URL is a url field and a password). Secret fields are never echoed,
	// logged or allowed in the profile, and forms render them as password inputs.
	Secret bool `json:"secret,omitempty"`
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

// ChatMessage is a message posted into a chat the platform bot can see, reduced
// to what ownership proof needs. Adapters produce it from their inbound update
// format; the accounts use case consumes it without knowing the platform.
type ChatMessage struct {
	// ChatID is the platform chat id, usable with ChatVerifier.VerifyChat.
	ChatID    string
	MessageID int64
	Text      string
	// SenderID identifies the human who posted. Empty when SenderTrusted.
	SenderID string
	// SenderTrusted is true when the platform itself guarantees that only
	// administrators can post this message (channel posts, anonymous admins).
	SenderTrusted bool
}

// ChatLinker is implemented by chat providers that let a user prove control of
// a chat by posting a one-time code there (Telegram).
type ChatLinker interface {
	ChatVerifier
	// BotUsername is the public handle users must add to their chat.
	BotUsername(ctx context.Context) (string, error)
	// ParseUpdate turns a raw inbound platform update (webhook body or polled
	// item) into a chat message. It returns (nil, nil) for updates that are
	// irrelevant (edits, joins, private chats, ...).
	ParseUpdate(raw []byte) (*ChatMessage, error)
	// IsChatAdmin reports whether a platform user owns or administers the chat.
	IsChatAdmin(ctx context.Context, chatID, userID string) (bool, error)
	// DeleteMessage removes a message from the chat.
	DeleteMessage(ctx context.Context, chatID string, messageID int64) error
	// LinkInstructions is the user-facing text that goes with a link code.
	LinkInstructions(botUsername, code string, ttl time.Duration) string
}

// TokenConnector is implemented by providers connected with a pasted credential.
type TokenConnector interface {
	// Verify makes a live whoami call with the submitted fields. Profile.Metadata
	// must hold non-secret data only; secret is the adapter-encoded credential
	// that the vault stores and Publish later receives as AccessToken.
	Verify(ctx context.Context, fields map[string]string) (p Profile, secret string, err error)
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
	// Title is the post title; only providers with Capabilities.RequiresTitle use it.
	Title string
	Text  string
	Media []MediaFile
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
