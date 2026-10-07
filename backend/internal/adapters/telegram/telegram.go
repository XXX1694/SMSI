// Package telegram is the real Telegram Bot API adapter.
//
// Telegram is not OAuth: the operator configures one bot (TELEGRAM_BOT_TOKEN),
// the user adds the bot as an administrator of their channel/group and supplies
// the chat (@username or numeric id). The backend verifies admin rights with
// getMe + getChat + getChatMember before storing the account.
//
// Limits: text ≤ 4096 chars, caption ≤ 1024 chars, ≤ 10 media per album,
// photo uploads ≤ 10 MB, video uploads ≤ 50 MB (Bot API upload limits).
// No idempotency support and no lookup → unknown outcomes need review.
package telegram

import (
	"net/http"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

// Name is the provider identifier.
const Name = "telegram"

// Limits.
const (
	MaxTextLength    = 4096
	MaxCaptionLength = 1024
	MaxMedia         = 10
	MaxPhotoBytes    = 10 << 20
	MaxUploadBytes   = 50 << 20
)

// TokenRef is stored in account metadata instead of the bot token itself.
const TokenRef = "env:TELEGRAM_BOT_TOKEN"

// Config configures the adapter.
type Config struct {
	BotToken   string
	APIBaseURL string
	HTTPClient *http.Client
}

// Adapter implements provider.Provider, ChatVerifier and Publisher.
type Adapter struct {
	token string
	base  string
	http  *http.Client
}

// New creates the adapter.
func New(cfg Config) *Adapter {
	base := strings.TrimRight(cfg.APIBaseURL, "/")
	if base == "" {
		base = "https://api.telegram.org"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &Adapter{token: cfg.BotToken, base: base, http: client}
}

func (a *Adapter) Name() string        { return Name }
func (a *Adapter) DisplayName() string { return "Telegram" }
func (a *Adapter) Supported() bool     { return true }
func (a *Adapter) Configured() bool    { return a.token != "" }

// Capabilities of the Telegram adapter.
func (a *Adapter) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		CanPublishText: true, CanPublishImage: true, CanPublishVideo: true, CanDelete: true,
		MaxTextLength: MaxTextLength, MaxCaptionLength: MaxCaptionLength, MaxMediaCount: MaxMedia,
		ConnectMethod: provider.ConnectTelegram,
		Notes: "Add the SocialOS bot as an admin with 'Post messages' to your channel, then connect with @channel or chat id. " +
			"Captions with media are limited to 1024 characters; videos ≤ 50 MB.",
	}
}
