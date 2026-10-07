package telegram

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// WebhookSecretHeader carries the secret_token given to setWebhook on every delivery.
const WebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

// secretRe is what Telegram accepts for secret_token: 1-256 of A-Z a-z 0-9 _ -.
var secretRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// ValidWebhookSecret reports whether s is acceptable to Telegram as a secret_token.
func ValidWebhookSecret(s string) bool { return secretRe.MatchString(s) }

// SecretMatches compares a received secret header with the configured one in
// constant time. An empty configured secret never matches.
func SecretMatches(got, want string) bool {
	if want == "" {
		return false
	}
	// Hash both sides so the comparison time does not reveal the secret's length either.
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

// WebhookInfo is the part of getWebhookInfo the operator tooling shows.
type WebhookInfo struct {
	URL                  string `json:"url"`
	PendingUpdateCount   int    `json:"pending_update_count"`
	LastErrorMessage     string `json:"last_error_message"`
	HasCustomCertificate bool   `json:"has_custom_certificate"`
}

// SetWebhook registers url as the update endpoint. Telegram will send secret in
// the X-Telegram-Bot-Api-Secret-Token header of every request.
func (a *Adapter) SetWebhook(ctx context.Context, rawURL, secret string, dropPending bool) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("webhook URL must be an absolute https:// URL")
	}
	if !ValidWebhookSecret(secret) {
		return errors.New("webhook secret must be 1-256 characters of A-Z a-z 0-9 _ -")
	}
	return a.callJSON(ctx, "setWebhook", map[string]any{
		"url": strings.TrimSpace(rawURL), "secret_token": secret, "allowed_updates": AllowedUpdates,
		"drop_pending_updates": dropPending,
	}, nil)
}

// DeleteWebhook removes the webhook so getUpdates polling can be used again.
func (a *Adapter) DeleteWebhook(ctx context.Context, dropPending bool) error {
	return a.callJSON(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": dropPending}, nil)
}

// GetWebhookInfo returns the current webhook state.
func (a *Adapter) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	var info WebhookInfo
	err := a.callJSON(ctx, "getWebhookInfo", map[string]any{}, &info)
	return info, err
}
